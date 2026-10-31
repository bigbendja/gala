package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"

	"golang.org/x/term"

	"github.com/bigbendja/gala/internal/auth"
	"github.com/bigbendja/gala/internal/config"
	"github.com/bigbendja/gala/internal/store"
)

// runAdmin ejecuta los subcomandos de `gala admin ...` y devuelve el código de
// salida. No necesita que GALA esté parado: abre la misma BD (SQLite en WAL,
// con busy_timeout) y el servidor vivo valida cada sesión contra ella, así que
// un cambio de clave corta las sesiones al instante.
func runAdmin(args []string, stdin *os.File, stdout, stderr io.Writer) int {
	if len(args) == 0 || args[0] != "set-password" {
		fmt.Fprintln(stderr, "Uso: gala admin set-password")
		fmt.Fprintln(stderr, "  Pone o cambia la clave de administrador (y cierra todas las sesiones).")
		fmt.Fprintln(stderr, "  La clave se pide por el teclado (sin eco) o se lee de la entrada estándar;")
		fmt.Fprintln(stderr, "  nunca se pasa como argumento.")
		return 2
	}

	pw, err := readNewPassword(stdin, stdout)
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}

	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintln(stderr, "Error leyendo la configuración:", err)
		return 1
	}
	st, err := store.Open(cfg.DBPath())
	if err != nil {
		fmt.Fprintln(stderr, "Error abriendo la base de datos:", err)
		return 1
	}
	defer st.Close()
	svc, err := auth.New(st.DB, cfg, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		fmt.Fprintln(stderr, "Error:", err)
		return 1
	}
	if err := svc.SetPassword(context.Background(), pw); err != nil {
		if errors.Is(err, auth.ErrPasswordPolicy) {
			fmt.Fprintln(stderr, "La clave debe tener entre 8 y 128 caracteres.")
			return 1
		}
		fmt.Fprintln(stderr, "Error guardando la clave:", err)
		return 1
	}
	fmt.Fprintln(stdout, "Listo: la clave de administrador está puesta y todas las sesiones abiertas se han cerrado.")
	return 0
}

// readNewPassword pide la clave dos veces si hay teclado, o lee una línea de la
// entrada estándar si viene de una tubería.
func readNewPassword(stdin *os.File, out io.Writer) (string, error) {
	if term.IsTerminal(int(stdin.Fd())) {
		fmt.Fprint(out, "Nueva clave de administrador: ")
		a, err := term.ReadPassword(int(stdin.Fd()))
		fmt.Fprintln(out)
		if err != nil {
			return "", fmt.Errorf("leer la clave: %w", err)
		}
		fmt.Fprint(out, "Repite la clave: ")
		b, err := term.ReadPassword(int(stdin.Fd()))
		fmt.Fprintln(out)
		if err != nil {
			return "", fmt.Errorf("leer la clave: %w", err)
		}
		if string(a) != string(b) {
			return "", errors.New("las dos claves no coinciden")
		}
		return string(a), nil
	}
	line, err := bufio.NewReader(stdin).ReadString('\n')
	if err != nil && line == "" {
		return "", errors.New("no se recibió ninguna clave por la entrada estándar")
	}
	return strings.TrimRight(line, "\r\n"), nil
}
