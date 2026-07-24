package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// ErrNoFFmpeg distingue "no hay ffprobe disponible" de un error real de
// sondeo (archivo corrupto, etc.): el llamante decide por extensión en el
// primer caso (Brief M4 §2).
var ErrNoFFmpeg = errors.New("media: ffprobe no disponible")

// probeTimeout acota cada invocación de ffprobe (Brief M4 §2).
const probeTimeout = 10 * time.Second

// MediaInfo es el resultado de sondear un archivo de vídeo con ffprobe
// (Brief M4 §2).
type MediaInfo struct {
	Container  string // "mp4","matroska","avi",... (normalizado desde format_name)
	DurationS  float64
	VideoCodec string // "h264","hevc","vp9",... vacío si no hay pista de vídeo
	AudioCodec string // "aac","ac3","mp3",... vacío si no hay pista de audio
	Width      int
	Height     int
	Bitrate    int64
	ProbedAt   time.Time
}

type probeStream struct {
	CodecName string `json:"codec_name"`
	CodecType string `json:"codec_type"`
	Width     int    `json:"width"`
	Height    int    `json:"height"`
	Duration  string `json:"duration"`
}

type probeFormat struct {
	FormatName string `json:"format_name"`
	Duration   string `json:"duration"`
	BitRate    string `json:"bit_rate"`
}

type probeOutput struct {
	Streams []probeStream `json:"streams"`
	Format  probeFormat   `json:"format"`
}

// Probe ejecuta ffprobe sobre path y devuelve su MediaInfo. ffprobePath
// vacío (ffprobe no localizado) devuelve ErrNoFFmpeg de inmediato, sin
// intentar ejecutar nada (Brief M4 §1-§2). Argumentos construidos, sin
// shell: exec.CommandContext directo (CLAUDE.md).
func Probe(ctx context.Context, ffprobePath, path string) (*MediaInfo, error) {
	if strings.TrimSpace(ffprobePath) == "" {
		return nil, ErrNoFFmpeg
	}

	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, ffprobePath,
		"-v", "quiet",
		"-print_format", "json",
		"-show_format",
		"-show_streams",
		path,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return nil, fmt.Errorf("ffprobe: tiempo agotado: %w", ctx.Err())
		}
		return nil, fmt.Errorf("ffprobe: %w: %s", err, strings.TrimSpace(stderr.String()))
	}

	var out probeOutput
	if err := json.Unmarshal(stdout.Bytes(), &out); err != nil {
		return nil, fmt.Errorf("ffprobe: decodificar salida: %w", err)
	}

	mi := &MediaInfo{
		Container: normalizeContainer(out.Format.FormatName),
		DurationS: parseFloat(out.Format.Duration),
		Bitrate:   parseInt(out.Format.BitRate),
		ProbedAt:  time.Now().UTC(),
	}
	for _, st := range out.Streams {
		switch st.CodecType {
		case "video":
			if mi.VideoCodec == "" {
				mi.VideoCodec = st.CodecName
				mi.Width = st.Width
				mi.Height = st.Height
			}
		case "audio":
			if mi.AudioCodec == "" {
				mi.AudioCodec = st.CodecName
			}
		}
		if mi.DurationS == 0 {
			if d := parseFloat(st.Duration); d > mi.DurationS {
				mi.DurationS = d
			}
		}
	}
	return mi, nil
}

func parseFloat(s string) float64 {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return 0
	}
	return f
}

func parseInt(s string) int64 {
	n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}
	return n
}

// normalizeContainer reduce el format_name de ffprobe (a veces una lista de
// alias, p. ej. "mov,mp4,m4a,3gp,3g2,mj2") a un nombre corto informativo.
// La decisión direct/remux (media.Decide) NO usa este campo: se basa en la
// extensión del archivo, más fiable aquí porque format_name no distingue de
// forma fiable Matroska de WebM (ambos se reportan como "matroska,webm").
func normalizeContainer(formatName string) string {
	switch {
	case strings.Contains(formatName, "mp4"):
		return "mp4"
	case strings.Contains(formatName, "matroska"):
		return "matroska"
	case strings.Contains(formatName, "mpegts"):
		return "mpegts"
	case formatName == "":
		return ""
	default:
		first, _, _ := strings.Cut(formatName, ",")
		return first
	}
}
