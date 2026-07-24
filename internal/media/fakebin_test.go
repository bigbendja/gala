package media

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// fakeHelperSource es un programa Go real y minúsculo que actúa como
// ffprobe/ffmpeg falso según variables de entorno (Brief M4 §8: "con
// ffmpeg fake si hace falta para CI"). Se compila una sola vez por proceso
// de test (buildFakeHelperBinary) y se reutiliza: es un ejecutable de
// verdad, así que exec.Command lo lanza igual que al ffmpeg/ffprobe real,
// sin necesitar shell ni scripts .bat/.sh específicos de plataforma.
const fakeHelperSource = `package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	switch os.Getenv("HELPER_MODE") {
	case "stdout":
		fmt.Print(os.Getenv("HELPER_STDOUT"))
		code, _ := strconv.Atoi(os.Getenv("HELPER_EXIT"))
		os.Exit(code)
	case "sleep":
		ms, _ := strconv.Atoi(os.Getenv("HELPER_SLEEP_MS"))
		time.Sleep(time.Duration(ms) * time.Millisecond)
		os.Exit(0)
	case "hls":
		// Simula ffmpeg -f hls: escribe una playlist + un segmento falsos en
		// las rutas que -hls_segment_filename y el último argumento indican.
		args := os.Args[1:]
		if len(args) == 0 {
			os.Exit(2)
		}
		playlist := args[len(args)-1]
		segPattern := ""
		for i, a := range args {
			if a == "-hls_segment_filename" && i+1 < len(args) {
				segPattern = args[i+1]
			}
		}
		_ = os.WriteFile(playlist, []byte("#EXTM3U\n#EXT-X-VERSION:3\n#EXTINF:6.0,\nseg00000.ts\n#EXT-X-ENDLIST\n"), 0o644)
		if segPattern != "" {
			segFile := strings.Replace(segPattern, "%05d", "00000", 1)
			_ = os.WriteFile(segFile, []byte("fake-ts-data"), 0o644)
		}
		ms, _ := strconv.Atoi(os.Getenv("HELPER_SLEEP_MS"))
		if ms > 0 {
			time.Sleep(time.Duration(ms) * time.Millisecond)
		}
		os.Exit(0)
	default:
		os.Exit(2)
	}
}
`

var (
	fakeHelperOnce sync.Once
	fakeHelperPath string
	fakeHelperErr  error
)

// buildFakeHelperBinary compila (una sola vez por proceso de test) el
// binario ffmpeg/ffprobe falso y devuelve su ruta. Salta el test (no falla)
// si no hay `go` disponible para compilarlo: no debería pasar en un entorno
// de desarrollo/CI de este proyecto, pero es un fallback honesto en vez de
// un pánico si algún día lo hay.
func buildFakeHelperBinary(t *testing.T) string {
	t.Helper()
	fakeHelperOnce.Do(func() {
		dir, err := os.MkdirTemp("", "gala-fakebin")
		if err != nil {
			fakeHelperErr = err
			return
		}
		src := filepath.Join(dir, "main.go")
		if err := os.WriteFile(src, []byte(fakeHelperSource), 0o644); err != nil {
			fakeHelperErr = err
			return
		}
		out := filepath.Join(dir, "fakebin")
		if runtime.GOOS == "windows" {
			out += ".exe"
		}
		cmd := exec.Command("go", "build", "-o", out, src)
		if buildOut, err := cmd.CombinedOutput(); err != nil {
			fakeHelperErr = fmt.Errorf("compilar binario falso: %w: %s", err, buildOut)
			return
		}
		fakeHelperPath = out
	})
	if fakeHelperErr != nil {
		t.Skipf("no se pudo compilar el binario ffmpeg/ffprobe falso (¿go no está en PATH?): %v", fakeHelperErr)
	}
	return fakeHelperPath
}
