package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
	"unicode/utf8"

	"github.com/bigbendja/gala/internal/lan"
)

// SessionCookie es la cookie de sesión de administración.
const SessionCookie = "gala_session"

// CSRFHeader es la cabecera que deben llevar las escrituras hechas por sesión.
const CSRFHeader = "X-GALA-CSRF"

const (
	timeLayout = "2006-01-02T15:04:05Z"
	// touchEvery: last_seen y la caducidad deslizante se escriben como mucho
	// una vez cada tanto, para no escribir en BD en cada petición.
	touchEvery = 5 * time.Minute
	maxArgon   = 2 // verificaciones argon2 simultáneas (64 MB cada una)
)

var (
	ErrNotConfigured = errors.New("la administración no está configurada")
	ErrBadPassword   = errors.New("clave incorrecta")
	ErrNoSession     = errors.New("sin sesión")
)

// RateLimitedError indica que la IP está bloqueada temporalmente.
type RateLimitedError struct{ Wait time.Duration }

func (e *RateLimitedError) Error() string {
	return fmt.Sprintf("demasiados intentos, espera %d segundos", int(e.Wait.Seconds())+1)
}

// Settings es lo que el servicio necesita de gala.json.
type Settings interface {
	SessionDays() int
	AdminLocalhostBypass() bool
}

// Session es una sesión de administración guardada en BD.
type Session struct {
	IDHash    string
	CreatedAt time.Time
	ExpiresAt time.Time
	LastSeen  time.Time
	UserAgent string
	IP        string
}

// Service concentra clave, sesiones y CSRF. Las sesiones se validan SIEMPRE
// contra la BD (no hay caché): set-password o logout-all, ejecutados desde otro
// proceso (el CLI), cortan las sesiones del servidor vivo al instante.
type Service struct {
	db     *sql.DB
	cfg    Settings
	log    *slog.Logger
	now    func() time.Time
	lim    *limiter
	sem    chan struct{}
	secret []byte
}

// New crea el servicio, asegura el secreto del servidor y purga sesiones caducadas.
func New(db *sql.DB, cfg Settings, log *slog.Logger) (*Service, error) {
	s := &Service{db: db, cfg: cfg, log: log, now: time.Now, sem: make(chan struct{}, maxArgon)}
	s.lim = newLimiter(func() time.Time { return s.now() })
	if err := s.ensureSecret(context.Background()); err != nil {
		return nil, err
	}
	s.purgeExpired(context.Background())
	return s, nil
}

func (s *Service) ensureSecret(ctx context.Context) error {
	var b []byte
	err := s.db.QueryRowContext(ctx, `SELECT secret FROM server_secret WHERE id = 1`).Scan(&b)
	if err == nil && len(b) >= 32 {
		s.secret = b
		return nil
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("leer secreto: %w", err)
	}
	b = make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return fmt.Errorf("generar secreto: %w", err)
	}
	// OR IGNORE: dos procesos arrancando a la vez no pisan el secreto del otro.
	if _, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO server_secret(id, secret) VALUES (1, ?)`, b); err != nil {
		return fmt.Errorf("guardar secreto: %w", err)
	}
	if err := s.db.QueryRowContext(ctx, `SELECT secret FROM server_secret WHERE id = 1`).Scan(&b); err != nil {
		return fmt.Errorf("releer secreto: %w", err)
	}
	s.secret = b
	return nil
}

func (s *Service) ts(t time.Time) string { return t.UTC().Format(timeLayout) }

func parseTS(v string) time.Time {
	t, _ := time.Parse(timeLayout, v)
	return t
}

func (s *Service) ttl() time.Duration {
	d := s.cfg.SessionDays()
	if d <= 0 {
		d = 30
	}
	return time.Duration(d) * 24 * time.Hour
}

func (s *Service) purgeExpired(ctx context.Context) {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM admin_sessions WHERE expires_at <= ?`, s.ts(s.now())); err != nil {
		s.log.Warn("auth: purgar sesiones caducadas", "err", err)
	}
}

