package stream

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"sync"
	"time"
)

var (
	// ErrTooManySessions: límite global de sesiones de remux concurrentes
	// alcanzado (Brief M4 §4: no fundir la CPU en hardware modesto).
	ErrTooManySessions = errors.New("stream: demasiadas sesiones de remux activas")
	// ErrSessionNotFound: sesión HLS desconocida o ya limpiada (huérfana,
	// reemplazada por un seek, o terminada).
	ErrSessionNotFound = errors.New("stream: sesión HLS no encontrada")
	// ErrFFmpegUnavailable: no se puede remuxear sin ffmpeg.
	ErrFFmpegUnavailable = errors.New("stream: ffmpeg no disponible")
)

const (
	// maxGlobalSessions es el límite de sesiones de remux concurrentes en
	// toda la aplicación (Brief M4 §4).
	maxGlobalSessions = 3
	// maxSessionsPerSource: al iniciar una nueva (p. ej. un seek), se cierra
	// la más antigua de la misma fuente si ya había este número (Brief §4).
	maxSessionsPerSource = 2
	// orphanMaxAge: una sesión (en memoria o en disco) más vieja que esto sin
	// actividad se considera huérfana y se borra (Brief §4).
	orphanMaxAge = 30 * time.Minute
	// orphanSweepInterval: cadencia del barrido periódico (Brief §4).
	orphanSweepInterval = 10 * time.Minute
	// hlsSegmentSeconds es la duración de cada segmento .ts (Brief §4).
	hlsSegmentSeconds = 6
	// playlistReadyTimeout acota la espera a que ffmpeg escriba el primer
	// segmento antes de responder a quien pidió iniciar la sesión.
	playlistReadyTimeout = 12 * time.Second

	playlistName = "index.m3u8"
)

// Session es una sesión de remux HLS activa para una fuente.
type Session struct {
	ID       string
	SourceID int64
	Dir      string
	StartAt  float64 // segundos de arranque en el archivo original (-ss)

	cmd        *exec.Cmd
	cancel     context.CancelFunc
	createdAt  time.Time
	lastAccess time.Time
	done       chan struct{} // cerrado cuando el proceso ffmpeg termina
}

// Manager gestiona las sesiones de remux HLS activas: como máximo
// maxGlobalSessions en total y maxSessionsPerSource por fuente (Brief M4
// §4). Las sesiones viven más que una sola petición HTTP (el cliente pide
// segmentos sueltos a lo largo del tiempo), así que su ciclo de vida cuelga
// del ctx de aplicación pasado a NewManager, no del contexto de cada
// petición — salvo la propia creación (ver StartSession).
type Manager struct {
	ctx        context.Context
	ffmpegPath string
	cacheDir   string // cache/hls/
	log        *slog.Logger

	mu       sync.Mutex
	sessions map[string]*Session
}

// NewManager crea el gestor de sesiones HLS. ctx debe vivir mientras viva la
// aplicación: al cancelarse, todas las sesiones activas matan su ffmpeg.
func NewManager(ctx context.Context, ffmpegPath, cacheDir string, log *slog.Logger) *Manager {
	return &Manager{ctx: ctx, ffmpegPath: ffmpegPath, cacheDir: cacheDir, log: log, sessions: map[string]*Session{}}
}

