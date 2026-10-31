package update

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
)

const (
	maxBinary   = 200 << 20 // tope de descarga
	versionWait = 15 * time.Second
	exitDelay   = time.Second // deja que la UI vea «Reiniciando…» antes de salir
)

// Fases de una actualización, tal como las ve la UI.
type Phase string

const (
	PhaseIdle        Phase = "idle"
	PhaseDownloading Phase = "downloading"
	PhaseVerifying   Phase = "verifying"
	PhaseInstalling  Phase = "installing"
	PhaseRestarting  Phase = "restarting"
	PhaseError       Phase = "error"
)

// Status es el progreso de la actualización en curso.
type Status struct {
	Phase   Phase  `json:"phase"`
	Done    int64  `json:"done"`
	Total   int64  `json:"total"`
	Error   string `json:"error,omitempty"`
	Version string `json:"version,omitempty"`
}

var (
	ErrBusy          = errors.New("ya hay una actualización en curso")
	ErrNotSupervised = errors.New("la actualización desde la app solo funciona cuando GALA corre como servicio")
	ErrNoUpdate      = errors.New("no hay una actualización disponible")
)

// PlaybackActiveError: hay reproducciones en curso; la UI avisa y ofrece
// «Actualizar igualmente».
type PlaybackActiveError struct{ Count int }

func (e *PlaybackActiveError) Error() string {
	return fmt.Sprintf("hay %d reproducción(es) en curso", e.Count)
}

// Installer descarga, verifica e instala la versión nueva.
type Installer struct {
	Checker *Checker
	Client  *http.Client
	// ExePath es el binario en ejecución. Los .new/.prev/marcador viven a su lado.
	ExePath string
	// DB es la base abierta, de la que se hace la copia de seguridad (VACUUM INTO).
	DB  *sql.DB
	Log *slog.Logger
	// Supervised: GALA corre bajo un supervisor que lo relanza al salir (systemd).
	Supervised bool
	// PlaybackActive devuelve cuántas reproducciones hay en curso (nil = 0).
	PlaybackActive func() int
	// RequestExit provoca la salida ordenada del proceso (la relanza el supervisor).
	RequestExit func()
	// FreeSpace devuelve los bytes libres en dir (nil = consulta real al sistema).
	FreeSpace func(dir string) (uint64, error)
	// RunVersion ejecuta `bin --version` y devuelve su salida (nil = ejecución real).
	RunVersion func(ctx context.Context, bin string) (string, error)

	mu     sync.Mutex
	status Status
}

func (in *Installer) dir() string      { return filepath.Dir(in.ExePath) }
func (in *Installer) newPath() string  { return in.ExePath + NewSuffix }
func (in *Installer) prevPath() string { return in.ExePath + PrevSuffix }

// Status devuelve el progreso actual.
func (in *Installer) Status() Status {
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.status.Phase == "" {
		return Status{Phase: PhaseIdle}
	}
	return in.status
}

func (in *Installer) set(s Status) {
	in.mu.Lock()
	in.status = s
	in.mu.Unlock()
}

func (in *Installer) progress(done, total int64) {
	in.mu.Lock()
	in.status.Done, in.status.Total = done, total
	in.mu.Unlock()
}

// CanApply dice si esta instalación puede actualizarse desde la app y, si no, por qué.
func (in *Installer) CanApply() (bool, string) {
	if !in.Supervised {
		return false, "La actualización desde la app solo está disponible cuando GALA corre como servicio. Descarga la versión nueva desde la página de la versión."
	}
	return true, ""
}

// Start valida y lanza la actualización en segundo plano. Devuelve enseguida;
// el progreso se lee con Status().
func (in *Installer) Start(force bool) error {
	if ok, _ := in.CanApply(); !ok {
		return ErrNotSupervised
	}
	info, ok := in.Checker.Cached()
	if !ok || !info.UpdateAvailable || info.AssetURL == "" || info.Checksum == "" {
		return ErrNoUpdate
	}
	if !force && in.PlaybackActive != nil {
		if n := in.PlaybackActive(); n > 0 {
			return &PlaybackActiveError{Count: n}
		}
	}
	in.mu.Lock()
	switch in.status.Phase {
	case PhaseDownloading, PhaseVerifying, PhaseInstalling, PhaseRestarting:
		in.mu.Unlock()
		return ErrBusy
	}
	in.status = Status{Phase: PhaseDownloading, Version: info.Latest}
	in.mu.Unlock()

	go func() {
		if err := in.run(context.Background(), info); err != nil {
			in.Log.Error("actualización fallida", "err", err)
			in.set(Status{Phase: PhaseError, Error: err.Error(), Version: info.Latest})
		}
	}()
	return nil
}

