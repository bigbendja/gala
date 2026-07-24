package media

import (
	"fmt"
	"testing"
)

// TestDecideNoMediaInfoDirectExtensions: sin sonda, los contenedores
// directos siguen siendo optimistas independientemente de si hay ffmpeg
// (Brief M4 §3).
func TestDecideNoMediaInfoDirectExtensions(t *testing.T) {
	for _, ext := range []string{"mp4", ".mp4", "m4v", "webm"} {
		for _, ffmpegAvailable := range []bool{true, false} {
			got := Decide(nil, ext, ffmpegAvailable)
			if got.Mode != ModeDirect {
				t.Errorf("Decide(nil, %q, %v) = %v, esperado ModeDirect", ext, ffmpegAvailable, got.Mode)
			}
		}
	}
}

// TestDecideNoMediaInfoNonDirectExtensions cubre la corrección del addendum
// post-entrega: sin sonda todavía (o tras un fallo transitorio), un
// contenedor no directo NUNCA se presenta como rechazado si hay ffmpeg — es
// un estado transitorio (ModeAnalyzing), no una incompatibilidad. Solo sin
// ffmpeg (permanente, nada va a analizarlo nunca) es Reject de verdad.
func TestDecideNoMediaInfoNonDirectExtensions(t *testing.T) {
	for _, ext := range []string{"mkv", "avi", "ts", ""} {
		t.Run(ext+"/ffmpeg=true", func(t *testing.T) {
			got := Decide(nil, ext, true)
			if got.Mode != ModeAnalyzing {
				t.Fatalf("Decide(nil, %q, true) = %v, esperado ModeAnalyzing (nunca Reject por falta de análisis)", ext, got.Mode)
			}
			if got.Reason == "" {
				t.Error("ModeAnalyzing sin Reason: la UI necesita un texto de espera")
			}
		})
		t.Run(ext+"/ffmpeg=false", func(t *testing.T) {
			got := Decide(nil, ext, false)
			if got.Mode != ModeReject {
				t.Fatalf("Decide(nil, %q, false) = %v, esperado ModeReject (sin ffmpeg nunca se podrá analizar)", ext, got.Mode)
			}
			if got.Reason == "" {
				t.Error("Reject sin Reason")
			}
		})
	}
}

// TestDecideMatrix es la tabla exhaustiva contenedor × vídeo × audio ×
// ffmpeg disponible/no (Brief M4 §8, actualizada por el addendum: audio
// incompatible con vídeo compatible ya no es Reject, es Remux con
// AudioTranscode=true).
func TestDecideMatrix(t *testing.T) {
	videoCodecs := []string{"h264", "vp8", "vp9", "av1", "hevc", "mpeg2video", "wmv3"}
	audioCodecs := []string{"aac", "mp3", "opus", "vorbis", "flac", "ac3", "eac3", "dts"}
	containers := []string{"mp4", "mkv", "webm", "avi", "ts"}

	for _, container := range containers {
		for _, vcodec := range videoCodecs {
			for _, acodec := range audioCodecs {
				for _, ffmpegAvailable := range []bool{true, false} {
					name := fmt.Sprintf("%s/%s/%s/ffmpeg=%v", container, vcodec, acodec, ffmpegAvailable)
					t.Run(name, func(t *testing.T) {
						info := &MediaInfo{Container: container, VideoCodec: vcodec, AudioCodec: acodec}
						got := Decide(info, container, ffmpegAvailable)
						wantMode, wantAudioTranscode := expectedDecision(container, vcodec, acodec, ffmpegAvailable)
						if got.Mode != wantMode {
							t.Fatalf("Decide(%+v, %q, %v) = %v, esperado %v (reason=%q)",
								info, container, ffmpegAvailable, got.Mode, wantMode, got.Reason)
						}
						if got.AudioTranscode != wantAudioTranscode {
							t.Fatalf("AudioTranscode = %v, esperado %v", got.AudioTranscode, wantAudioTranscode)
						}
						if got.Mode == ModeReject && got.Reason == "" {
							t.Error("Reject sin Reason (la UI necesita un motivo legible)")
						}
						if got.Mode == ModeDirect && got.Reason != "" {
							t.Errorf("Reason = %q en modo Direct; no debería rellenarse", got.Reason)
						}
						if got.AudioTranscode && got.Mode != ModeRemux {
							t.Errorf("AudioTranscode=true fuera de ModeRemux (Mode=%v)", got.Mode)
						}
					})
				}
			}
		}
	}
}

// expectedDecision reimplementa las reglas del Brief M4 §3 (+ addendum) de
// forma independiente (sin llamar a Decide) para comparar contra el
// resultado real.
func expectedDecision(container, vcodec, acodec string, ffmpegAvailable bool) (PlayMode, bool) {
	videoOK := compatibleVideoCodecs[vcodec]
	audioOK := compatibleAudioCodecs[acodec]
	if !videoOK {
		return ModeReject, false
	}
	if directExtensions[container] && audioOK {
		return ModeDirect, false
	}
	// Vídeo compatible, y o el contenedor no es directo o el audio no lo es
	// (o ambos): hace falta ffmpeg, para remux del contenedor y/o
	// recodificar el audio.
	if !ffmpegAvailable {
		return ModeReject, false
	}
	return ModeRemux, !audioOK
}