// --- clave -----------------------------------------------------------------

// Configured dice si ya hay una clave de administrador.
func (s *Service) Configured(ctx context.Context) (bool, error) {
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM admin_auth WHERE id = 1`).Scan(&n); err != nil {
		return false, fmt.Errorf("leer admin_auth: %w", err)
	}
	return n > 0, nil
}

// NetworkAdmin dice si se puede administrar desde la red (por defecto sí).
func (s *Service) NetworkAdmin(ctx context.Context) bool {
	var v int
	if err := s.db.QueryRowContext(ctx, `SELECT network_admin FROM admin_auth WHERE id = 1`).Scan(&v); err != nil {
		return true
	}
	return v != 0
}

// SetNetworkAdmin activa o desactiva la administración desde la red.
func (s *Service) SetNetworkAdmin(ctx context.Context, enabled bool) error {
	v := 0
	if enabled {
		v = 1
	}
	res, err := s.db.ExecContext(ctx, `UPDATE admin_auth SET network_admin = ?, updated_at = ? WHERE id = 1`, v, s.ts(s.now()))
	if err != nil {
		return fmt.Errorf("guardar network_admin: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotConfigured
	}
	return nil
}

func (s *Service) acquire(ctx context.Context) (release func(), err error) {
	select {
	case s.sem <- struct{}{}:
		return func() { <-s.sem }, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// SetPassword guarda la clave (primera vez o reseteo) y borra TODAS las
// sesiones en la misma transacción. Lo usan el CLI y Ajustes (solo localhost).
func (s *Service) SetPassword(ctx context.Context, pw string) error {
	return s.setPassword(ctx, pw, "")
}

func (s *Service) setPassword(ctx context.Context, pw, keepSession string) error {
	if err := ValidatePassword(pw); err != nil {
		return err
	}
	release, err := s.acquire(ctx)
	if err != nil {
		return err
	}
	hash, err := HashPassword(pw)
	release()
	if err != nil {
		return err
	}
	now := s.ts(s.now())
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("iniciar transacción: %w", err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `INSERT INTO admin_auth(id, password_hash, created_at, updated_at) VALUES (1, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET password_hash = excluded.password_hash, updated_at = excluded.updated_at`,
		hash, now, now); err != nil {
		return fmt.Errorf("guardar clave: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM admin_sessions WHERE id_hash <> ?`, keepSession); err != nil {
		return fmt.Errorf("borrar sesiones: %w", err)
	}
	return tx.Commit()
}

// verify comprueba la clave contra la BD (y re-hashea si los parámetros cambiaron).
func (s *Service) verify(ctx context.Context, pw string) error {
	var enc string
	err := s.db.QueryRowContext(ctx, `SELECT password_hash FROM admin_auth WHERE id = 1`).Scan(&enc)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrNotConfigured
	}
	if err != nil {
		return fmt.Errorf("leer clave: %w", err)
	}
	release, err := s.acquire(ctx)
	if err != nil {
		return err
	}
	ok, rehash, verr := VerifyPassword(pw, enc)
	release()
	if verr != nil {
		return fmt.Errorf("verificar clave: %w", verr)
	}
	if !ok {
		return ErrBadPassword
	}
	if rehash {
		if h, err := HashPassword(pw); err == nil {
			_, _ = s.db.ExecContext(ctx, `UPDATE admin_auth SET password_hash = ? WHERE id = 1`, h)
		}
	}
	return nil
}

