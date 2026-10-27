package lan

import (
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"sync"
)

// Controller gestiona el listener TCP de un *http.Server para poder cambiar
// su bind (127.0.0.1 <-> 0.0.0.0) en caliento, sin tirar el proceso.
//
// Comportamiento documentado (Brief M7 §1): el http.Server es el MISMO antes
// y después; solo se cierra el listener viejo y se abre uno nuevo. Las
// conexiones ya establecidas del PC (p. ej. una reproducción en curso) no se
// cortan. Al DESACTIVAR el Modo LAN sí se cortan las conexiones de
// dispositivos remotos: "desactivar" tiene que dejar fuera al móvil ya.
type Controller struct {
	srv *http.Server
	log *slog.Logger

	mu    sync.Mutex
	ln    net.Listener
	bind  string
	port  int
	conns map[net.Conn]struct{}
}

// NewController envuelve srv. Debe crearse antes de la primera conexión.
func NewController(srv *http.Server, log *slog.Logger) *Controller {
	c := &Controller{srv: srv, log: log, conns: map[net.Conn]struct{}{}}
	prev := srv.ConnState
	srv.ConnState = func(conn net.Conn, st http.ConnState) {
		c.mu.Lock()
		switch st {
		case http.StateNew:
			c.conns[conn] = struct{}{}
		case http.StateClosed, http.StateHijacked:
			delete(c.conns, conn)
		}
		c.mu.Unlock()
		if prev != nil {
			prev(conn, st)
		}
	}
	return c
}

// Listen (re)abre el listener en bind:port. Si ya había uno, lo cierra
// primero (en Windows no pueden convivir 0.0.0.0:p y 127.0.0.1:p). Si el
// nuevo falla, intenta restaurar el anterior y devuelve el error.
func (c *Controller) Listen(bind string, port int) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	oldBind, oldPort := c.bind, c.port
	hadOld := c.ln != nil
	if c.ln != nil {
		_ = c.ln.Close()
		c.ln = nil
	}

	ln, err := net.Listen("tcp", net.JoinHostPort(bind, strconv.Itoa(port)))
	if err != nil {
		if hadOld {
			if back, berr := net.Listen("tcp", net.JoinHostPort(oldBind, strconv.Itoa(oldPort))); berr == nil {
				c.attach(back, oldBind, oldPort)
			} else {
				c.log.Error("no se pudo restaurar el listener anterior", "err", berr)
			}
		}
		return fmt.Errorf("escuchar en %s:%d: %w", bind, port, err)
	}
	c.attach(ln, bind, port)

	if !isOpenBind(bind) {
		c.dropRemoteLocked()
	}
	return nil
}

func (c *Controller) attach(ln net.Listener, bind string, port int) {
	c.ln, c.bind = ln, bind
	// Con port 0 el SO elige uno: guardar el real.
	if tcp, ok := ln.Addr().(*net.TCPAddr); ok {
		port = tcp.Port
	}
	c.port = port
	go func() {
		err := c.srv.Serve(ln)
		if err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
			c.log.Error("servidor http", "err", err)
		}
	}()
}

// dropRemoteLocked cierra las conexiones que no vienen del propio PC.
func (c *Controller) dropRemoteLocked() {
	for conn := range c.conns {
		if !IsLoopbackAddr(conn.RemoteAddr()) {
			_ = conn.Close()
			delete(c.conns, conn)
		}
	}
}

// Addr devuelve la dirección en la que se escucha ahora ("" si ninguna).
func (c *Controller) Addr() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ln == nil {
		return ""
	}
	return c.ln.Addr().String()
}

// Close cierra el listener (no el http.Server: eso lo hace su Shutdown).
func (c *Controller) Close() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ln != nil {
		_ = c.ln.Close()
		c.ln = nil
	}
}

func isOpenBind(bind string) bool {
	ip := net.ParseIP(bind)
	return bind == "" || (ip != nil && !ip.IsLoopback())
}

// IsLoopbackAddr indica si addr es 127.0.0.0/8 o ::1.
func IsLoopbackAddr(addr net.Addr) bool {
	if addr == nil {
		return false
	}
	return IsLoopbackHostPort(addr.String())
}

// IsLoopbackHostPort indica si "host:puerto" (o solo host) es loopback.
func IsLoopbackHostPort(hostport string) bool {
	host, _, err := net.SplitHostPort(hostport)
	if err != nil {
		host = hostport
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
