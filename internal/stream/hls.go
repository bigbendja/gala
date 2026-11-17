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
	// maxGlobalSessions es el límite de sesiones HLS concurrentes en toda la
	// aplicación (Brief M4 §4).
	maxGlobalSessions = 3
	// maxTranscodeSessions acota las sesiones que RECODIFICAN vídeo a la vez:
	// transcodificar pesa mucho más que remuxar (copiar), así que en hardware
	// modesto conviene un tope más estricto que el global, incluso con
	// aceleración por hardware.
	maxTranscodeSessions = 2
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
	// transcodeReadyTimeout: margen mayor para el primer segmento cuando se
	// recodifica el vídeo (más lento que copiarlo), sobre todo por software.
	transcodeReadyTimeout = 25 * time.Second

	playlistName = "index.m3u8"
)

// Session es una sesión HLS activa para una fuente (remux o transcode).
type Session struct {
	ID       string
	SourceID int64
	Dir      string
	StartAt  float64 // segundos de arranque en el archivo original (-ss)

	transcode  bool // recodifica vídeo (pesa): cuenta para maxTranscodeSessions
	cmd        *exec.Cmd
	cancel     context.CancelFunc
	createdAt  time.Time
	lastAccess time.Time
	done       chan struct{} // cerrado cuando el proceso ffmpeg termina
}

// videoEncoder describe cómo codificar vídeo a H.264 en buildHLSArgs: los
// argumentos globales antes de -i (p. ej. el dispositivo VAAPI) y los de la
// pista de vídeo. Lo elige DetectVideoEncoder al arrancar.
type videoEncoder struct {
	name     string   // para logs: "vaapi", "software"
	preInput []string // antes de -i (p. ej. -vaapi_device ...)
	video    []string // sustituye a "-c:v copy": filtros + -c:v <enc> + calidad
}

// softwareH264 es el codificador de respaldo: x264 por software. Un preset
// rápido para ir en tiempo real en hardware modesto con un stream.
var softwareH264 = videoEncoder{
	name:  "software",
	video: []string{"-c:v", "libx264", "-preset", "veryfast", "-crf", "23"},
}

const vaapiDevice = "/dev/dri/renderD128"

// DetectVideoEncoder elige el codificador H.264 para transcodificar. Prueba
// VAAPI DE VERDAD (un encode diminuto): solo lo usa si funciona con los
// permisos reales del proceso — si el usuario que ejecuta GALA no está en el
// grupo 'render', VAAPI da permiso denegado y se cae a software sin romper
// nada. Si no hay dispositivo o no funciona, x264 por software, que cualquier
// CPU decente aguanta para un stream.
func DetectVideoEncoder(ffmpegPath string, log *slog.Logger) videoEncoder {
	if ffmpegPath == "" {
		return softwareH264
	}
	if _, err := os.Stat(vaapiDevice); err == nil && vaapiWorks(ffmpegPath) {
		if log != nil {
			log.Info("stream: transcodificación acelerada por hardware (VAAPI) disponible")
		}
		return videoEncoder{
			name:     "vaapi",
			preInput: []string{"-vaapi_device", vaapiDevice},
			video:    []string{"-vf", "format=nv12,hwupload", "-c:v", "h264_vaapi", "-qp", "24"},
		}
	}
	if log != nil {
		log.Info("stream: transcodificación por software (x264); sin aceleración por hardware")
	}
	return softwareH264
}

// vaapiWorks hace un encode VAAPI diminuto a /dev/null: comprueba que el
// dispositivo y el codificador h264_vaapi funcionan con los permisos reales
// del proceso. Argumentos fijos, sin shell (CLAUDE.md).
func vaapiWorks(ffmpegPath string) bool {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	args := []string{"-v", "error", "-vaapi_device", vaapiDevice,
		"-f", "lavfi", "-i", "testsrc=duration=0.1:size=256x144:rate=5",
		"-vf", "format=nv12,hwupload", "-c:v", "h264_vaapi", "-f", "null", "-"}
	return exec.CommandContext(ctx, ffmpegPath, args...).Run() == nil
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
	encoder    videoEncoder // codificador de vídeo para transcode (ver SetVideoEncoder)

	mu       sync.Mutex
	sessions map[string]*Session
}

// NewManager crea el gestor de sesiones HLS. ctx debe vivir mientras viva la
// aplicación: al cancelarse, todas las sesiones activas matan su ffmpeg. El
// codificador de transcode arranca en software; main lo afina con
// SetVideoEncoder(DetectVideoEncoder(...)) tras detectar el hardware.
func NewManager(ctx context.Context, ffmpegPath, cacheDir string, log *slog.Logger) *Manager {
	return &Manager{ctx: ctx, ffmpegPath: ffmpegPath, cacheDir: cacheDir, log: log,
		encoder: softwareH264, sessions: map[string]*Session{}}
}

