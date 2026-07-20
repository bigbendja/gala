package catalog

import (
	"context"
	"fmt"
)

// SeedDemo puebla la BD con datos ficticios de desarrollo si está vacía.
// Solo para el flag --demo: permite ver la galería y probar la API sin
// providers reales (llegan en M2). Títulos inventados a propósito.
func SeedDemo(ctx context.Context, r *Repo) (int, error) {
	n, err := r.CountItems(ctx)
	if err != nil {
		return 0, err
	}
	if n > 0 {
		return 0, nil // nunca sobre datos existentes
	}

	str := func(s string) *string { return &s }
	num := func(i int) *int { return &i }

	type demo struct {
		item    Item
		sources []Source
	}
	demos := []demo{
		{
			item: Item{Kind: KindMovie, Title: "Cielo de Harmattan", Year: num(2023),
				Overview:   str("Tras diez años fuera, Nadia vuelve a la ciudad costera de su infancia."),
				Genres:     []string{"Drama"},
				RatingCert: str("12"), Confirmed: true},
			sources: []Source{
				{Provider: "localfs", Locator: `E:\Peliculas\Cielo.de.Harmattan.2023.1080p.mkv`,
					QualityLabel: str("1080p"), Health: HealthAlive, Priority: 10},
				{Provider: "http", Locator: "https://ejemplo.invalid/harmattan_hd.mp4",
					Health: HealthDead},
			},
		},
		{
			item: Item{Kind: KindMovie, Title: "Marea Negra", Year: num(2019),
				Genres: []string{"Thriller"}, Confirmed: true},
			sources: []Source{{Provider: "localfs",
				Locator:      `E:\Peliculas\Marea.Negra.2019.720p.mp4`,
				QualityLabel: str("720p"), Health: HealthSlow, Priority: 10}},
		},
		{
			item: Item{Kind: KindSeries, Title: "Ruta 3", Year: num(2024),
				Genres: []string{"Drama", "Crimen"}, Confirmed: true},
		},
		{
			item: Item{Kind: KindMovie, Title: "El Último Tren", Year: num(2024),
				Genres: []string{"Aventura"}, Confirmed: true, Wished: true},
			// deseo: sin fuentes
		},
		{
			item: Item{Kind: KindMovie, Title: "pelicula.sin.identificar.x264",
				Genres: []string{}, Confirmed: false},
			sources: []Source{{Provider: "localfs",
				Locator: `E:\Descargas\pelicula.sin.identificar.x264.mkv`,
				Health:  HealthUnverified}},
		},
	}

	count := 0
	for _, d := range demos {
		id, err := r.InsertItem(ctx, &d.item, "")
		if err != nil {
			return count, fmt.Errorf("seed item %q: %w", d.item.Title, err)
		}
		count++
		for i := range d.sources {
			d.sources[i].ItemID = id
			if _, err := r.InsertSource(ctx, &d.sources[i], ""); err != nil {
				return count, fmt.Errorf("seed source de %q: %w", d.item.Title, err)
			}
		}
	}
	return count, nil
}
