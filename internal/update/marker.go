// Package update implementa la actualización de GALA desde GitHub Releases:
// comprobar, descargar y verificar, intercambiar el binario con marcador a
// prueba de cortes, y confirmar o dar por fallido el arranque nuevo.
//
// El rollback lo decide el script scripts/gala-preflight (ExecStartPre de la
// unit systemd), que no depende de que el binario nuevo llegue a main(). Este
// paquete escribe lo que ese script lee: el marcador `update-pending` y los
// ficheros hermanos del binario. El formato es texto clave=valor a propósito:
// lo leen Go y sh.
package update

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Nombres de los ficheros hermanos del binario (viven en su mismo directorio).
const (
	PendingFile    = "update-pending"
	ResultFile     = "update-result"
	DBBackupFile   = "gala.db.pre-update"     // copia de la BD antes de la última actualización con éxito
	DBBackupNew    = "gala.db.pre-update.new" // copia de la actualización en curso
	NewSuffix      = ".new"
	PrevSuffix     = ".prev"
	StateReady     = "preparing" // marcador escrito ANTES de tocar el binario
	StateSwapped   = "swapped"   // los dos renombrados ya se hicieron
	ResultOK       = "ok"
	ResultRolledBk = "rolled_back"
)

// Marker es el estado de una actualización en curso.
type Marker struct {
	State             string
	From              string
	New               string
	Attempts          int
	MigracionesNuevas bool
}

func (m Marker) encode() string {
	mig := 0
	if m.MigracionesNuevas {
		mig = 1
	}
	return fmt.Sprintf("state=%s\nfrom_version=%s\nnew_version=%s\nattempts=%d\nmigraciones_nuevas=%d\n",
		m.State, clean(m.From), clean(m.New), m.Attempts, mig)
}

// clean evita que un valor rompa el formato clave=valor (saltos de línea).
func clean(v string) string {
	return strings.NewReplacer("\n", " ", "\r", " ").Replace(v)
}

// WriteMarker escribe el marcador de forma atómica y duradera (fichero
// temporal + fsync + rename + fsync del directorio): un corte de luz deja el
// marcador anterior o el nuevo, nunca uno a medias.
func WriteMarker(dir string, m Marker) error {
	return writeFileAtomic(filepath.Join(dir, PendingFile), []byte(m.encode()))
}

// ReadMarker lee el marcador; ok es false si no hay actualización en curso.
func ReadMarker(dir string) (m Marker, ok bool, err error) {
	kv, ok, err := readKV(filepath.Join(dir, PendingFile))
	if err != nil || !ok {
		return Marker{}, ok, err
	}
	m.State = kv["state"]
	m.From = kv["from_version"]
	m.New = kv["new_version"]
	m.Attempts, _ = strconv.Atoi(kv["attempts"])
	m.MigracionesNuevas = kv["migraciones_nuevas"] == "1"
	return m, true, nil
}

// Result es el resultado de la última actualización, que la UI enseña hasta
// que el administrador lo descarta.
type Result struct {
	Status string // ok | rolled_back
	From   string
	To     string
	Reason string
	At     time.Time
}

// WriteResult guarda el resultado de forma atómica.
func WriteResult(dir string, r Result) error {
	at := r.At
	if at.IsZero() {
		at = time.Now()
	}
	body := fmt.Sprintf("status=%s\nfrom_version=%s\nnew_version=%s\nreason=%s\nat=%s\n",
		clean(r.Status), clean(r.From), clean(r.To), clean(r.Reason), at.UTC().Format(time.RFC3339))
	return writeFileAtomic(filepath.Join(dir, ResultFile), []byte(body))
}

// ReadResult lee el resultado; ok es false si no hay ninguno pendiente de ver.
func ReadResult(dir string) (r Result, ok bool, err error) {
	kv, ok, err := readKV(filepath.Join(dir, ResultFile))
	if err != nil || !ok {
		return Result{}, ok, err
	}
	r = Result{Status: kv["status"], From: kv["from_version"], To: kv["new_version"], Reason: kv["reason"]}
	r.At, _ = time.Parse(time.RFC3339, kv["at"])
	return r, true, nil
}

// ClearResult descarta el resultado (el administrador ya lo vio).
func ClearResult(dir string) error {
	err := os.Remove(filepath.Join(dir, ResultFile))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func readKV(path string) (map[string]string, bool, error) {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	defer f.Close()
	kv := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, found := strings.Cut(strings.TrimRight(sc.Text(), "\r"), "=")
		if found {
			kv[strings.TrimSpace(k)] = v
		}
	}
	return kv, true, sc.Err()
}

// writeFileAtomic escribe path sin dejar nunca un fichero a medias.
func writeFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, filepath.Base(path)+".tmp-*")
	if err != nil {
		return fmt.Errorf("crear temporal: %w", err)
	}
	name := tmp.Name()
	cleanup := func() { _ = os.Remove(name) }
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("escribir: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		cleanup()
		return fmt.Errorf("sync: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanup()
		return err
	}
	if err := os.Rename(name, path); err != nil {
		cleanup()
		return fmt.Errorf("renombrar: %w", err)
	}
	syncDir(dir)
	return nil
}

// syncDir fuerza a disco la entrada de directorio (en Windows no aplica y se ignora).
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		d.Close()
	}
}