// StartSession lanza una nueva sesión de remux -c copy para sourceID, leyendo
// srcPath desde startSeconds (Brief M4 §4: "seeking... arranque con -ss
// {segundos} creando sesión nueva"). Si la fuente ya tenía
// maxSessionsPerSource sesiones activas, se cierra la más antigua antes de
// crear la nueva. requestCtx es el contexto de la petición HTTP que originó
// el arranque: si se cancela ANTES de que el playlist esté listo (cliente
// corta mientras se está preparando), la sesión recién creada se aborta
// (Brief §4, "cancelación... contexto del request"); una vez lista, la
// sesión sigue viva de forma independiente (ver Manager).
func (m *Manager) StartSession(requestCtx context.Context, sourceID int64, srcPath string, startSeconds float64) (*Session, error) {
	if m.ffmpegPath == "" {
		return nil, ErrFFmpegUnavailable
	}

	m.mu.Lock()
	var ownSessions []*Session
	for _, s := range m.sessions {
		if s.SourceID == sourceID {
			ownSessions = append(ownSessions, s)
		}
	}
	if len(ownSessions) >= maxSessionsPerSource {
		sort.Slice(ownSessions, func(i, j int) bool { return ownSessions[i].createdAt.Before(ownSessions[j].createdAt) })
		m.removeLocked(ownSessions[0])
	}
	if len(m.sessions) >= maxGlobalSessions {
		m.mu.Unlock()
		return nil, ErrTooManySessions
	}
	m.mu.Unlock()

	id := newSessionID()
	dir := filepath.Join(m.cacheDir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("crear sesión hls: %w", err)
	}

	procCtx, cancel := context.WithCancel(m.ctx)
	cmd := exec.CommandContext(procCtx, m.ffmpegPath, buildHLSArgs(srcPath, dir, startSeconds)...)

	if err := cmd.Start(); err != nil {
		cancel()
		os.RemoveAll(dir)
		return nil, fmt.Errorf("lanzar ffmpeg: %w", err)
	}

	sess := &Session{
		ID: id, SourceID: sourceID, Dir: dir, StartAt: startSeconds,
		cmd: cmd, cancel: cancel, createdAt: time.Now(), lastAccess: time.Now(),
		done: make(chan struct{}),
	}
	go func() {
		err := cmd.Wait()
		if err != nil && procCtx.Err() == nil && m.log != nil {
			m.log.Warn("stream: ffmpeg remux terminó con error", "session", id, "err", err)
		}
		close(sess.done)
	}()

	m.mu.Lock()
	m.sessions[id] = sess
	m.mu.Unlock()

	if err := waitPlaylist(requestCtx, filepath.Join(dir, playlistName), playlistReadyTimeout); err != nil {
		m.StopSession(id)
		return nil, fmt.Errorf("el remux no generó la lista de reproducción a tiempo: %w", err)
	}
	return sess, nil
}

// buildHLSArgs construye los argumentos de ffmpeg para remux -c copy a HLS
// (Brief M4 §4). Sin transcode: solo copia de streams, coste de CPU casi
// nulo (CLAUDE.md/Brief: "sin transcode de vídeo en MVP1").
func buildHLSArgs(srcPath, dir string, startSeconds float64) []string {
	args := []string{"-y"}
	if startSeconds > 0 {
		args = append(args, "-ss", strconv.FormatFloat(startSeconds, 'f', 3, 64))
	}
	return append(args,
		"-i", srcPath,
		"-c", "copy",
		"-f", "hls",
		"-hls_time", strconv.Itoa(hlsSegmentSeconds),
		"-hls_playlist_type", "event",
		"-hls_segment_filename", filepath.Join(dir, "seg%05d.ts"),
		filepath.Join(dir, playlistName),
	)
}

