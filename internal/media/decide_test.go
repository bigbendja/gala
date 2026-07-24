package media

import (
	"fmt"
	"testing"
)

// TestDecideNoMediaInfo cubre el caso "sonda pendiente o sin ffmpeg": modo
// optimista por extensión (Brief M4 §3).
func TestDecideNoMediaInfo(t *testing.T) {
	cases := []struct {
		ext      string
		wantMode PlayMode
	}{
		{"mp4", ModeDirect},
		{".mp4", ModeDirect}, // con o sin punto, indiferente
		{"m4v", ModeDirect},
		{"webm", ModeDirect},
		{"mkv", ModeReject},
		{"avi", ModeReject},
		{"ts", ModeReject},
		{"", ModeReject},
	}
	for _, tc := range cases {
		t.Run(tc.ext, func(t *testing.T) {
			got := Decide(nil, tc.ext, true)
			if got.Mode != tc.wantMode {
				t.Fatalf("Decide(nil, %q, true) = %v, esperado %v", tc.ext, got.Mode, tc.wantMode)
			}
			if got.Mode == ModeReject && got.Reason == "" {
				t.Error("Reject sin Reason")
			}
		})
	}
}

// TestDecideMatrix es la tabla exhaustiva contenedor × vídeo × audio ×
// ffmpeg disponible/no pedida por el Brief M4 §8.
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
						want := expectedMode(container, vcodec, acodec, ffmpegAvailable)
						if got.Mode != want {
							t.Fatalf("Decide(%+v, %q, %v) = %v, esperado %v (reason=%q)",
								info, container, ffmpegAvailable, got.Mode, want, got.Reason)
						}
						if got.Mode == ModeReject && got.Reason == "" {
							t.Error("Reject sin Reason (la UI necesita un motivo legible)")
						}
						if got.Mode != ModeReject && got.Reason != "" {
							t.Errorf("Reason = %q en modo %v; solo debería rellenarse en Reject", got.Reason, got.Mode)
						}
					})
				}
			}
		}
	}
}

// expectedMode reimplementa las reglas del Brief M4 §3 de forma
// independiente (sin llamar a Decide) para comparar contra el resultado
// real: vídeo/audio compatibles, contenedor directo por extensión, remux si
// hay ffmpeg, reject si no.
func expectedMode(container, vcodec, acodec string, ffmpegAvailable bool) PlayMode {
	videoOK := compatibleVideoCodecs[vcodec]
	audioOK := compatibleAudioCodecs[acodec]
	if !videoOK || !audioOK {
		return ModeReject
	}
	if directExtensions[container] {
		return ModeDirect
	}
	if !ffmpegAvailable {
		return ModeReject
	}
	return ModeRemux
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

// TestDecideReasonMentionsCodec: los ejemplos exactos del Brief M4 §3.
func TestDecideReasonMentionsCodec(t *testing.T) {
	hevc := Decide(&MediaInfo{VideoCodec: "hevc", AudioCodec: "aac"}, "mkv", true)
	if hevc.Mode != ModeReject {
		t.Fatalf("HEVC debería ser Reject, got %v", hevc.Mode)
	}
	if !contains(hevc.Reason, "HEVC") {
		t.Errorf("Reason = %q, esperado que mencione HEVC", hevc.Reason)
	}

	ac3 := Decide(&MediaInfo{VideoCodec: "h264", AudioCodec: "ac3"}, "mkv", true)
	if ac3.Mode != ModeReject {
		t.Fatalf("AC3 debería ser Reject, got %v", ac3.Mode)
	}
	if !contains(ac3.Reason, "AC3") {
		t.Errorf("Reason = %q, esperado que mencione AC3", ac3.Reason)
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
