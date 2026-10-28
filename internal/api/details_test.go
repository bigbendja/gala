package api_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/bigbendja/gala/internal/catalog"
)

func seedDetailed(t *testing.T, e *lanEnv) (itemID, similarInLib int64) {
	t.Helper()
	ctx := context.Background()
	tmdb := int64(27205)
	it := catalog.Item{Kind: catalog.KindMovie, Title: "Origen", TMDBID: &tmdb, Genres: []string{}, Confirmed: true, IdentifyStatus: catalog.IdentifyOK}
	id, err := e.repo.InsertItem(ctx, &it, "")
	if err != nil {
		t.Fatal(err)
	}
	srcID, err := e.repo.InsertSource(ctx, &catalog.Source{ItemID: id, Provider: "localfs", Locator: `D:\Privado\Origen.mkv`}, "")
	if err != nil {
		t.Fatal(err)
	}
	if err := e.repo.UpsertMediaInfo(ctx, catalog.MediaInfo{SourceID: srcID, Container: "matroska", VideoCodec: "h264", AudioCodec: "aac", Width: 1920, Height: 1080}); err != nil {
		t.Fatal(err)
	}
	// Un similar que SÍ está en la biblioteca (con fuente) y otro que no.
	other := int64(157336)
	oi := catalog.Item{Kind: catalog.KindMovie, Title: "Interstellar", TMDBID: &other, Genres: []string{}, Confirmed: true}
	libID, _ := e.repo.InsertItem(ctx, &oi, "")
	e.repo.InsertSource(ctx, &catalog.Source{ItemID: libID, Provider: "http", Locator: "https://a.example/i.mp4"}, "")

	rt, vote := 148, 8.4
	if err := e.repo.UpsertDetails(ctx, &catalog.Details{
		ItemID: id, TMDBKind: "movie", RuntimeMin: &rt, VoteAverage: &vote, Countries: []string{"Estados Unidos"},
		Cast:    []catalog.Person{{ID: 1, Name: "Leonardo DiCaprio", Character: "Cobb", Photo: "profile_w185_x.jpg", ProfilePath: "/raw.jpg"}, {ID: 2, Name: "Tom Hardy"}},
		Crew:    []catalog.CrewGroup{{Role: "Dirección", Names: []string{"Christopher Nolan"}}},
		Similar: []catalog.SimilarTitle{{TMDBID: 157336, Kind: "movie", Title: "Interstellar", Year: 2014, Poster: "sposter_w185_y.jpg"}, {TMDBID: 603, Kind: "movie", Title: "Matrix", Year: 1999}},
	}); err != nil {
		t.Fatal(err)
	}
	return id, libID
}

func TestItemDetailsEndpoint(t *testing.T) {
	e := newLanEnv(t)
	id, libID := seedDetailed(t, e)

	rec := e.call("GET", "/api/v1/items/"+itoa(id)+"/details", phone, "", nil)
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var v struct {
		HasDetails bool     `json:"has_details"`
		RuntimeMin int      `json:"runtime_min"`
		Vote       float64  `json:"vote_average"`
		Countries  []string `json:"countries"`
		Cast       []struct {
			Name, Character, Photo string
		} `json:"cast"`
		Crew    []catalog.CrewGroup `json:"crew"`
		Similar []struct {
			Title  string `json:"title"`
			Poster string `json:"poster"`
			ItemID *int64 `json:"item_id"`
		} `json:"similar"`
		Files []struct {
			Width, Height int
			VideoCodec    string `json:"video_codec"`
		} `json:"files"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		t.Fatal(err)
	}
	if !v.HasDetails || v.RuntimeMin != 148 || v.Vote != 8.4 || v.Countries[0] != "Estados Unidos" {
		t.Errorf("básicos = %+v", v)
	}
	if len(v.Cast) != 2 || v.Cast[0].Photo != "profile_w185_x.jpg" || v.Cast[1].Photo != "" {
		t.Errorf("reparto = %+v", v.Cast)
	}
	if len(v.Similar) != 2 || v.Similar[0].ItemID == nil || *v.Similar[0].ItemID != libID || v.Similar[1].ItemID != nil {
		t.Errorf("similares: el que está en la biblioteca enlaza, el otro no: %+v", v.Similar)
	}
	if len(v.Files) != 1 || v.Files[0].Width != 1920 || v.Files[0].VideoCodec != "h264" {
		t.Errorf("ficha técnica = %+v", v.Files)
	}
	// Un dispositivo de la red NUNCA ve rutas del disco ni rutas crudas de TMDB.
	body := rec.Body.String()
	for _, leak := range []string{"Privado", `D:\`, "/raw.jpg", "profile_path", "locator"} {
		if strings.Contains(body, leak) {
			t.Errorf("la respuesta filtra %q: %s", leak, body)
		}
	}
}

func TestItemDetailsWithoutTMDBDataIsEmptyNotError(t *testing.T) {
	e := newLanEnv(t)
	it := catalog.Item{Kind: catalog.KindOther, Title: "video casero", Genres: []string{}}
	id, _ := e.repo.InsertItem(context.Background(), &it, "")

	rec := e.call("GET", "/api/v1/items/"+itoa(id)+"/details", pc, "", nil)
	if rec.Code != 200 {
		t.Fatalf("status %d", rec.Code)
	}
	var v map[string]any
	json.Unmarshal(rec.Body.Bytes(), &v)
	if v["has_details"] != false {
		t.Errorf("has_details = %v", v["has_details"])
	}
	for _, k := range []string{"cast", "crew", "similar", "files", "countries"} {
		if arr, ok := v[k].([]any); !ok || len(arr) != 0 {
			t.Errorf("%s debe ser [] y es %v", k, v[k])
		}
	}
	if rec := e.call("GET", "/api/v1/items/9999/details", pc, "", nil); rec.Code != 404 {
		t.Errorf("item inexistente = %d", rec.Code)
	}
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }
