package api_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/bigbendja/gala/internal/buildinfo"
)

func TestGetVersion(t *testing.T) {
	buildinfo.Version, buildinfo.Commit, buildinfo.Date = "v0.9.0", "abc1234", "2026-10-30T10:00:00Z"
	ts := newTestServer(t)
	resp, err := http.Get(ts.URL + "/api/v1/version")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	var got map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got["version"] != "v0.9.0" || got["commit"] != "abc1234" || got["date"] != "2026-10-30T10:00:00Z" {
		t.Errorf("respuesta inesperada: %v", got)
	}
}
