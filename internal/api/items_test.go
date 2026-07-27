package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"strconv"
	"testing"

	"github.com/bigbendja/gala/internal/catalog"
)

// --- parent_id (Brief M10a-fix B1) -----------------------------------------

func TestHandleListItemsParentIDFiltersChildren(t *testing.T) {
	ps := newPlayTestServer(t, nil, nil)
	ctx := context.Background()

	seriesID, err := ps.repo.InsertItem(ctx, &catalog.Item{
		Kind: catalog.KindSeries, Title: "Ruta 3", Genres: []string{}, Confirmed: true,
	}, "")
	if err != nil {
		t.Fatalf("InsertItem serie: %v", err)
	}
	seasonID, err := ps.repo.InsertItem(ctx, &catalog.Item{
		Kind: catalog.KindSeason, ParentID: &seriesID, Title: "Temporada 1", Genres: []string{}, Confirmed: true,
	}, "")
	if err != nil {
		t.Fatalf("InsertItem temporada: %v", err)
	}
	ps.insertMovie(t, "Película Suelta")

	resp, err := http.Get(ps.URL + "/api/v1/items?parent_id=" + strconv.FormatInt(seriesID, 10))
	if err != nil {
		t.Fatalf("GET /items?parent_id=: %v", err)
	}
	defer resp.Body.Close()
	var res struct {
		Items []catalog.Item `json:"items"`
		Total int            `json:"total"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res.Total != 1 || res.Items[0].ID != seasonID {
		t.Fatalf("parent_id=%d = %+v, esperado solo la temporada", seriesID, res.Items)
	}

	respTop, err := http.Get(ps.URL + "/api/v1/items?parent_id=null")
	if err != nil {
		t.Fatalf("GET /items?parent_id=null: %v", err)
	}
	defer respTop.Body.Close()
	var resTop struct {
		Total int `json:"total"`
	}
	if err := json.NewDecoder(respTop.Body).Decode(&resTop); err != nil {
		t.Fatalf("decode top: %v", err)
	}
	if resTop.Total != 2 {
		t.Fatalf("parent_id=null total = %d, esperado 2 (serie + película, sin la temporada)", resTop.Total)
	}

	respBad, err := http.Get(ps.URL + "/api/v1/items?parent_id=abc")
	if err != nil {
		t.Fatalf("GET /items?parent_id=abc: %v", err)
	}
	defer respBad.Body.Close()
	if respBad.StatusCode != http.StatusBadRequest {
		t.Fatalf("parent_id no numérico = %d, esperado 400", respBad.StatusCode)
	}
}

// --- in_progress (Brief M10a-fix B3) ----------------------------------------

func TestHandleListItemsInProgress(t *testing.T) {
	ps := newPlayTestServer(t, nil, nil)
	itemID := ps.insertMovie(t, "En curso")

	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatalf("cookiejar.New: %v", err)
	}
	client := &http.Client{Jar: jar}

	// Sin cookie de dispositivo todavía: vacío, nunca error.
	respEmpty, err := http.Get(ps.URL + "/api/v1/items?in_progress=true")
	if err != nil {
		t.Fatalf("GET /items?in_progress= (sin cookie): %v", err)
	}
	defer respEmpty.Body.Close()
	var resEmpty struct {
		Total int `json:"total"`
	}
	if err := json.NewDecoder(respEmpty.Body).Decode(&resEmpty); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resEmpty.Total != 0 {
		t.Fatalf("in_progress sin cookie = %d, esperado 0", resEmpty.Total)
	}

	// Crea la cookie y progreso real vía PUT /playback (mismo flujo de M4).
	body, _ := json.Marshal(map[string]any{"position_s": 30.0, "duration_s": 100.0})
	putReq, _ := http.NewRequest(http.MethodPut, ps.URL+"/api/v1/playback/"+strconv.FormatInt(itemID, 10), bytes.NewReader(body))
	putReq.Header.Set("Content-Type", "application/json")
	putResp, err := client.Do(putReq)
	if err != nil {
		t.Fatalf("PUT /playback: %v", err)
	}
	putResp.Body.Close()
	if putResp.StatusCode != http.StatusOK {
		t.Fatalf("PUT /playback status = %d", putResp.StatusCode)
	}

	resp, err := client.Get(ps.URL + "/api/v1/items?in_progress=true")
	if err != nil {
		t.Fatalf("GET /items?in_progress=true: %v", err)
	}
	defer resp.Body.Close()
	var res struct {
		Items []catalog.Item `json:"items"`
		Total int            `json:"total"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if res.Total != 1 || res.Items[0].ID != itemID {
		t.Fatalf("in_progress = %+v, esperado solo %q", res.Items, "En curso")
	}
	if res.Items[0].Progress == nil || res.Items[0].Progress.PositionS != 30 {
		t.Fatalf("Progress = %+v, esperado position_s=30", res.Items[0].Progress)
	}
}

// --- deseos (Brief M10a-fix B2) ---------------------------------------------

func TestHandleWishAddAndRemove(t *testing.T) {
	ps := newPlayTestServer(t, nil, nil)
	itemID := ps.insertMovie(t, "El Último Tren")

	addResp, err := http.Post(ps.URL+"/api/v1/items/"+strconv.FormatInt(itemID, 10)+"/wish", "", nil)
	if err != nil {
		t.Fatalf("POST /wish: %v", err)
	}
	defer addResp.Body.Close()
	if addResp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(addResp.Body)
		t.Fatalf("status = %d, body=%s", addResp.StatusCode, data)
	}

	it, err := ps.repo.Get(context.Background(), itemID)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !it.Wished {
		t.Fatal("Wished = false tras POST /wish")
	}

	req, _ := http.NewRequest(http.MethodDelete, ps.URL+"/api/v1/items/"+strconv.FormatInt(itemID, 10)+"/wish", nil)
	delResp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /wish: %v", err)
	}
	defer delResp.Body.Close()
	if delResp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(delResp.Body)
		t.Fatalf("status = %d, body=%s", delResp.StatusCode, data)
	}

	it2, err := ps.repo.Get(context.Background(), itemID)
	if err != nil {
		t.Fatalf("Get (2): %v", err)
	}
	if it2.Wished {
		t.Fatal("Wished = true tras DELETE /wish")
	}
}

