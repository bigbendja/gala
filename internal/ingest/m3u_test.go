package ingest

import "testing"

func TestParseM3UBasic(t *testing.T) {
	content := "#EXTM3U\n" +
		"#EXTINF:-1 tvg-name=\"BB\" group-title=\"Series\",Breaking Bad S01E01\n" +
		"http://x.com/bb1.mp4\n" +
		"#EXTINF:120,Corto\n" +
		"http://x.com/corto.mp4\n"
	entries := ParseM3U(content)
	if len(entries) != 2 {
		t.Fatalf("entries = %d, esperado 2", len(entries))
	}
	if entries[0].Title != "Breaking Bad S01E01" || entries[0].GroupTitle != "Series" || entries[0].TvgName != "BB" || entries[0].URL != "http://x.com/bb1.mp4" {
		t.Errorf("entries[0] = %+v", entries[0])
	}
	if entries[1].DurationS != 120 || entries[1].URL != "http://x.com/corto.mp4" {
		t.Errorf("entries[1] = %+v", entries[1])
	}
}

// TestParseM3UCRLFBlankLinesAndComments cubre robustez: CRLF, líneas en
// blanco, y directivas no soportadas (#EXTGRP) no deben romper el parseo.
func TestParseM3UCRLFBlankLinesAndComments(t *testing.T) {
	content := "#EXTM3U\r\n" +
		"\r\n" +
		"#EXTGRP:Acción\r\n" +
		"#EXTINF:-1,Peli Uno\r\n" +
		"\r\n" +
		"http://x.com/uno.mp4\r\n" +
		"#EXTINF:-1,Peli Dos\r\n" +
		"http://x.com/dos.mp4\r\n"
	entries := ParseM3U(content)
	if len(entries) != 2 {
		t.Fatalf("entries = %d, esperado 2: %+v", len(entries), entries)
	}
	if entries[0].Title != "Peli Uno" || entries[1].Title != "Peli Dos" {
		t.Errorf("entries = %+v", entries)
	}
}

// TestParseM3UGroupTitleWithComma: una coma DENTRO de un atributo
// entrecomillado no debe confundirse con el separador de título.
func TestParseM3UGroupTitleWithComma(t *testing.T) {
	content := `#EXTINF:-1 group-title="Acción, Drama",El Título Real` + "\nhttp://x.com/a.mp4\n"
	entries := ParseM3U(content)
	if len(entries) != 1 {
		t.Fatalf("entries = %d, esperado 1", len(entries))
	}
	if entries[0].GroupTitle != "Acción, Drama" || entries[0].Title != "El Título Real" {
		t.Errorf("entries[0] = %+v", entries[0])
	}
}

// TestParseM3UURLWithoutExtInf: una URL suelta sin EXTINF previo se admite
// igual (M3U "a pelo").
func TestParseM3UURLWithoutExtInf(t *testing.T) {
	entries := ParseM3U("#EXTM3U\nhttp://x.com/a.mp4\n")
	if len(entries) != 1 || entries[0].URL != "http://x.com/a.mp4" || entries[0].Title != "" {
		t.Fatalf("entries = %+v", entries)
	}
}

func TestParseM3UEmpty(t *testing.T) {
	if entries := ParseM3U(""); len(entries) != 0 {
		t.Errorf("entries = %+v, esperado vacío", entries)
	}
}

// TestParseM3UMixedSupportedAndUnsupported: una lista mixta con directos y
// un host no soportado (Brief M5 §7: el pipeline debe omitir el segundo sin
// tirar el parseo de los demás).
func TestParseM3UMixedSupportedAndUnsupported(t *testing.T) {
	content := "#EXTM3U\n" +
		"#EXTINF:-1,Directo Uno\nhttp://cdn.ejemplo.net/uno.mp4\n" +
		"#EXTINF:-1,Streamtape Embed\nhttps://streamtape.com/e/abc123\n" +
		"#EXTINF:-1,Directo Dos\nhttp://cdn.ejemplo.net/dos.mp4\n"
	entries := ParseM3U(content)
	if len(entries) != 3 {
		t.Fatalf("entries = %d, esperado 3 (el parser no filtra, eso lo hace el pipeline)", len(entries))
	}
	classes := make([]URLClass, len(entries))
	for i, e := range entries {
		c, err := ClassifyURL(e.URL)
		if err != nil {
			t.Fatalf("ClassifyURL(%q): %v", e.URL, err)
		}
		classes[i] = c
	}
	want := []URLClass{ClassDirect, ClassUnsupported, ClassDirect}
	for i, w := range want {
		if classes[i] != w {
			t.Errorf("classes[%d] = %v, esperado %v", i, classes[i], w)
		}
	}
}

func TestClassifyM3UContentChannelListVsSingleHLS(t *testing.T) {
	channelList := "#EXTM3U\n#EXTINF:-1,Canal Uno\nhttp://x.com/1.mp4\n#EXTINF:-1,Canal Dos\nhttp://x.com/2.mp4\n"
	if !classifyM3UContent(channelList) {
		t.Error("classifyM3UContent(lista de canales) = false, esperado true")
	}

	singleHLS := "#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:6\n#EXT-X-MEDIA-SEQUENCE:0\n" +
		"#EXTINF:6.0,\nseg0.ts\n#EXTINF:6.0,\nseg1.ts\n#EXT-X-ENDLIST\n"
	if classifyM3UContent(singleHLS) {
		t.Error("classifyM3UContent(HLS de un vídeo) = true, esperado false")
	}

	singleEntryNoRepeat := "#EXTM3U\n#EXTINF:-1,Única\nhttp://x.com/1.mp4\n"
	if classifyM3UContent(singleEntryNoRepeat) {
		t.Error("classifyM3UContent(una sola entrada) = true, esperado false")
	}
}
