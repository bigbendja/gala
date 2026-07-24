package media

import (
	"fmt"
	"strings"
)

// PlayMode es la decisión de reproducción para una fuente concreta.
type PlayMode string

const (
	ModeDirect PlayMode = "direct" // servir el archivo tal cual
	ModeRemux  PlayMode = "remux"  // ffmpeg -c copy → HLS
	ModeReject PlayMode = "reject" // no reproducible en el navegador
)

// compatibleVideoCodecs y compatibleAudioCodecs son los códecs que un
// navegador moderno reproduce de forma nativa (Brief M4 §3). Nombres tal
// como los reporta ffprobe (codec_name).
var (
	compatibleVideoCodecs = map[string]bool{"h264": true, "vp8": true, "vp9": true, "av1": true}
	compatibleAudioCodecs = map[string]bool{"aac": true, "mp3": true, "opus": true, "vorbis": true, "flac": true}

	// directContainers son los contenedores que un navegador reproduce sin
	// remux si los códecs también son compatibles (Brief M4 §3). Se comprueba
	// por EXTENSIÓN, no por el format_name de ffprobe (ver normalizeContainer
	// en probe.go: "matroska,webm" no distingue MKV de WebM de forma fiable).
	directExtensions = map[string]bool{"mp4": true, "m4v": true, "webm": true}
)

// Decision es el resultado de Decide: el modo y, si es Reject, un motivo en
// español listo para mostrar en la UI (Brief M4 §3).
type Decision struct {
	Mode   PlayMode
	Reason string // solo relevante si Mode == ModeReject
}

// Decide elige el modo de reproducción de una fuente (Brief M4 §3).
//
//   - info nil (sonda pendiente o sin ffmpeg): modo optimista, por extensión:
//     Direct para mp4/m4v/webm, Reject "no se pudo analizar" para el resto.
//   - vídeo o audio no compatibles: Reject, con motivo legible.
//   - contenedor directo (por extensión) y códecs compatibles: Direct.
//   - códecs compatibles pero contenedor no directo: Remux si hay ffmpeg,
//     si no Reject "falta ffmpeg".
func Decide(info *MediaInfo, ext string, ffmpegAvailable bool) Decision {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))

	if info == nil {
		if directExtensions[ext] {
			return Decision{Mode: ModeDirect}
		}
		return Decision{Mode: ModeReject, Reason: "no se pudo analizar el archivo"}
	}

	if info.VideoCodec == "" {
		return Decision{Mode: ModeReject, Reason: "no se detectó ninguna pista de vídeo reproducible"}
	}
	if info.AudioCodec == "" {
		return Decision{Mode: ModeReject, Reason: "no se detectó ninguna pista de audio reproducible"}
	}
	if !compatibleVideoCodecs[info.VideoCodec] {
		return Decision{Mode: ModeReject, Reason: fmt.Sprintf(
			"El vídeo usa %s, que este navegador no reproduce", strings.ToUpper(info.VideoCodec))}
	}
	// Audio incompatible con vídeo compatible sería resoluble transcodificando
	// solo el audio; fuera de MVP1 (solo remux -c copy, sin transcode).
	// Anotado en ISSUES-MVP2.md.
	if !compatibleAudioCodecs[info.AudioCodec] {
		return Decision{Mode: ModeReject, Reason: fmt.Sprintf(
			"El audio usa %s, que este navegador no reproduce", strings.ToUpper(info.AudioCodec))}
	}

	if directExtensions[ext] {
		return Decision{Mode: ModeDirect}
	}
	if !ffmpegAvailable {
		return Decision{Mode: ModeReject, Reason: "falta ffmpeg para convertir este contenedor"}
	}
	return Decision{Mode: ModeRemux}
}