func TestHandleWishAddUnknownItem404(t *testing.T) {
	ps := newPlayTestServer(t, nil, nil)
	resp, err := http.Post(ps.URL+"/api/v1/items/999999/wish", "", nil)
	if err != nil {
		t.Fatalf("POST /wish: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, esperado 404", resp.StatusCode)
	}
}

// --- descartar pendiente (Brief M10c §3, cierra el hueco anotado en M10b) --

func TestHandleDeleteItem(t *testing.T) {
	ps := newPlayTestServer(t, nil, nil)
	itemID := ps.insertMovie(t, "Basura Sin Identificar")

	req, _ := http.NewRequest(http.MethodDelete, ps.URL+"/api/v1/items/"+strconv.FormatInt(itemID, 10), nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /items/{id}: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		data, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, body=%s", resp.StatusCode, data)
	}

	if _, err := ps.repo.Get(context.Background(), itemID); err == nil {
		t.Fatal("el item seguía existiendo tras DELETE")
	}
}

func TestHandleDeleteItemNotFound(t *testing.T) {
	ps := newPlayTestServer(t, nil, nil)
	req, _ := http.NewRequest(http.MethodDelete, ps.URL+"/api/v1/items/999999", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /items/{id}: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status = %d, esperado 404", resp.StatusCode)
	}
}

func TestHandleDeleteItemInvalidID(t *testing.T) {
	ps := newPlayTestServer(t, nil, nil)
	req, _ := http.NewRequest(http.MethodDelete, ps.URL+"/api/v1/items/abc", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("DELETE /items/{id}: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, esperado 400", resp.StatusCode)
	}
}
