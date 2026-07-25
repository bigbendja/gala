package api

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// maxIngestBodyBytes es el límite de tamaño del blob de ingesta (Brief M5
// §5/§6): un pegado o archivo razonable, no una forma de tirar el servidor.
const maxIngestBodyBytes = 5 << 20 // 5 MB

// handleIngest: POST /api/v1/ingest — texto plano (el textarea) o multipart
// con un archivo .m3u/.csv/.txt (Brief M5 §5). Devuelve el ResumenIngesta.
func (s *Server) handleIngest(w http.ResponseWriter, r *http.Request) {
	if s.Ingest == nil {
		s.fail(w, errors.New("pipeline de ingesta no configurado"))
		return
	}

	// http.MaxBytesReader corta la lectura (texto plano o multipart, ambos
	// leen de r.Body) en cuanto se supera el límite, con un *http.MaxBytesError
	// detectable — más fiable que fijar un tope solo en ParseMultipartForm,
	// que no acota el cuerpo entero por sí solo.
	r.Body = http.MaxBytesReader(w, r.Body, maxIngestBodyBytes)

	blob, filename, err := readIngestBody(r)
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeJSON(w, http.StatusRequestEntityTooLarge, map[string]string{"error": "el contenido supera el límite de 5 MB"})
			return
		}
		badRequest(w, "cuerpo no válido: "+err.Error())
		return
	}
	if strings.TrimSpace(blob) == "" {
		badRequest(w, "nada que ingerir")
		return
	}

	res, err := s.Ingest.Run(r.Context(), blob, filename, "")
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

// readIngestBody lee el blob de la petición: multipart con un campo "file",
// o el cuerpo entero como texto plano si no es multipart.
func readIngestBody(r *http.Request) (blob, filename string, err error) {
	if strings.HasPrefix(r.Header.Get("Content-Type"), "multipart/") {
		if err := r.ParseMultipartForm(maxIngestBodyBytes); err != nil {
			return "", "", err
		}
		file, header, ferr := r.FormFile("file")
		if ferr != nil {
			return "", "", fmt.Errorf("falta el archivo (campo \"file\"): %w", ferr)
		}
		defer file.Close()
		data, rerr := io.ReadAll(file)
		if rerr != nil {
			return "", "", rerr
		}
		return string(data), header.Filename, nil
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		return "", "", err
	}
	return string(data), "", nil
}

// handleIngestLists: GET /api/v1/ingest/lists — trazabilidad de ingestas
// (Brief M5 §5).
func (s *Server) handleIngestLists(w http.ResponseWriter, r *http.Request) {
	lists, err := s.Catalog.ListIngestLists(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, lists)
}
