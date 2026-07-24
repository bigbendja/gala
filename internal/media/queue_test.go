package media

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/bigbendja/gala/internal/catalog"
	"github.com/bigbendja/gala/internal/providers/localfs"
	"github.com/bigbendja/gala/internal/store"
)

func newRepoForTest(t *testing.T) *catalog.Repo {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("abrir store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return catalog.NewRepo(st.DB)
}

func insertLocalfsSource(t *testing.T, ctx context.Context, r *catalog.Repo, title, locator string) int64 {
	t.Helper()
	item := catalog.Item{Kind: catalog.KindOther, Title: title, Genres: []string{}, Confirmed: true}
	itemID, err := r.InsertItem(ctx, &item, "")
	if err != nil {
		t.Fatalf("InsertItem: %v", err)
	}
	src := catalog.Source{ItemID: itemID, Provider: localfs.ProviderID, Locator: locator}
	srcID, err := r.InsertSource(ctx, &src, "")
	if err != nil {
		t.Fatalf("InsertSource: %v", err)
	}
	return srcID
}

// TestQueueDrainNoFFmpegIsNoop cubre "nunca abortar": sin ffmpeg
// disponible, drain() no debe fallar ni marcar nada, solo no hacer nada
// (Brief M4 §1-§2).
func TestQueueDrainNoFFmpegIsNoop(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForTest(t)
	insertLocalfsSource(t, ctx, repo, "peli", "no-existe.mp4")

	q := NewQueue(ctx, repo, &FFmpeg{}, discardLogger()) // FFmpeg vacío: Available()=false
	q.drain()

	if q.Status().Processed != 0 {
		t.Errorf("Processed = %d, esperado 0 (sin ffmpeg no se sondea nada)", q.Status().Processed)
	}
}

// TestQueueDrainProbesAndStores usa el ffprobe falso para comprobar que
// drain() sonda las fuentes pendientes y persiste media_info.
func TestQueueDrainProbesAndStores(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForTest(t)
	srcID := insertLocalfsSource(t, ctx, repo, "peli", "video.mp4")

	fakeProbe := buildFakeFFprobe(t, `{
		"streams": [
			{"codec_type":"video","codec_name":"h264","width":1920,"height":1080},
			{"codec_type":"audio","codec_name":"aac"}
		],
		"format": {"format_name":"mov,mp4,m4a,3gp,3g2,mj2","duration":"42.0","bit_rate":"900000"}
	}`, 0)
	ffmpeg := &FFmpeg{FFmpegPath: fakeProbe, FFprobePath: fakeProbe}

	q := NewQueue(ctx, repo, ffmpeg, discardLogger())
	q.drain()

	if got := q.Status().Processed; got != 1 {
		t.Fatalf("Processed = %d, esperado 1", got)
	}
	mi, ok, err := repo.GetMediaInfo(ctx, srcID)
	if err != nil {
		t.Fatalf("GetMediaInfo: %v", err)
	}
	if !ok {
		t.Fatal("GetMediaInfo: no se guardó media_info tras drain()")
	}
	if mi.VideoCodec != "h264" || mi.AudioCodec != "aac" || mi.DurationS != 42 {
		t.Fatalf("media_info = %+v", mi)
	}

	// Segunda pasada: ya sondeada, no debe volver a procesarla.
	q.drain()
	if got := q.Status().Processed; got != 1 {
		t.Errorf("Processed tras segundo drain = %d, esperado seguir en 1 (ya sondeada)", got)
	}
}

// TestQueueDrainSkipsUnreadableFile: un archivo que no se puede sondear
// (ffprobe falla) no debe tirar el resto del drenado ni quedar en un bucle;
// simplemente se registra el aviso y se sigue.
func TestQueueDrainSkipsUnreadableFile(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForTest(t)
	insertLocalfsSource(t, ctx, repo, "corrupto", "corrupto.mkv")

	fake := buildFakeFFprobe(t, "", 1) // ffprobe falla
	ffmpeg := &FFmpeg{FFmpegPath: fake, FFprobePath: fake}

	q := NewQueue(ctx, repo, ffmpeg, discardLogger())
	q.drain() // no debe entrar en pánico ni colgarse

	if got := q.Status().Processed; got != 1 {
		t.Errorf("Processed = %d, esperado 1 (se cuenta el intento aunque falle)", got)
	}
}

// TestQueueEnqueueDoesNotBlock: igual que identify.Queue, el canal de señal
// tiene buffer 1 y no debe bloquear ni acumular.
func TestQueueEnqueueDoesNotBlock(t *testing.T) {
	ctx := context.Background()
	repo := newRepoForTest(t)
	q := NewQueue(ctx, repo, &FFmpeg{}, discardLogger())

	done := make(chan struct{})
	go func() {
		for i := 0; i < 10; i++ {
			q.Enqueue()
		}
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("Enqueue se bloqueó")
	}
}
