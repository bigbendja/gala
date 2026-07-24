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
// esos paquetes para la justificación). A diferencia de esos dos, aquí el
// MISMO binario puede necesitarse en los dos papeles dentro de un solo test
// (decideForSource sondea con "ffprobe" y luego StartSession remuxea con
// "ffmpeg"), así que el papel se detecta por los ARGUMENTOS recibidos
// (-show_streams/-print_format → ffprobe; el resto → ffmpeg -f hls) en vez
// de por una única variable de entorno global que no podría valer para
// ambos papeles a la vez.
const fakeHelperSource = `package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

func isProbeInvocation(args []string) bool {
	for _, a := range args {
		if a == "-show_streams" || a == "-print_format" {
			return true
		}
	}
	return false
}

func main() {
	args := os.Args[1:]
	if isProbeInvocation(args) {
		fmt.Print(os.Getenv("HELPER_STDOUT"))
		code, _ := strconv.Atoi(os.Getenv("HELPER_EXIT"))
		os.Exit(code)
	}

	// ffmpeg -f hls: escribe playlist + segmento, y vuelca los argumentos
	// recibidos en args.txt (junto a la playlist) para que el test pueda
	// comprobar si se pidió -c:a aac (AudioTranscode) o -c copy.
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
	dir := playlist[:strings.LastIndex(playlist, string(os.PathSeparator))]
	_ = os.WriteFile(dir+string(os.PathSeparator)+"args.txt", []byte(strings.Join(args, "\n")), 0o644)

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

// setFakeProbeResponse configura el JSON que el "ffprobe" falso devuelve en
// su próxima invocación (decideForSource lo llama antes de cada remux si
// falta media_info).
func setFakeProbeResponse(t *testing.T, videoCodec, audioCodec, formatName string) {
	t.Helper()
	t.Setenv("HELPER_STDOUT", fmt.Sprintf(
		`{"streams":[{"codec_type":"video","codec_name":%q},{"codec_type":"audio","codec_name":%q}],"format":{"format_name":%q,"duration":"10.0"}}`,
		videoCodec, audioCodec, formatName))
	t.Setenv("HELPER_EXIT", "0")
}
