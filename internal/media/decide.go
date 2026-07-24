package media

import (
	"fmt"
	"strings"
)

// PlayMode es la decisión de reproducción para una fuente concreta.
type PlayMode string

const (
	ModeDirect    PlayMode = "direct"    // servir el archivo tal cual
	ModeRemux     PlayMode = "remux"     // ffmpeg -c copy (o -c:a aac) → HLS
	ModeReject    PlayMode = "reject"    // no reproducible en el navegador
	ModeAnalyzing PlayMode = "analyzing" // sonda pendiente o fallida: estado transitorio, NO un rechazo
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

// Decision es el resultado de Decide: el modo, un motivo en español listo
// para mostrar en la UI (Reject y Analyzing), y si el remux necesita
// recodificar el audio (Brief M4 addendum §2).
type Decision struct {
	Mode           PlayMode
	Reason         string // relevante en ModeReject y ModeAnalyzing
	AudioTranscode bool   // solo relevante si Mode == ModeRemux
}

// Decide elige el modo de reproducción de una fuente (Brief M4 §3, corregido
// por el addendum post-entrega tras la prueba real).
//
//   - info nil (sonda pendiente o fallida) con contenedor no directo: si hay
//     ffmpeg, ModeAnalyzing ("Analizando el archivo…") — NUNCA Reject solo
//     por falta de análisis, es un estado transitorio que se resuelve solo
//     (la cola de fondo o el siguiente /play lo reintentan). Sin ffmpeg no
//     hay nada que analizar nunca: Reject permanente.
//   - vídeo no compatible (hevc, mpeg2, wmv...): Reject, sin remedio en MVP1.
//   - audio no compatible (ac3, eac3, dts...) con vídeo SÍ compatible: ya NO
//     es Reject — Remux con AudioTranscode=true (-c:v copy -c:a aac), tanto
//     si el contenedor ya era directo como si no. Sin ffmpeg, Reject (no hay
//     forma de recodificar el audio).
//   - contenedor directo (por extensión) y códecs (ambos) compatibles: Direct.
//   - códecs compatibles pero contenedor no directo: Remux si hay ffmpeg,
//     si no Reject "falta ffmpeg".
func Decide(info *MediaInfo, ext string, ffmpegAvailable bool) Decision {
	ext = strings.ToLower(strings.TrimPrefix(ext, "."))

	if info == nil {
		if directExtensions[ext] {
			return Decision{Mode: ModeDirect}
		}
		if ffmpegAvailable {
			return Decision{Mode: ModeAnalyzing, Reason: "Analizando el archivo…"}
		}
		return Decision{Mode: ModeReject, Reason: "no se pudo analizar el archivo (ffmpeg no disponible)"}
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

	audioOK := compatibleAudioCodecs[info.AudioCodec]
	if directExtensions[ext] && audioOK {
		return Decision{Mode: ModeDirect}
	}

	// A partir de aquí hace falta ffmpeg: o el contenedor no es directo, o el
	// audio hay que recodificarlo (o ambos a la vez, p. ej. mkv+ac3).
	if !ffmpegAvailable {
		reason := "falta ffmpeg para convertir este contenedor"
		if !audioOK && directExtensions[ext] {
			reason = "falta ffmpeg para convertir el audio"
		}
		return Decision{Mode: ModeReject, Reason: reason}
	}
	return Decision{Mode: ModeRemux, AudioTranscode: !audioOK}
}
