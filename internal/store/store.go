// Package store gestiona la base de datos SQLite embebida y sus migraciones.
package store

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"os"
	"sort"

	_ "modernc.org/sqlite" // driver puro Go, sin CGo
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Store envuelve la conexión a la base de datos.
type Store struct {
	DB *sql.DB
}

// Open abre (o crea) la base de datos en path y aplica migraciones pendientes.
func Open(path string) (*Store, error) {
	// busy_timeout evita "database is locked" con accesos concurrentes desde LAN.
	dsn := fmt.Sprintf("file:%s?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)", path)
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("abrir sqlite: %w", err)
	}
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping sqlite: %w", err)
	}
	s := &Store{DB: db}
	if err := s.migrate(); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

// Close cierra la conexión.
func (s *Store) Close() error { return s.DB.Close() }

// migrate aplica en orden los .sql de migrations/ que no consten en schema_migrations.
func (s *Store) migrate() error {
	if _, err := s.DB.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY, applied_at TEXT NOT NULL DEFAULT (datetime('now'))
	)`); err != nil {
		return fmt.Errorf("crear schema_migrations: %w", err)
	}

	entries, err := fs.ReadDir(migrationsFS, "migrations")
	if err != nil {
		return fmt.Errorf("leer migraciones embebidas: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		names = append(names, e.Name())
	}
	sort.Strings(names)

	for _, name := range names {
		var done int
		err := s.DB.QueryRow(`SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, name).Scan(&done)
		if err != nil {
			return err
		}
		if done > 0 {
			continue
		}
		body, err := migrationsFS.ReadFile("migrations/" + name)
		if err != nil {
			return err
		}
		tx, err := s.DB.Begin()
		if err != nil {
			return err
		}
		if _, err := tx.Exec(string(body)); err != nil {
			tx.Rollback()
			return fmt.Errorf("migración %s: %w", name, err)
		}
		if _, err := tx.Exec(`INSERT INTO schema_migrations(version) VALUES (?)`, name); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

// MigrationCount cuenta las migraciones ya aplicadas en la BD de path sin
// abrirla con migrate (0 si el fichero o la tabla no existen). Lo usa
// `gala --selfcheck` para informar de cuántas aplicó el arranque nuevo.
func MigrationCount(path string) (int, error) {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}
	db, err := sql.Open("sqlite", fmt.Sprintf("file:%s?_pragma=busy_timeout(5000)", path))
	if err != nil {
		return 0, err
	}
	defer db.Close()
	return countMigrations(db), nil
}

// AppliedMigrations cuenta las migraciones aplicadas en una BD ya abierta.
func (s *Store) AppliedMigrations() int { return countMigrations(s.DB) }

func countMigrations(db *sql.DB) int {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&n); err != nil {
		return 0 // la tabla aún no existe
	}
	return n
}
