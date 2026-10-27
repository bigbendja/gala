package api_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/bigbendja/gala/internal/catalog"
)

func TestHomeSettingsDefaultAndToggle(t *testing.T) {
	e := newLanEnv(t)

	rec := e.call("GET", "/api/v1/settings/home", phone, "", nil)
	if rec.Code != 200 || rec.Body.String() != "{\"hero\":true}\n" {
		t.Fatalf("GET desde el móvil: %d %q (el hero está activo por defecto y lo lee cualquier dispositivo)", rec.Code, rec.Body)
	}

	// Solo el PC puede cambiarlo; el móvil recibe local_only.
	if rec := e.call("PUT", "/api/v1/settings/home", phone, `{"hero":false}`, nil); rec.Code != 403 {
		t.Fatalf("PUT desde el móvil = %d, esperado 403", rec.Code)
	}
	if !e.cfg.HomeHeroEnabled() {
		t.Fatal("el móvil consiguió ocultar el hero")
	}

	if rec := e.call("PUT", "/api/v1/settings/home", pc, `{"hero":false}`, nil); rec.Code != 200 {
		t.Fatalf("PUT desde el PC = %d: %s", rec.Code, rec.Body)
	}
	if e.cfg.HomeHeroEnabled() {
		t.Fatal("PUT no ocultó el hero")
	}
	rec = e.call("GET", "/api/v1/settings/home", pc, "", nil)
	var v struct{ Hero bool }
	json.Unmarshal(rec.Body.Bytes(), &v)
	if v.Hero {
		t.Errorf("GET tras ocultar = %s", rec.Body)
	}

	// Cuerpo sin "hero": 400, y no cambia nada.
	if rec := e.call("PUT", "/api/v1/settings/home", pc, `{}`, nil); rec.Code != 400 {
		t.Errorf("PUT sin hero = %d, esperado 400", rec.Code)
	}
	// Los demás ajustes siguen siendo del PC.
	if rec := e.call("GET", "/api/v1/settings/tmdb", phone, "", nil); rec.Code != 403 {
		t.Errorf("GET settings/tmdb desde el móvil = %d, esperado 403", rec.Code)
	}
}

func TestGenresEndpoint(t *testing.T) {
	e := newLanEnv(t)

	// BD vacía: [] (nunca null).
	rec := e.call("GET", "/api/v1/genres", phone, "", nil)
	if rec.Code != 200 || rec.Body.String() != "[]\n" {
		t.Fatalf("vacío: %d %q", rec.Code, rec.Body)
	}

	it := catalog.Item{Kind: catalog.KindSeries, Title: "Serie", Genres: []string{"Drama", "Documental"}, Confirmed: true}
	if _, err := e.repo.InsertItem(context.Background(), &it, ""); err != nil {
		t.Fatal(err)
	}
	rec = e.call("GET", "/api/v1/genres", phone, "", nil)
	var got []catalog.GenreCount
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || len(got) != 2 {
		t.Fatalf("géneros = %s (%v)", rec.Body, err)
	}
}

func TestListItemsAttachesSeriesTitleToEpisodes(t *testing.T) {
	e := newLanEnv(t)
	ctx := context.Background()
	prov := catalog.Item{Kind: catalog.KindOther, Title: "bb.s01e01", Genres: []string{}}
	id, _ := e.repo.InsertItem(ctx, &prov, "")
	e.repo.InsertSource(ctx, &catalog.Source{ItemID: id, Provider: "http", Locator: "https://a.example/1"}, "")
	if err := e.repo.ApplyEpisodeIdentification(ctx, id, 1396, "Breaking Bad", nil, nil, nil, nil,
		[]string{"Drama"}, 1, 1, "", "Piloto", ""); err != nil {
		t.Fatal(err)
	}

	rec := e.call("GET", "/api/v1/items?kind=episode", phone, "", nil)
	var res struct {
		Items []struct {
			Title       string `json:"title"`
			SeriesTitle string `json:"series_title"`
		} `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &res); err != nil || len(res.Items) != 1 {
		t.Fatalf("items = %s (%v)", rec.Body, err)
	}
	if res.Items[0].Title != "Piloto" || res.Items[0].SeriesTitle != "Breaking Bad" {
		t.Errorf("episodio = %+v", res.Items[0])
	}
}