func (in *Installer) run(ctx context.Context, info Info) error {
	defer func() {
		// Si algo falló antes del intercambio, no deja el binario descargado.
		if in.Status().Phase != PhaseRestarting {
			_ = os.Remove(in.newPath())
		}
	}()

	// 0. Espacio: el binario nuevo se descarga junto al actual (hace falta el
	//    doble de su tamaño, como margen para la copia y los temporales).
	if err := in.checkSpace(info.AssetSize); err != nil {
		return err
	}

	// 1. Descarga a gala.new (mismo directorio: el rename posterior es atómico)
	//    verificando el SHA256 mientras llega.
	if err := in.download(ctx, info); err != nil {
		return err
	}

	// 2. Comprobar que ejecuta y es la versión esperada, ANTES de tocar nada.
	in.set(Status{Phase: PhaseVerifying, Version: info.Latest})
	if err := os.Chmod(in.newPath(), 0o755); err != nil {
		return fmt.Errorf("permisos del binario nuevo: %w", err)
	}
	out, err := in.runVersion(ctx, in.newPath())
	if err != nil {
		return fmt.Errorf("el binario descargado no se puede ejecutar en este equipo: %w", err)
	}
	if !strings.Contains(out, info.Latest) {
		return fmt.Errorf("el binario descargado no es la versión %s", info.Latest)
	}

	// 3. Copia de seguridad de la BD (consistente y en caliente). El esquema
	//    solo avanza (migraciones hacia delante) y el binario viejo no detecta
	//    uno más nuevo: por eso el rollback puede restaurarla.
	in.set(Status{Phase: PhaseInstalling, Version: info.Latest})
	if err := in.backupDB(ctx); err != nil {
		return err
	}

	// 4. Intercambio a prueba de cortes. El marcador se escribe ANTES de tocar
	//    el binario (intención) y se actualiza DESPUÉS (hecho); el preflight
	//    reconcilia cualquier corte entre medias mirando qué ficheros existen.
	cur := in.Checker.Current
	if err := WriteMarker(in.dir(), Marker{State: StatePreparing, From: cur, New: info.Latest}); err != nil {
		return fmt.Errorf("escribir el marcador: %w", err)
	}
	if err := os.Rename(in.ExePath, in.prevPath()); err != nil {
		_ = os.Remove(filepath.Join(in.dir(), PendingFile))
		return fmt.Errorf("guardar el binario actual como .prev: %w", err)
	}
	if err := os.Rename(in.newPath(), in.ExePath); err != nil {
		// Deshacer: que el binario viejo vuelva a su sitio.
		if rerr := os.Rename(in.prevPath(), in.ExePath); rerr != nil {
			in.Log.Error("no se pudo restaurar el binario tras un intercambio fallido", "err", rerr)
		} else {
			_ = os.Remove(filepath.Join(in.dir(), PendingFile))
		}
		return fmt.Errorf("instalar el binario nuevo: %w", err)
	}
	if err := WriteMarker(in.dir(), Marker{State: StateSwapped, From: cur, New: info.Latest}); err != nil {
		// El intercambio ya está hecho: el preflight lo reconcilia igualmente
		// (marcador «preparing» con .new ausente y .prev presente = intercambiado).
		in.Log.Warn("no se pudo actualizar el marcador a «swapped»", "err", err)
	}

	// 5. Salida ordenada: el supervisor (Restart=always) arranca el binario nuevo.
	in.set(Status{Phase: PhaseRestarting, Version: info.Latest})
	in.Log.Info("actualización instalada; reiniciando", "de", cur, "a", info.Latest)
	if in.RequestExit != nil {
		time.AfterFunc(exitDelay, in.RequestExit)
	}
	return nil
}

