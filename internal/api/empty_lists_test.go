package api_test

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// Con la BD vacía, todo endpoint que devuelve una lista debe serializar []
// y NUNCA null: el frontend recorre las listas sin comprobar, y un null en
// /pending tumbaba toda la app con una instalación nueva.
func TestEmptyDBListEndpointsReturnEmptyArrayNotNull(t *testing.T) {
	its := newIdentifyTestServer(t, nil)
	for _, path := range []string{"/api/v1/pending", "/api/v1/folders", "/api/v1/volumes", "/api/v1/ingest/lists"} {
		resp, err := http.Get(its.URL + path)
		if err != nil {
			t.Fatalf("GET %s: %v", path, err)
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("GET %s: status %d (%s)", path, resp.StatusCode, body)
			continue
		}
		if got := strings.TrimSpace(string(body)); got != "[]" {
			t.Errorf("GET %s con BD vacía = %q, esperado []", path, got)
		}
	}
}