// ChangePassword cambia la clave (exige la actual) y cierra el resto de sesiones.
// keepSession es el id_hash de la sesión desde la que se cambia ("" = ninguna).
// ip es la IP a la que se limitan los intentos fallidos ("" = localhost, sin límite).
func (s *Service) ChangePassword(ctx context.Context, current, next, keepSession, ip string) error {
	if err := ValidatePassword(next); err != nil {
		return err
	}
	if ip != "" {
		if ok, wait := s.lim.allow(ip); !ok {
			return &RateLimitedError{Wait: wait}
		}
	}
	if err := s.verify(ctx, current); err != nil {
		if errors.Is(err, ErrBadPassword) && ip != "" {
			s.lim.fail(ip)
		}
		return err
	}
	if ip != "" {
		s.lim.succeed(ip)
	}
	return s.setPassword(ctx, next, keepSession)
}

// --- sesiones ----------------------------------------------------------------

func hashToken(tok string) string {
	h := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(h[:])
}

// Login verifica la clave y crea una sesión. local = la petición viene del
// propio equipo: nunca se bloquea (vía de rescate). Devuelve el token (para la
// cookie) y el token CSRF.
func (s *Service) Login(ctx context.Context, pw, ip, ua string, local bool) (token, csrf string, err error) {
	if !local {
		if ok, wait := s.lim.allow(ip); !ok {
			s.log.Warn("auth: login bloqueado por demasiados intentos", "ip", ip)
			return "", "", &RateLimitedError{Wait: wait}
		}
	}
	if verr := s.verify(ctx, pw); verr != nil {
		if errors.Is(verr, ErrBadPassword) {
			if !local {
				s.lim.fail(ip)
			}
			s.log.Warn("auth: clave incorrecta", "ip", ip) // nunca la clave
		}
		return "", "", verr
	}
	if !local {
		s.lim.succeed(ip)
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generar token: %w", err)
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	id := hashToken(token)
	now := s.now()
	if _, err := s.db.ExecContext(ctx, `INSERT INTO admin_sessions(id_hash, created_at, expires_at, last_seen, user_agent, ip)
		VALUES (?, ?, ?, ?, ?, ?)`, id, s.ts(now), s.ts(now.Add(s.ttl())), s.ts(now), truncate(ua, 300), ip); err != nil {
		return "", "", fmt.Errorf("crear sesión: %w", err)
	}
	s.purgeExpired(ctx)
	s.log.Info("auth: sesión de administración iniciada", "ip", ip)
	return token, s.CSRFToken(id), nil
}

// truncate corta por runas (no por bytes) para no partir un carácter UTF-8.
func truncate(v string, n int) string {
	if utf8.RuneCountInString(v) <= n {
		return v
	}
	return string([]rune(v)[:n])
}

// Lookup busca la sesión de un token en la BD; ErrNoSession si no existe o caducó.
func (s *Service) Lookup(ctx context.Context, token string) (*Session, error) {
	if token == "" {
		return nil, ErrNoSession
	}
	id := hashToken(token)
	var created, expires, seen string
	sess := &Session{IDHash: id}
	err := s.db.QueryRowContext(ctx, `SELECT created_at, expires_at, last_seen, user_agent, ip
		FROM admin_sessions WHERE id_hash = ?`, id).Scan(&created, &expires, &seen, &sess.UserAgent, &sess.IP)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNoSession
	}
	if err != nil {
		return nil, fmt.Errorf("leer sesión: %w", err)
	}
	sess.CreatedAt, sess.ExpiresAt, sess.LastSeen = parseTS(created), parseTS(expires), parseTS(seen)
	if !s.now().Before(sess.ExpiresAt) {
		_, _ = s.db.ExecContext(ctx, `DELETE FROM admin_sessions WHERE id_hash = ?`, id)
		return nil, ErrNoSession
	}
	return sess, nil
}

// touch renueva la caducidad deslizante (como mucho una vez cada touchEvery).
func (s *Service) touch(ctx context.Context, sess *Session) {
	now := s.now()
	if now.Sub(sess.LastSeen) < touchEvery {
		return
	}
	if _, err := s.db.ExecContext(ctx, `UPDATE admin_sessions SET last_seen = ?, expires_at = ? WHERE id_hash = ?`,
		s.ts(now), s.ts(now.Add(s.ttl())), sess.IDHash); err != nil {
		s.log.Warn("auth: renovar sesión", "err", err)
	}
}