// waitPlaylist espera a que path exista y no esté vacío (ffmpeg ya escribió
// el primer segmento), hasta timeout o a que ctx se cancele.
func waitPlaylist(ctx context.Context, path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(150 * time.Millisecond)
	defer ticker.Stop()
	for {
		if info, err := os.Stat(path); err == nil && info.Size() > 0 {
			return nil
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("tiempo agotado esperando %s", path)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

// Session devuelve la sesión id, marcándola como usada recientemente
// (Brief §4: el barrido de huérfanas se basa en inactividad).
func (m *Manager) Session(id string) (*Session, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess, ok := m.sessions[id]
	if ok {
		sess.lastAccess = time.Now()
	}
	return sess, ok
}

// StopSession termina y limpia una sesión conocida. No hace nada si id no
// existe (ya limpiada).
func (m *Manager) StopSession(id string) {
	m.mu.Lock()
	sess, ok := m.sessions[id]
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
	if ok {
		m.stopProcess(sess)
	}
}

// removeLocked quita sess del mapa (el llamante ya tiene m.mu) y libera su
// proceso en segundo plano, para no bloquear a quien tiene el lock mientras
// se espera a que ffmpeg termine.
func (m *Manager) removeLocked(sess *Session) {
	delete(m.sessions, sess.ID)
	go m.stopProcess(sess)
}

func (m *Manager) stopProcess(sess *Session) {
	sess.cancel()
	<-sess.done
	os.RemoveAll(sess.Dir)
}

// CleanupOrphans cierra las sesiones en memoria sin actividad desde hace más
// de maxAge (Brief §4).
func (m *Manager) CleanupOrphans(maxAge time.Duration) {
	cutoff := time.Now().Add(-maxAge)
	m.mu.Lock()
	var stale []*Session
	for id, s := range m.sessions {
		if s.lastAccess.Before(cutoff) {
			stale = append(stale, s)
			delete(m.sessions, id)
		}
	}
	m.mu.Unlock()
	for _, s := range stale {
		m.stopProcess(s)
	}
}

// CleanupOrphanDirs borra subdirectorios de cache/hls/ que no correspondan a
// ninguna sesión en memoria y sean más viejos que maxAge: restos de una
// sesión anterior de GALA que no se cerró limpiamente (proceso matado,
// apagado brusco). Se llama al arrancar y periódicamente (Brief §4).
func (m *Manager) CleanupOrphanDirs(maxAge time.Duration) {
	entries, err := os.ReadDir(m.cacheDir)
	if err != nil {
		return // cache/hls/ aún no existe: nada que limpiar
	}
	m.mu.Lock()
	tracked := make(map[string]bool, len(m.sessions))
	for id := range m.sessions {
		tracked[id] = true
	}
	m.mu.Unlock()

	cutoff := time.Now().Add(-maxAge)
	for _, e := range entries {
		if !e.IsDir() || tracked[e.Name()] {
			continue
		}
		info, err := e.Info()
		if err != nil || info.ModTime().After(cutoff) {
			continue
		}
		os.RemoveAll(filepath.Join(m.cacheDir, e.Name()))
	}
}

// RunSweeper limpia sesiones huérfanas (en memoria y en disco) al arrancar y
// cada orphanSweepInterval (Brief §4). Debe lanzarse en su propia goroutine;
// termina al cancelarse el ctx de Manager.
func (m *Manager) RunSweeper() {
	sweep := func() {
		m.CleanupOrphans(orphanMaxAge)
		m.CleanupOrphanDirs(orphanMaxAge)
	}
	sweep()
	ticker := time.NewTicker(orphanSweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-m.ctx.Done():
			return
		case <-ticker.C:
			sweep()
		}
	}
}

// reHLSFilename son los únicos nombres de archivo que ServeFile acepta
// dentro de una sesión: la lista de reproducción y segmentos numerados
// (Brief §4: anti path-traversal también aquí, CLAUDE.md).
var reHLSFilename = regexp.MustCompile(`^(index\.m3u8|seg\d{5}\.ts)$`)

// ServeFile sirve un fichero (playlist o segmento) de la sesión sessionID.
func (m *Manager) ServeFile(w http.ResponseWriter, r *http.Request, sessionID, filename string) error {
	sess, ok := m.Session(sessionID)
	if !ok {
		return ErrSessionNotFound
	}
	if !reHLSFilename.MatchString(filename) {
		return fmt.Errorf("stream: nombre de fichero hls no válido: %q", filename)
	}
	full := filepath.Join(sess.Dir, filename)
	f, err := os.Open(full)
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil {
		return err
	}
	w.Header().Set("Content-Type", ContentType(filename))
	http.ServeContent(w, r, filename, info.ModTime(), f)
	return nil
}

func newSessionID() string {
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		return strconv.FormatInt(time.Now().UnixNano(), 36)
	}
	return hex.EncodeToString(b)
}
