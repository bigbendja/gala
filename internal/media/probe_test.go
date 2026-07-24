package media

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// --- unidad, sin ffmpeg: helpers internos de parseo -----------------------

func TestParseFloat(t *testing.T) {
	cases := map[string]float64{"10.5": 10.5, "  3 ": 3, "": 0, "n/a": 0, "0": 0}
	for in, want := range cases {
		if got := parseFloat(in); got != want {
			t.Errorf("parseFloat(%q) = %v, esperado %v", in, got, want)
		}
	}
}

func TestParseInt(t *testing.T) {
	cases := map[string]int64{"12345": 12345, "": 0, "n/a": 0}
	for in, want := range cases {
		if got := parseInt(in); got != want {
			t.Errorf("parseInt(%q) = %v, esperado %v", in, got, want)
		}
	}
}

func TestNormalizeContainer(t *testing.T) {
	cases := map[string]string{
		"mov,mp4,m4a,3gp,3g2,mj2": "mp4",
		"matroska,webm":           "matroska",
		"mpegts":                  "mpegts",
		"":                        "",
		"avi":                     "avi",
	}
	for in, want := range cases {
		if got := normalizeContainer(in); got != want {
			t.Errorf("normalizeContainer(%q) = %q, esperado %q", in, got, want)
		}
	}
}

// TestProbeNoFFprobePath: ffprobePath vacío devuelve ErrNoFFmpeg de
// inmediato, sin intentar ejecutar nada (Brief M4 §2).
func TestProbeNoFFprobePath(t *testing.T) {
	_, err := Probe(context.Background(), "", "cualquier-cosa.mp4")
	if !errors.Is(err, ErrNoFFmpeg) {
		t.Fatalf("err = %v, esperado ErrNoFFmpeg", err)
	}
}

// TestProbeParsesFakeFFprobeOutput sustituye ffprobe por un script/binario
// falso que imprime un JSON fijo, para probar el parseo de Probe() sin
// depender de tener ffmpeg real instalado (a diferencia de
// TestProbeAgainstGeneratedClips, que si necesita ffprobe real).
func TestProbeParsesFakeFFprobeOutput(t *testing.T) {
	fake := buildFakeFFprobe(t, `{
		"streams": [
			{"codec_type":"video","codec_name":"h264","width":1280,"height":720},
			{"codec_type":"audio","codec_name":"aac"}
		],
		"format": {"format_name":"mov,mp4,m4a,3gp,3g2,mj2","duration":"10.000000","bit_rate":"1500000"}
	}`, 0)

	info, err := Probe(context.Background(), fake, "irrelevante.mp4")
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if info.Container != "mp4" || info.VideoCodec != "h264" || info.AudioCodec != "aac" {
		t.Fatalf("info = %+v", info)
	}
	if info.DurationS != 10 || info.Width != 1280 || info.Height != 720 || info.Bitrate != 1500000 {
		t.Fatalf("info = %+v", info)
	}
}

// TestProbeFFprobeFails: ffprobe existe pero devuelve código de error (p.
// ej. archivo corrupto o formato desconocido): Probe debe propagar un error
// claro, no un MediaInfo vacío silencioso.
func TestProbeFFprobeFails(t *testing.T) {
	fake := buildFakeFFprobe(t, "", 1)
	_, err := Probe(context.Background(), fake, "corrupto.mkv")
	if err == nil {
		t.Fatal("err = nil, esperado un error (ffprobe salió con código != 0)")
	}
}

// TestProbeTimeout: un ffprobe que nunca termina debe cortarse por el
// timeout interno, no colgar la petición para siempre.
func TestProbeTimeout(t *testing.T) {
	if testing.Short() {
		t.Skip("lento: implica esperar el timeout de Probe")
	}
	fake := buildFakeFFprobeSleep(t, 30*time.Second)

	start := time.Now()
	_, err := probeWithTimeout(context.Background(), fake, "lo-que-sea.mkv", 500*time.Millisecond)
	if err == nil {
		t.Fatal("err = nil, esperado tiempo agotado")
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("Probe tardó %v, esperado cortarse mucho antes de los 30s del proceso falso", elapsed)
	}
}

// --- integración real: requiere ffprobe de verdad y los clips generados ---

// TestProbeAgainstGeneratedClips sondea los clips de
// scripts/make_testmedia.(ps1|sh) con el ffprobe REAL del sistema. Si no hay
// ffmpeg en el entorno (la CI no lo tiene) o los clips no se generaron
// todavía, se salta con un mensaje claro (Brief M4 §8).
func TestProbeAgainstGeneratedClips(t *testing.T) {
	ffprobePath, err := exec.LookPath("ffprobe")
	if err != nil {
		t.Skip("ffprobe no está en el PATH de este entorno: instálalo para probar internal/media/probe.go contra clips reales (ver README)")
	}
	mediaDir := filepath.Join("..", "..", "testdata", "media")
	clipPath := filepath.Join(mediaDir, "direct_h264_aac.mp4")
	if _, err := os.Stat(clipPath); err != nil {
		t.Skipf("falta %s: genera la matriz con scripts/make_testmedia.ps1 (o .sh) antes de correr este test", clipPath)
	}

	info, err := Probe(context.Background(), ffprobePath, clipPath)
	if err != nil {
		t.Fatalf("Probe: %v", err)
	}
	if info.VideoCodec != "h264" {
		t.Errorf("VideoCodec = %q, esperado h264", info.VideoCodec)
	}
	if info.AudioCodec != "aac" {
		t.Errorf("AudioCodec = %q, esperado aac", info.AudioCodec)
	}
	if info.Container != "mp4" {
		t.Errorf("Container = %q, esperado mp4", info.Container)
	}
	if info.DurationS < 9 || info.DurationS > 11 {
		t.Errorf("DurationS = %v, esperado ~10", info.DurationS)
	}
}

// --- helpers de ffprobe falso (binario real compilado, ver fakebin_test.go) ---

// buildFakeFFprobe devuelve la ruta de un "ffprobe" falso que imprime stdout
// y sale con exitCode. Probe() no fija cmd.Env, así que hereda el proceso de
// test (exec.Cmd con Env=nil usa os.Environ()): t.Setenv basta para pasarle
// el comportamiento deseado sin tocar la firma de Probe().
func buildFakeFFprobe(t *testing.T, stdout string, exitCode int) string {
	t.Helper()
	bin := buildFakeHelperBinary(t)
	t.Setenv("HELPER_MODE", "stdout")
	t.Setenv("HELPER_STDOUT", stdout)
	t.Setenv("HELPER_EXIT", strconv.Itoa(exitCode))
	return bin
}

// buildFakeFFprobeSleep devuelve la ruta de un "ffprobe" falso que se queda
// dormido dur antes de terminar (para probar el timeout de Probe).
func buildFakeFFprobeSleep(t *testing.T, dur time.Duration) string {
	t.Helper()
	bin := buildFakeHelperBinary(t)
	t.Setenv("HELPER_MODE", "sleep")
	t.Setenv("HELPER_SLEEP_MS", strconv.Itoa(int(dur.Milliseconds())))
	return bin
}

// probeWithTimeout es como Probe pero con un timeout inyectado, para no
// esperar los probeTimeout (10s) reales en el test.
func probeWithTimeout(ctx context.Context, ffprobePath, path string, timeout time.Duration) (*MediaInfo, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return Probe(ctx, ffprobePath, path)
}
