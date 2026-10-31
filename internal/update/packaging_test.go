package update

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// scripts/install.sh lleva COPIAS de gala-preflight y de gala.service dentro
// (así el instalador es un único fichero que se copia al servidor). Si alguien
// edita el original y no la copia, el servidor instalaría una versión vieja.

func readScript(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "scripts", name))
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

// heredoc extrae el contenido entre «<<'MARCA'» y la línea MARCA.
func heredoc(t *testing.T, script, marker string) string {
	t.Helper()
	start := strings.Index(script, "<<'"+marker+"'\n")
	if start < 0 {
		t.Fatalf("install.sh no contiene el bloque %s", marker)
	}
	body := script[start+len("<<'"+marker+"'\n"):]
	end := strings.Index(body, "\n"+marker+"\n")
	if end < 0 {
		t.Fatalf("el bloque %s no se cierra", marker)
	}
	return body[:end]
}

func TestInstallShLlevaLaCopiaActualDelPreflight(t *testing.T) {
	install := readScript(t, "install.sh")
	want := strings.TrimRight(readScript(t, "gala-preflight"), "\n")
	if got := heredoc(t, install, "GALA_PREFLIGHT_EOF"); got != want {
		t.Fatal("la copia de gala-preflight dentro de scripts/install.sh no coincide con scripts/gala-preflight: " +
			"pega el contenido actual del script entre «<<'GALA_PREFLIGHT_EOF'» y «GALA_PREFLIGHT_EOF»")
	}
}

func TestInstallShLlevaLaCopiaActualDeLaUnit(t *testing.T) {
	install := readScript(t, "install.sh")
	want := strings.TrimRight(readScript(t, "gala.service"), "\n")
	if got := heredoc(t, install, "GALA_UNIT_EOF"); got != want {
		t.Fatal("la copia de gala.service dentro de scripts/install.sh no coincide con scripts/gala.service: " +
			"pega el contenido actual entre «<<'GALA_UNIT_EOF'» y «GALA_UNIT_EOF»")
	}
}

func TestUnitSystemdTieneLoQueExigeElDiseno(t *testing.T) {
	unit := readScript(t, "gala.service")
	for _, want := range []string{
		"StartLimitIntervalSec=0",               // que systemd no se rinda antes del rollback
		"ExecStartPre=/opt/gala/gala-preflight", // reconcilia/revierte antes de cada arranque
		"ExecStart=/opt/gala/gala --no-browser", // sin abrir navegador en un servidor
		"Restart=always",                        // el auto-update reinicia saliendo
		"NoNewPrivileges=true",                  // el reinicio no necesita sudo
		"User=gala",
		"WorkingDirectory=/opt/gala",
		"ReadWritePaths=/opt/gala", // ahí escribe el auto-update
	} {
		if !strings.Contains(unit, want) {
			t.Errorf("gala.service no contiene %q", want)
		}
	}
	if strings.Contains(unit, "sudo") {
		t.Error("el servicio no debe depender de sudo")
	}
}

func TestInstallShTieneSintaxisValida(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sin sh")
	}
	script, _ := filepath.Abs(filepath.Join("..", "..", "scripts", "install.sh"))
	if out, err := exec.Command(sh, "-n", script).CombinedOutput(); err != nil {
		t.Fatalf("sh -n: %v\n%s", err, out)
	}
}

func TestInstallShAyudaYArgumentosInvalidos(t *testing.T) {
	sh, err := exec.LookPath("sh")
	if err != nil {
		t.Skip("sin sh")
	}
	script, _ := filepath.Abs(filepath.Join("..", "..", "scripts", "install.sh"))
	out, err := exec.Command(sh, script, "--help").CombinedOutput()
	if err != nil || !strings.Contains(string(out), "--binary") || !strings.Contains(string(out), "--download") {
		t.Fatalf("--help: %v\n%s", err, out)
	}
	out, err = exec.Command(sh, script, "--no-existe").CombinedOutput()
	if err == nil || !strings.Contains(string(out), "opción desconocida") {
		t.Fatalf("una opción desconocida debe fallar con mensaje: %v\n%s", err, out)
	}
	out, err = exec.Command(sh, script, "--port", "abc").CombinedOutput()
	if err == nil || !strings.Contains(string(out), "--port debe ser un número") {
		t.Fatalf("puerto no numérico: %v\n%s", err, out)
	}
}

// /etc/os-release define VERSION, ID, NAME…: si se carga en el shell principal
// pisa la variable con la versión de GALA a descargar (así se rompió --download
// en v0.10.0: la URL llevaba «24.04.5 LTS (Noble Numbat)»).
func TestInstallShNoCargaOsReleaseEnElShellPrincipal(t *testing.T) {
	install := readScript(t, "install.sh")
	for _, line := range strings.Split(install, "\n") {
		trim := strings.TrimSpace(line)
		if strings.Contains(trim, "/etc/os-release") && strings.HasPrefix(trim, ". ") {
			t.Fatalf("«%s» carga os-release sin aislar: hazlo dentro de $( ... )", trim)
		}
	}
	if strings.Contains(install, "\nVERSION=") || strings.Contains(install, "\"$VERSION\"") {
		t.Fatal("la variable VERSION del instalador colisiona con os-release: usa GALA_VERSION")
	}
}
