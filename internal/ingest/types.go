// Package ingest implementa la ingesta universal de M5: detecta el tipo de
// una entrada pegada (texto, M3U, CSV), extrae y clasifica sus locators,
// los enruta hacia el catálogo (reutilizando el normalizador/matcher de M3)
// y devuelve un resumen honesto de qué pasó con cada cosa (Brief M5).
package ingest

// InputType es el tipo de un blob de entrada completo (Brief M5 §1).
type InputType string

const (
	InputM3U  InputType = "m3u"
	InputCSV  InputType = "csv"
	InputText InputType = "text"
)

// Candidate es un locator candidato a convertirse en fuente del catálogo,
// ya clasificado (Brief M5 §2-§4). TitleHint/Year vienen del EXTINF de una
// M3U o de las columnas title/year de un CSV; vacíos si no hay pista y el
// normalizador de M3 deriva el título del propio locator.
type Candidate struct {
	Locator   string
	Class     URLClass
	TitleHint string
	Year      int
	// Origin es de dónde vino el candidato dentro de ESTA ingesta: "m3u"
	// (de una lista M3U, propia o descubierta al resolver un m3u_remoto),
	// "csv" (de una fila de CSV), o "url" (de texto suelto). Alimenta
	// ResumenIngesta.PorTipo.
	Origin string
	// nestedListID: si el candidato vino de expandir una M3U remota, el id
	// de la fila ingest_lists creada para ESA lista concreta (trazabilidad
	// fina); 0 = usar la lista de la ingesta completa.
	nestedListID int64
}

// LocalPathCandidate es una línea de texto suelto que resultó ser una ruta
// de archivo/carpeta existente en este equipo (Brief M5 §1). M5 NO la
// procesa: se informa en el resumen para que el usuario use "Añadir
// carpeta" (M2), decisión de alcance documentada.
type LocalPathCandidate struct {
	Path  string
	IsDir bool
}
