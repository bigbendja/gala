package update

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Estos tests ejecutan el script REAL scripts/gala-preflight (el que decide el
// rollback en el servidor) contra directorios temporales con un «gala» falso.

type pfDir struct {
	t      *testing.T
	dir    string
	script string
	sh     string
}

func newPF(t *testing.T) *pfDir {
	t.Helper()
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sin sh: el preflight es un script POSIX")
	}
	script, err := filepath.Abs(filepath.Join("..", "..", "scripts", "gala-preflight"))
	if err != nil {
		t.Fatal(err)
	}
	return &pfDir{t: t, dir: t.TempDir(), script: script, sh: sh}
}

// fakeBin escribe un «gala» que responde a --selfcheck con rc y la salida dada.
func (p *pfDir) fakeBin(path string, rc int, out string) {
	body := "#!/bin/sh\ncase \"$1\" in\n--selfcheck)\n\techo '" + out + "'\n\texit " + string(rune('0'+rc)) + "\n\t;;\nesac\n"
	if err := os.WriteFile(path, []byte(body), 0o755); err != nil {
		p.t.Fatal(err)
	}
}

func (p *pfDir) path(name string) string { return filepath.Join(p.dir, name) }

func (p *pfDir) write(name, content string) {
	if err := os.WriteFile(p.path(name), []byte(content), 0o644); err != nil {
		p.t.Fatal(err)
	}
}

func (p *pfDir) has(name string) bool {
	_, err := os.Stat(p.path(name))
	return err == nil
}

func (p *pfDir) text(name string) string {
	b, err := os.ReadFile(p.path(name))
	if err != nil {
		p.t.Fatalf("leer %s: %v", name, err)
	}
	return strings.ReplaceAll(string(b), "\r", "")
}

// run ejecuta el preflight y comprueba que SIEMPRE sale con 0.
func (p *pfDir) run() string {
	p.t.Helper()
	cmd := exec.Command(p.sh, p.script)
	cmd.Env = append(os.Environ(), "GALA_DIR="+p.dir, "GALA_SELFCHECK_TIMEOUT=10")
	out, err := cmd.CombinedOutput()
	if err != nil {
		p.t.Fatalf("el preflight debe salir siempre con 0: %v\n%s", err, out)
	}
	return string(out)
}

func (p *pfDir) marker(state string, attempts int, mig int) {
	p.write(PendingFile, "state="+state+"\nfrom_version=v0.9.0\nnew_version=v0.10.0\nattempts="+
		string(rune('0'+attempts))+"\nmigraciones_nuevas="+string(rune('0'+mig))+"\n")
}

func (p *pfDir) assertRolledBack(reasonPart string) {
	p.t.Helper()
	if p.has(PendingFile) {
		p.t.Error("el marcador debe borrarse")
	}
	if p.has("gala.new") {
		p.t.Error("no debe quedar .new")
	}
	res := p.text(ResultFile)
	if !strings.Contains(res, "status=rolled_back") || !strings.Contains(res, "from_version=v0.9.0") ||
		!strings.Contains(res, "new_version=v0.10.0") || !strings.Contains(res, reasonPart) {
		p.t.Errorf("resultado: %q (esperaba %q)", res, reasonPart)
	}
}

func TestPreflightSinMarcadorNoHaceNada(t *testing.T) {
	p := newPF(t)
	p.fakeBin(p.path("gala"), 0, "migraciones_aplicadas=0")
	p.run()
	if p.has(PendingFile) || p.has(ResultFile) {
		t.Fatal("sin actualización en curso no se crea nada")
	}
}

func TestPreflightArranqueSanoCuentaElIntentoYDejaElMarcador(t *testing.T) {
	p := newPF(t)
	p.fakeBin(p.path("gala"), 0, "migraciones_aplicadas=0")
	p.fakeBin(p.path("gala.prev"), 0, "x")
	p.marker(StateSwapped, 0, 0)
	p.run()
	m := p.text(PendingFile)
	if !strings.Contains(m, "attempts=1") || !strings.Contains(m, "state=swapped") || !strings.Contains(m, "migraciones_nuevas=0") {
		t.Fatalf("marcador: %q", m)
	}
	if !p.has("gala.prev") || p.has(ResultFile) {
		t.Fatal("un arranque sano no revierte ni anuncia nada: lo confirma gala")
	}
}

func TestPreflightRecuerdaQueHuboMigraciones(t *testing.T) {
	p := newPF(t)
	p.fakeBin(p.path("gala"), 0, "migraciones_aplicadas=2")
	p.fakeBin(p.path("gala.prev"), 0, "x")
	p.marker(StateSwapped, 0, 0)
	p.run()
	if m := p.text(PendingFile); !strings.Contains(m, "migraciones_nuevas=1") {
		t.Fatalf("debe anotar las migraciones: %q", m)
	}
	// En el reintento ya no aplica ninguna, pero la anotación es permanente.
	p.fakeBin(p.path("gala"), 0, "migraciones_aplicadas=0")
	p.run()
	if m := p.text(PendingFile); !strings.Contains(m, "migraciones_nuevas=1") || !strings.Contains(m, "attempts=2") {
		t.Fatalf("la anotación debe conservarse: %q", m)
	}
}

