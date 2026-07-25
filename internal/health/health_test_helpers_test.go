package health

import (
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/bigbendja/gala/internal/catalog"
	"github.com/bigbendja/gala/internal/store"
)

func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func newTestRepo(t *testing.T) *catalog.Repo {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("abrir store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return catalog.NewRepo(st.DB)
}
