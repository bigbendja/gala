package api_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"testing"
)

// fakeHelperSource: mismo patrón que internal/media y internal/stream (ver
// esos paquetes para la justificación). Sirve tanto de ffprobe falso
// (HELPER_MODE=stdout) como de ffmpeg falso para HLS (HELPER_MODE=hls).
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
	case "hls":
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

// buildFakeBinary compila (una sola vez por proceso de test) el binario
// ffmpeg/ffprobe falso compartido por los tests de este paquete.
func buildFakeBinary(t *testing.T) string {
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