func TestPreflightAutocomprobacionFallidaRevierte(t *testing.T) {
	p := newPF(t)
	p.fakeBin(p.path("gala"), 1, "base de datos: rota")
	p.write("gala.prev", "#!/bin/sh\necho viejo\n")
	p.marker(StateSwapped, 0, 0)
	p.run()
	p.assertRolledBack("autocomprobación")
	if p.has("gala.prev") {
		t.Error("gala.prev se mueve a gala")
	}
	if !strings.Contains(p.text("gala"), "viejo") {
		t.Error("debe volver el binario anterior")
	}
}

func TestPreflightTrasTresArranquesFallidosRevierte(t *testing.T) {
	p := newPF(t)
	p.fakeBin(p.path("gala"), 0, "migraciones_aplicadas=0")
	p.write("gala.prev", "viejo")
	p.marker(StateSwapped, 3, 0) // 3 intentos ya gastados: el 4.º revierte
	p.run()
	p.assertRolledBack("3 intentos")
	if p.text("gala") != "viejo" {
		t.Error("debe volver el anterior")
	}
}

func TestPreflightRestauraLaBDSoloSiHuboMigraciones(t *testing.T) {
	for _, mig := range []int{0, 1} {
		p := newPF(t)
		p.fakeBin(p.path("gala"), 1, "mal")
		p.write("gala.prev", "viejo")
		p.write("gala.db", "BD-NUEVA-MIGRADA")
		p.write("gala.db-wal", "wal")
		p.write("gala.db-shm", "shm")
		p.write(DBBackupNew, "BD-ANTES-DE-ACTUALIZAR")
		p.write(DBBackupFile, "COPIA-DE-LA-ACTUALIZACION-ANTERIOR")
		p.marker(StateSwapped, 0, mig)
		p.run()
		p.assertRolledBack("falló")
		if mig == 1 {
			if p.text("gala.db") != "BD-ANTES-DE-ACTUALIZAR" {
				t.Errorf("con migraciones se restaura la copia: %q", p.text("gala.db"))
			}
			if p.has("gala.db-wal") || p.has("gala.db-shm") {
				t.Error("los ficheros WAL/SHM deben borrarse al restaurar la BD")
			}
		} else if p.text("gala.db") != "BD-NUEVA-MIGRADA" {
			t.Errorf("sin migraciones la BD no se toca: %q", p.text("gala.db"))
		}
		if p.has(DBBackupNew) {
			t.Error("la copia en curso se descarta")
		}
		if p.text(DBBackupFile) != "COPIA-DE-LA-ACTUALIZACION-ANTERIOR" {
			t.Error("la copia de la actualización anterior no se toca")
		}
	}
}

func TestPreflightCorteEntreLosDosRenombrados(t *testing.T) {
	p := newPF(t)
	// gala → gala.prev ya hecho; gala.new → gala no: no hay `gala`.
	p.write("gala.prev", "viejo")
	p.fakeBin(p.path("gala.new"), 0, "x")
	p.write(DBBackupNew, "copia")
	p.marker(StatePreparing(), 0, 0)
	p.run()
	p.assertRolledBack("interrumpió durante la instalación")
	if p.text("gala") != "viejo" || p.has("gala.prev") {
		t.Fatal("debe volver el binario anterior a su sitio")
	}
}

func TestPreflightCorteAntesDeIntercambiar(t *testing.T) {
	p := newPF(t)
	p.write("gala", "viejo")
	p.write("gala.new", "descargado")
	p.marker(StatePreparing(), 0, 0)
	p.run()
	p.assertRolledBack("antes de instalarse")
	if p.text("gala") != "viejo" {
		t.Fatal("sigue el binario de siempre")
	}
}

func TestPreflightIntercambioHechoPeroSinAnotarSeTrataComoNuevo(t *testing.T) {
	p := newPF(t)
	p.fakeBin(p.path("gala"), 0, "migraciones_aplicadas=1")
	p.write("gala.prev", "viejo")
	p.marker(StatePreparing(), 0, 0) // los dos renombrados se hicieron; faltó pasar a swapped
	p.run()
	m := p.text(PendingFile)
	if !strings.Contains(m, "state=swapped") || !strings.Contains(m, "attempts=1") || !strings.Contains(m, "migraciones_nuevas=1") {
		t.Fatalf("debe continuar como versión recién instalada: %q", m)
	}
}

// StatePreparing evita repetir el literal en los tests de arriba.
func StatePreparing() string { return StateReady }