// SetVideoEncoder fija el codificador que usará el transcode. Pensado para
// llamarse una vez al arranque con el resultado de DetectVideoEncoder.
func (m *Manager) SetVideoEncoder(enc videoEncoder) {
	m.mu.Lock()
	m.encoder = enc
	m.mu.Unlock()
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
// audioTranscode recodifica el audio a AAC (-c:v copy -c:a aac) en vez de
// copiar ambos streams: caso de vídeo compatible con audio que no lo es
// (AC3/EAC3/DTS...), Brief M4 addendum §2. Sigue sin tocar el vídeo.
func (m *Manager) StartSession(requestCtx context.Context, sourceID int64, srcPath string, startSeconds float64, transcodeVideo, audioTranscode bool) (*Session, error) {
	if m.ffmpegPath == "" {
		return nil, ErrFFmpegUnavailable
	}

	m.mu.Lock()
	var ownSessions []*Session
	transcoding := 0
	for _, s := range m.sessions {
		if s.SourceID == sourceID {
			ownSessions = append(ownSessions, s)
		}
		if s.transcode {
			transcoding++
		}
	}
	if len(ownSessions) >= maxSessionsPerSource {
		sort.Slice(ownSessions, func(i, j int) bool { return ownSessions[i].createdAt.Before(ownSessions[j].createdAt) })
		if ownSessions[0].transcode {
			transcoding--
		}
		m.removeLocked(ownSessions[0])
	}
	if len(m.sessions) >= maxGlobalSessions || (transcodeVideo && transcoding >= maxTranscodeSessions) {
		m.mu.Unlock()
		return nil, ErrTooManySessions
	}
	enc := m.encoder
	m.mu.Unlock()

	id := newSessionID()
	dir := filepath.Join(m.cacheDir, id)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("crear sesión hls: %w", err)
	}

	procCtx, cancel := context.WithCancel(m.ctx)
	cmd := exec.CommandContext(procCtx, m.ffmpegPath, buildHLSArgs(enc, srcPath, dir, startSeconds, transcodeVideo, audioTranscode)...)

	if err := cmd.Start(); err != nil {
		cancel()
		os.RemoveAll(dir)
		return nil, fmt.Errorf("lanzar ffmpeg: %w", err)
	}

	sess := &Session{
		ID: id, SourceID: sourceID, Dir: dir, StartAt: startSeconds, transcode: transcodeVideo,
		cmd: cmd, cancel: cancel, createdAt: time.Now(), lastAccess: time.Now(),
		done: make(chan struct{}),
	}
	go func() {
		err := cmd.Wait()
		if err != nil && procCtx.Err() == nil && m.log != nil {
			m.log.Warn("stream: ffmpeg terminó con error", "session", id, "transcode", transcodeVideo, "err", err)
		}
		close(sess.done)
	}()

	m.mu.Lock()
	m.sessions[id] = sess
	m.mu.Unlock()

	// El transcode tarda más en escribir el primer segmento (recodifica, no
	// copia), así que le damos más margen que al remux.
	timeout := playlistReadyTimeout
	if transcodeVideo {
		timeout = transcodeReadyTimeout
	}
	if err := waitPlaylist(requestCtx, filepath.Join(dir, playlistName), timeout); err != nil {
		m.StopSession(id)
		return nil, fmt.Errorf("la conversión no generó la lista de reproducción a tiempo: %w", err)
	}
	return sess, nil
}

// buildHLSArgs construye los argumentos de ffmpeg para remux a HLS (Brief M4
// §4). Sin transcode de VÍDEO nunca (CLAUDE.md/Brief: "sin transcode de
// vídeo en MVP1"): audioTranscode recodifica solo el audio a AAC cuando el
// original no es reproducible en el navegador (Brief M4 addendum §2), coste
// de CPU bajo porque el vídeo se sigue copiando tal cual.
func buildHLSArgs(enc videoEncoder, srcPath, dir string, startSeconds float64, transcodeVideo, audioTranscode bool) []string {
	args := []string{"-y"}
	if transcodeVideo {
		// Opciones globales del codificador (p. ej. -vaapi_device) antes de -i.
		args = append(args, enc.preInput...)
	}
	if startSeconds > 0 {
		args = append(args, "-ss", strconv.FormatFloat(startSeconds, 'f', 3, 64))
	}
	// -sn: descarta los subtítulos. Con "-c copy" un subtítulo de texto
	// embebido (subrip/ass en MKV) llegaba al muxer HLS, que solo acepta
	// webvtt, y tumbaba la sesión entera aunque vídeo y audio fueran válidos.
	args = append(args, "-i", srcPath, "-sn")
	switch {
	case transcodeVideo:
		// Recodifica el vídeo a H.264 (el navegador no decodifica el original).
		args = append(args, enc.video...)
		if audioTranscode {
			args = append(args, "-c:a", "aac", "-b:a", "192k")
		} else {
			args = append(args, "-c:a", "copy")
		}
	case audioTranscode:
		args = append(args, "-c:v", "copy", "-c:a", "aac", "-b:a", "192k")
	default:
		args = append(args, "-c", "copy")
	}
	return append(args,
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

// StopSessionOf termina la sesión id solo si pertenece a sourceID. Devuelve
// si existía y se cerró. Es lo que usa el cliente al salir del reproductor
// (o cambiar de fuente): sin esto las sesiones solo se liberaban por
// inactividad (30 min) y tres reproducciones abandonadas dejaban al servidor
// respondiendo 503 hasta reiniciar.
func (m *Manager) StopSessionOf(id string, sourceID int64) bool {
	m.mu.Lock()
	sess, ok := m.sessions[id]
	if ok && sess.SourceID != sourceID {
		ok = false
	}
	if ok {
		delete(m.sessions, id)
	}
	m.mu.Unlock()
	if ok {
		m.stopProcess(sess)
	}
	return ok
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