// SessionFromRequest devuelve la sesión de la cookie, o nil.
func (s *Service) SessionFromRequest(r *http.Request) *Session {
	c, err := r.Cookie(SessionCookie)
	if err != nil {
		return nil
	}
	sess, err := s.Lookup(r.Context(), c.Value)
	if err != nil {
		return nil
	}
	return sess
}

// Logout cierra una sesión; LogoutAll cierra todas.
func (s *Service) Logout(ctx context.Context, idHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM admin_sessions WHERE id_hash = ?`, idHash)
	return err
}

func (s *Service) LogoutAll(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM admin_sessions`)
	return err
}

// List devuelve las sesiones vivas, la más reciente primero.
func (s *Service) List(ctx context.Context) ([]Session, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id_hash, created_at, expires_at, last_seen, user_agent, ip
		FROM admin_sessions WHERE expires_at > ? ORDER BY last_seen DESC`, s.ts(s.now()))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Session{}
	for rows.Next() {
		var x Session
		var c, e, l string
		if err := rows.Scan(&x.IDHash, &c, &e, &l, &x.UserAgent, &x.IP); err != nil {
			return nil, err
		}
		x.CreatedAt, x.ExpiresAt, x.LastSeen = parseTS(c), parseTS(e), parseTS(l)
		out = append(out, x)
	}
	return out, rows.Err()
}

// --- CSRF ----------------------------------------------------------------------

// CSRFToken es HMAC(secreto, id de sesión): determinista, no se guarda.
func (s *Service) CSRFToken(idHash string) string {
	m := hmac.New(sha256.New, s.secret)
	m.Write([]byte(idHash))
	return hex.EncodeToString(m.Sum(nil))
}

func (s *Service) csrfOK(r *http.Request, sess *Session) bool {
	got := r.Header.Get(CSRFHeader)
	return got != "" && subtle.ConstantTimeCompare([]byte(got), []byte(s.CSRFToken(sess.IDHash))) == 1
}

// --- puerta de administración (lan.AdminGate) ------------------------------------

// LocalBypass: ¿localhost es admin sin sesión? Por defecto sí. Solo si
// gala.json lo desactiva Y ya hay clave: sin clave, localhost nunca se queda
// fuera (rescate).
func (s *Service) LocalBypass() bool {
	if s.cfg.AdminLocalhostBypass() {
		return true
	}
	ok, err := s.Configured(context.Background())
	return err != nil || !ok
}

// Check decide si una petición a una ruta de administración pasa: sesión
// válida (y CSRF en escrituras). Localhost sin bypass solo exige sesión.
func (s *Service) Check(r *http.Request) (ok bool, status int, code, msg string) {
	ctx := r.Context()
	configured, err := s.Configured(ctx)
	if err != nil {
		s.log.Error("auth: comprobar configuración", "err", err)
		return false, http.StatusInternalServerError, "internal", "No se pudo comprobar la sesión."
	}
	local := lan.IsLocalAdmin(r)
	if !configured {
		return false, http.StatusUnauthorized, "auth_not_configured",
			"La administración desde la red no está configurada. Pon una contraseña desde el equipo o por SSH."
	}
	if !local && !s.NetworkAdmin(ctx) {
		return false, http.StatusForbidden, "network_admin_disabled",
			"La administración desde la red está desactivada. Actívala desde el equipo."
	}
	sess := s.SessionFromRequest(r)
	if sess == nil {
		return false, http.StatusUnauthorized, "login_required", "Inicia sesión para administrar GALA."
	}
	switch r.Method {
	case http.MethodGet, http.MethodHead, http.MethodOptions:
	default:
		if !s.csrfOK(r, sess) {
			return false, http.StatusForbidden, "csrf_invalid", "La sesión no es válida para este cambio. Recarga la página."
		}
	}
	s.touch(ctx, sess)
	return true, 0, "", ""
}