func (in *Installer) download(ctx context.Context, info Info) error {
	if err := in.Checker.allowURL(info.AssetURL); err != nil {
		return err
	}
	client := *in.Client
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("demasiadas redirecciones")
		}
		return in.Checker.allowURL(req.URL.String()) // también las redirecciones
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, info.AssetURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", "GALA-updater")
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("descargar: %w", ErrOffline)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("descargar: el servidor respondió %d", resp.StatusCode)
	}
	total := resp.ContentLength
	if total > maxBinary {
		return fmt.Errorf("el binario pesa más de %d MB", maxBinary>>20)
	}
	in.progress(0, total)

	f, err := os.OpenFile(in.newPath(), os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		if isDiskFull(err) {
			return errDiskFull
		}
		return fmt.Errorf("crear %s: %w", filepath.Base(in.newPath()), err)
	}
	h := sha256.New()
	pw := &progressWriter{w: io.MultiWriter(f, h), on: func(n int64) { in.progress(n, total) }}
	n, err := io.Copy(pw, io.LimitReader(resp.Body, maxBinary+1))
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		if isDiskFull(err) {
			return errDiskFull
		}
		return fmt.Errorf("descargar: %w", err)
	}
	if n > maxBinary {
		return fmt.Errorf("el binario pesa más de %d MB", maxBinary>>20)
	}
	in.set(Status{Phase: PhaseVerifying, Done: n, Total: n, Version: info.Latest})
	if got := hex.EncodeToString(h.Sum(nil)); got != strings.ToLower(info.Checksum) {
		return errors.New("la descarga está corrupta (el checksum no coincide); no se ha instalado nada")
	}
	return nil
}

type progressWriter struct {
	w  io.Writer
	n  int64
	on func(int64)
}

func (p *progressWriter) Write(b []byte) (int, error) {
	n, err := p.w.Write(b)
	p.n += int64(n)
	p.on(p.n)
	return n, err
}

func (in *Installer) runVersion(ctx context.Context, bin string) (string, error) {
	if in.RunVersion != nil {
		return in.RunVersion(ctx, bin)
	}
	ctx, cancel := context.WithTimeout(ctx, versionWait)
	defer cancel()
	out, err := exec.CommandContext(ctx, bin, "--version").Output() // argumentos fijos, sin shell
	return string(out), err
}

// backupDB copia la BD a gala.db.pre-update.new (VACUUM INTO: copia
// consistente con el servidor en marcha). La copia de la actualización
// ANTERIOR (gala.db.pre-update) se conserva hasta que esta tenga éxito.
func (in *Installer) backupDB(ctx context.Context) error {
	if in.DB == nil {
		return errors.New("sin base de datos para la copia de seguridad")
	}
	dst := filepath.Join(in.dir(), DBBackupNew)
	_ = os.Remove(dst)
	q := "VACUUM INTO '" + strings.ReplaceAll(dst, "'", "''") + "'"
	if _, err := in.DB.ExecContext(ctx, q); err != nil {
		return fmt.Errorf("copia de seguridad de la base de datos: %w", err)
	}
	return nil
}

var errDiskFull = errors.New("el disco está lleno: no hay espacio para descargar la versión nueva. Libera espacio y vuelve a intentarlo; no se ha cambiado nada")

// isDiskFull reconoce «disco lleno» (ENOSPC en Unix, ERROR_DISK_FULL/112 en Windows).
func isDiskFull(err error) bool {
	var errno syscall.Errno
	if !errors.As(err, &errno) {
		return false
	}
	return errno == syscall.ENOSPC || (runtime.GOOS == "windows" && (errno == 112 || errno == 39))
}

// checkSpace falla con un mensaje claro si no cabe el doble del binario.
func (in *Installer) checkSpace(assetSize int64) error {
	if assetSize <= 0 {
		return nil // tamaño desconocido: lo detectará la propia descarga
	}
	free := freeBytes
	if in.FreeSpace != nil {
		free = in.FreeSpace
	}
	have, err := free(in.dir())
	if err != nil {
		in.Log.Warn("actualización: no se pudo consultar el espacio libre", "err", err)
		return nil // sin dato fiable, que decida la descarga (ENOSPC se traduce igual)
	}
	need := uint64(assetSize) * 2
	if have < need {
		return fmt.Errorf("no hay espacio suficiente en el disco para actualizar: hacen falta %s libres y hay %s. Libera espacio y vuelve a intentarlo; no se ha cambiado nada",
			humanMB(need), humanMB(have))
	}
	return nil
}

func humanMB(n uint64) string {
	return strings.ReplaceAll(fmt.Sprintf("%.1f MB", float64(n)/(1024*1024)), ".", ",")
}
