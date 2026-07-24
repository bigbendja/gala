package stream

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// fakeHelperSource: mismo patrón que internal/media/fakebin_test.go (paquete
// distinto, así que se define aquí también; ver ese archivo para la
// justificación de compilar un binario real en vez de reinventar un mock de
// exec.Cmd). Este "ffmpeg falso" simula `-f hls`: escribe una playlist y un
// segmento en las rutas que recibe por argumentos, con un retraso previo
// opcional (para probar la cancelación por contexto) y uno posterior (para
// mantener el proceso "vivo" el tiempo que haga falta el test).
const fakeHelperSource = `package main

import (
	"os"
	"strconv"
	"strings"
	"time"
)

func main() {
	if pre := os.Getenv("HELPER_PREDELAY_MS"); pre != "" {
		if ms, _ := strconv.Atoi(pre); ms > 0 {
			time.Sleep(time.Duration(ms) * time.Millisecond)
		}
	}
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
	if post := os.Getenv("HELPER_SLEEP_MS"); post != "" {
		if ms, _ := strconv.Atoi(post); ms > 0 {
			time.Sleep(time.Duration(ms) * time.Millisecond)
		}
	}
	os.Exit(0)
}
`

var (
	fakeHelperOnce sync.Once
	fakeHelperPath string
	fakeHelperErr  error
)

// buildFakeFFmpeg compila (una sola vez por proceso de test) el binario
// ffmpeg falso y lo configura vía variables de entorno (heredadas por
// exec.CommandContext, que no fija Env explícito). preDelayMS retrasa la
// escritura de la playlist (para probar cancelación); postDelayMS mantiene
// el proceso vivo después de escribirla.
func buildFakeFFmpeg(t *testing.T, preDelayMS, postDelayMS int) string {
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
			fakeHelperErr = fmt.Errorf("compilar binario ffmpeg falso: %w: %s", err, buildOut)
			return
		}
		fakeHelperPath = out
	})
	if fakeHelperErr != nil {
		t.Skipf("no se pudo compilar el binario ffmpeg falso (¿go no está en PATH?): %v", fakeHelperErr)
	}
	t.Setenv("HELPER_PREDELAY_MS", itoa(preDelayMS))
	t.Setenv("HELPER_SLEEP_MS", itoa(postDelayMS))
	return fakeHelperPath
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	if neg {
		b = append([]byte{'-'}, b...)
	}
	return string(b)
}
