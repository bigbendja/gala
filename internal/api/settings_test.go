package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"
)

func TestHandleGetTMDBSettingsReflectsConfigured(t *testing.T) {
	its := newIdentifyTestServer(t, nil)

	resp, err := http.Get(its.URL + "/api/v1/settings/tmdb")
	if err != nil {
		t.Fatalf("GET /settings/tmdb: %v", err)
	}
	defer resp.Body.Close()
	var got struct {
		Configured bool `json:"configured"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	// newIdentifyTestServer construye el *identify.Client de prueba con
	// "test-key", pero eso es independiente de config.Config.TMDBAPIKey
	// (que es lo que expone este endpoint): un config.NewAt(dir) fresco
	// empieza vacío.
	if got.Configured {
		t.Fatal("Configured = true, esperado false (gala.json de test empieza vacío)")
	}
}

func TestHandlePutTMDBSettingsValidKey(t *testing.T) {
	its := newIdentifyTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"results":[]}`))
	})

	body, _ := json.Marshal(map[string]string{"api_key": "clave-nueva-valida"})
	req, _ := http.NewRequest(http.MethodPut, its.URL+"/api/v1/settings/tmdb", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /settings/tmdb: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body=%s", resp.StatusCode, data)
	}

	data, err := os.ReadFile(filepath.Join(its.dir, "gala.json"))
	if err != nil {
		t.Fatalf("leer gala.json: %v", err)
	}
	var cfg struct {
		TMDBAPIKey string `json:"tmdb_api_key"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parsear gala.json: %v", err)
	}
	if cfg.TMDBAPIKey != "clave-nueva-valida" {
		t.Fatalf("tmdb_api_key persistida = %q, esperado %q", cfg.TMDBAPIKey, "clave-nueva-valida")
	}
}

func TestHandlePutTMDBSettingsInvalidKeyRejected(t *testing.T) {
	its := newIdentifyTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"status_code":7,"status_message":"Invalid API key: You must be granted a valid key.","success":false}`))
	})

	body, _ := json.Marshal(map[string]string{"api_key": "clave-mala"})
	req, _ := http.NewRequest(http.MethodPut, its.URL+"/api/v1/settings/tmdb", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /settings/tmdb: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, esperado 422, body=%s", resp.StatusCode, data)
	}

	// No debe haberse guardado la clave inválida. config.NewAt() (a
	// diferencia de Load()) no escribe gala.json hasta el primer Save(), así
	// que si la validación rechazó la clave ANTES de llamar a Save(), el
	// archivo ni siquiera debería existir todavía.
	data, err := os.ReadFile(filepath.Join(its.dir, "gala.json"))
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		t.Fatalf("leer gala.json: %v", err)
	}
	var cfg struct {
		TMDBAPIKey string `json:"tmdb_api_key"`
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		t.Fatalf("parsear gala.json: %v", err)
	}
	if cfg.TMDBAPIKey == "clave-mala" {
		t.Fatal("la clave inválida se guardó de todos modos")
	}
}

func TestHandlePutTMDBSettingsEmptyKeyRejected(t *testing.T) {
	its := newIdentifyTestServer(t, nil)
	req, _ := http.NewRequest(http.MethodPut, its.URL+"/api/v1/settings/tmdb", bytes.NewReader([]byte(`{"api_key":""}`)))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PUT /settings/tmdb: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, esperado 400", resp.StatusCode)
	}
}
