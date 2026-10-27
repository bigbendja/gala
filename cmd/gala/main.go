// GALA — binario principal.
// Arranca la base de datos, la API y el frontend embebido, y abre el navegador.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	"github.com/bigbendja/gala/internal/api"
	"github.com/bigbendja/gala/internal/catalog"
	"github.com/bigbendja/gala/internal/config"
	"github.com/bigbendja/gala/internal/health"
	"github.com/bigbendja/gala/internal/identify"
	"github.com/bigbendja/gala/internal/ingest"
	"github.com/bigbendja/gala/internal/lan"
	"github.com/bigbendja/gala/internal/media"
	"github.com/bigbendja/gala/internal/providers/httpprovider"
	"github.com/bigbendja/gala/internal/providers/localfs"
	"github.com/bigbendja/gala/internal/scan"
	"github.com/bigbendja/gala/internal/store"
	"github.com/bigbendja/gala/internal/stream"
	"github.com/bigbendja/gala/web"
)

func main() {
	noBrowser := flag.Bool("no-browser", false, "no abrir el navegador al arrancar")
	demo := flag.Bool("demo", false, "poblar la BD con datos ficticios de desarrollo (solo si está vacía)")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Contexto de vida de la aplicación: lo usan el poller de volúmenes y los
	// escaneos en segundo plano, que deben sobrevivir a peticiones HTTP
	// concretas y apagarse limpiamente con Ctrl+C / señal del sistema.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		log.Error("configuración", "err", err)
		os.Exit(1)
	}

	st, err := store.Open(cfg.DBPath())
	if err != nil {
		log.Error("base de datos", "err", err)
		os.Exit(1)
	}
	defer st.Close()

	repo := catalog.NewRepo(st.DB)

	if *demo {
		n, err := catalog.SeedDemo(context.Background(), repo)
		if err != nil {
			log.Error("seed demo", "err", err)
			os.Exit(1)
		}
		if n > 0 {
			log.Info("datos de demostración creados", "items", n)
		} else {
			log.Info("la BD ya tiene datos; --demo ignorado")
		}
	}

	cacheDir, err := cfg.CacheDir()
	if err != nil {
		log.Error("cache", "err", err)
		os.Exit(1)
	}

	tmdbKey := cfg.TMDBAPIKey
	tmdbClient := identify.NewClient(tmdbKey, cacheDir, repo)
	tmdbClient.Log = log
	idQueue := identify.NewQueue(ctx, repo, tmdbClient, log)
	go idQueue.Run()

	// ffmpeg/ffprobe: nunca aborta el arranque si faltan (Brief M4 §1); ver
	// README para colocarlos a mano en desarrollo (M11 los empaquetará).
	exeDir, err := config.BaseDir()
	if err != nil {
		log.Error("directorio base", "err", err)
		os.Exit(1)
	}
	ffmpeg := media.Locate(exeDir, log)
	mediaQueue := media.NewQueue(ctx, repo, ffmpeg, log)
	go mediaQueue.Run()
	hlsManager := stream.NewManager(ctx, ffmpeg.FFmpegPath, filepath.Join(cacheDir, "hls"), log)
	go hlsManager.RunSweeper()

	// Ingesta universal (M5): crea items/sources provisionales y despierta
	// la MISMA cola de identificación de M3, sin lógica de matching propia.
	ingestPipeline := ingest.NewPipeline(repo, idQueue, log)

	// Salud de fuentes (M6): un checker por provider; drive/mega y
	// cualquier provider desconocido caen al fallback de Worker
	// (sin_verificar, nunca caída — no poder verificar no es estar caído).
	healthCheckers := map[string]health.Checker{
		localfs.ProviderID:      &health.LocalfsChecker{Repo: repo},
		httpprovider.ProviderID: health.NewHTTPChecker(),
		"drive":                 health.UnverifiableChecker{Detail: "pendiente de conector"},
		"mega":                  health.UnverifiableChecker{Detail: "pendiente de conector"},
	}
	healthWorker := health.NewWorker(ctx, repo, healthCheckers, log)
	go healthWorker.Run()
	ingestPipeline.Health = healthWorker

	enum := localfs.NewEnumerator()
	scanner := scan.New(ctx, cfg, repo, enum, log)
	// Fin de escaneo de M2 despierta la cola de identificación de M3, la de
	// sondeo de medios de M4 y el worker de salud de M6 (fuentes recién
	// dadas de alta, aún sin_verificar).
	scanner.AfterScan = func() {
		idQueue.Enqueue()
		mediaQueue.Enqueue()
		healthWorker.Enqueue()
	}
	apiSrv := api.New(st, repo, cfg, scanner, enum, idQueue, tmdbClient, cacheDir, log, ffmpeg, hlsManager, ingestPipeline, healthWorker)

	// Poller de volúmenes: detecta conexión/desconexión de discos y dispara
	// el re-escaneo de las carpetas registradas que vivan en ellos.
	poller := localfs.NewPoller(enum, func() []string { return cfg.MediaFolders }, repo,
		func(folders []string) {
			if err := scanner.Start(folders); err != nil && !errors.Is(err, scan.ErrAlreadyRunning) {
				log.Error("re-escaneo tras cambio de volumen", "err", err)
			}
		}, log)
	go poller.Run(ctx)

	// Escaneo inicial asíncrono: no bloquea el arranque, la galería se llena
	// en vivo mientras el servidor ya está escuchando.
	if err := scanner.Start(nil); err != nil {
		log.Warn("escaneo inicial", "err", err)
	}

	mux := http.NewServeMux()
	apiSrv.Routes(mux)
	mux.Handle("/", web.Handler())

	// Modo LAN (M7): el guardián envuelve TODO el mux. Con bind 127.0.0.1 solo
	// llegan peticiones del propio PC y no hace nada; con 0.0.0.0 limita a los
	// demás dispositivos a ver/reproducir (y al PIN, si lo hay).
	guard := lan.NewGuard(cfg)
	apiSrv.LanGuard = guard
	srv := &http.Server{
		Handler:           guard.Middleware(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}
	listener := lan.NewController(srv, log)
	apiSrv.LanListen = func(bind string) error { return listener.Listen(bind, cfg.Port) }

	bind, _ := cfg.LanState()
	if err := listener.Listen(bind, cfg.Port); err != nil {
		log.Error("servidor http", "err", err)
		os.Exit(1)
	}
	log.Info("GALA arrancado", "version", api.Version, "url", localURL(cfg), "modo_lan", cfg.LanEnabled())

	if !*noBrowser {
		openBrowser(localURL(cfg), log)
	}

	<-ctx.Done()

	log.Info("apagando…")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	listener.Close()
}

// localURL devuelve la URL local aunque el bind sea 0.0.0.0.
func localURL(cfg *config.Config) string {
	host := cfg.Bind
	if host == "0.0.0.0" || host == "" {
		host = "127.0.0.1"
	}
	return fmt.Sprintf("http://%s:%d", host, cfg.Port)
}

// openBrowser abre la URL en el navegador por defecto del sistema.
func openBrowser(url string, log *slog.Logger) {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	default:
		cmd = exec.Command("xdg-open", url)
	}
	if err := cmd.Start(); err != nil {
		log.Warn("no se pudo abrir el navegador; abre manualmente", "url", url, "err", err)
	}
}
