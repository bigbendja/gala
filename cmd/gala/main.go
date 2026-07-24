// GALA — binario principal.
// Arranca la base de datos, la API y el frontend embebido, y abre el navegador.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net"
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
	"github.com/bigbendja/gala/internal/identify"
	"github.com/bigbendja/gala/internal/media"
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

	enum := localfs.NewEnumerator()
	scanner := scan.New(ctx, cfg, repo, enum, log)
	// Fin de escaneo de M2 despierta la cola de identificación de M3 y la de
	// sondeo de medios de M4 (Brief M3 §4 / M4 §2).
	scanner.AfterScan = func() {
		idQueue.Enqueue()
		mediaQueue.Enqueue()
	}
	apiSrv := api.New(st, repo, cfg, scanner, enum, idQueue, tmdbClient, cacheDir, log, ffmpeg, hlsManager)

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

	addr := net.JoinHostPort(cfg.Bind, fmt.Sprintf("%d", cfg.Port))
	srv := &http.Server{
		Addr:              addr,
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		log.Info("GALA arrancado", "version", api.Version, "url", localURL(cfg))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("servidor http", "err", err)
			os.Exit(1)
		}
	}()

	if !*noBrowser {
		openBrowser(localURL(cfg), log)
	}

	<-ctx.Done()

	log.Info("apagando…")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
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