// TestDecideMissingTracks cubre archivos sin pista de vídeo o de audio
// detectada: no debe auto-confirmarse silenciosamente ninguna de las dos.
func TestDecideMissingTracks(t *testing.T) {
	noVideo := &MediaInfo{Container: "mp4", VideoCodec: "", AudioCodec: "aac"}
	if got := Decide(noVideo, "mp4", true); got.Mode != ModeReject || got.Reason == "" {
		t.Fatalf("sin pista de vídeo: got %+v, esperado Reject con motivo", got)
	}

	noAudio := &MediaInfo{Container: "mp4", VideoCodec: "h264", AudioCodec: ""}
	if got := Decide(noAudio, "mp4", true); got.Mode != ModeReject || got.Reason == "" {
		t.Fatalf("sin pista de audio: got %+v, esperado Reject con motivo", got)
	}
}

// TestDecideReasonMentionsCodec: HEVC sigue siendo Reject (vídeo
// incompatible, sin remedio en MVP1); el ejemplo del Brief M4 §3.
func TestDecideReasonMentionsCodec(t *testing.T) {
	hevc := Decide(&MediaInfo{VideoCodec: "hevc", AudioCodec: "aac"}, "mkv", true)
	if hevc.Mode != ModeReject {
		t.Fatalf("HEVC debería ser Reject, got %v", hevc.Mode)
	}
	if !contains(hevc.Reason, "HEVC") {
		t.Errorf("Reason = %q, esperado que mencione HEVC", hevc.Reason)
	}
}

// TestDecideAudioTranscode cubre la corrección del addendum post-entrega:
// vídeo compatible + audio incompatible (AC3/EAC3/DTS) ya no es Reject,
// es Remux con recodificación de audio — tanto en un contenedor ya directo
// (mp4+ac3) como en uno que además necesitaba remux (mkv+ac3).
func TestDecideAudioTranscode(t *testing.T) {
	cases := []struct {
		name      string
		container string
	}{
		{"mkv (contenedor también necesitaba remux)", "mkv"},
		{"mp4 (contenedor ya era directo)", "mp4"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			info := &MediaInfo{Container: tc.container, VideoCodec: "h264", AudioCodec: "ac3"}
			got := Decide(info, tc.container, true)
			if got.Mode != ModeRemux {
				t.Fatalf("Mode = %v, esperado ModeRemux (audio recodificable, vídeo compatible)", got.Mode)
			}
			if !got.AudioTranscode {
				t.Error("AudioTranscode = false, esperado true (el audio no es reproducible tal cual)")
			}
		})
	}

	// Sin ffmpeg no hay forma de recodificar el audio: Reject.
	withoutFFmpeg := Decide(&MediaInfo{Container: "mp4", VideoCodec: "h264", AudioCodec: "ac3"}, "mp4", false)
	if withoutFFmpeg.Mode != ModeReject {
		t.Fatalf("sin ffmpeg = %v, esperado Reject", withoutFFmpeg.Mode)
	}
	if !contains(withoutFFmpeg.Reason, "ffmpeg") {
		t.Errorf("Reason = %q, esperado que mencione la falta de ffmpeg", withoutFFmpeg.Reason)
	}

	// Vídeo y audio compatibles: sigue sin recodificar nada (AudioTranscode=false).
	clean := Decide(&MediaInfo{Container: "mkv", VideoCodec: "h264", AudioCodec: "aac"}, "mkv", true)
	if clean.AudioTranscode {
		t.Error("AudioTranscode = true con audio ya compatible; no debería recodificarse")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (func() bool {
		for i := 0; i+len(substr) <= len(s); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	})()
}

// TestDecideNoFFmpegForcesReject: mismo archivo (contenedor no directo,
// códecs compatibles) decide distinto según haya ffmpeg o no.
func TestDecideNoFFmpegForcesReject(t *testing.T) {
	info := &MediaInfo{VideoCodec: "h264", AudioCodec: "aac"}
	withFFmpeg := Decide(info, "mkv", true)
	if withFFmpeg.Mode != ModeRemux {
		t.Fatalf("con ffmpeg = %v, esperado Remux", withFFmpeg.Mode)
	}
	withoutFFmpeg := Decide(info, "mkv", false)
	if withoutFFmpeg.Mode != ModeReject {
		t.Fatalf("sin ffmpeg = %v, esperado Reject", withoutFFmpeg.Mode)
	}
	if !contains(withoutFFmpeg.Reason, "ffmpeg") {
		t.Errorf("Reason = %q, esperado que mencione la falta de ffmpeg", withoutFFmpeg.Reason)
	}
}
