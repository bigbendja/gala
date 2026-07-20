package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bigbendja/gala/internal/providers/localfs"
	"github.com/bigbendja/gala/internal/scan"
)

// folderInfo es una carpeta registrada con el estado de su volumen.
type folderInfo struct {
	Path      string `json:"path"`
	Connected bool   `json:"connected"`
}

// handleListFolders: GET /api/v1/folders.
func (s *Server) handleListFolders(w http.ResponseWriter, r *http.Request) {
	folders := append([]string(nil), s.Config.MediaFolders...)
	vols, err := s.Enum.Volumes(r.Context(), folders)
	if err != nil {
		s.fail(w, err)
		return
	}
	out := make([]folderInfo, 0, len(folders))
	for _, f := range folders {
		_, connected := localfs.MatchVolume(f, vols)
		out = append(out, folderInfo{Path: f, Connected: connected})
	}
	writeJSON(w, http.StatusOK, out)
}

type addFolderRequest struct {
	Path string `json:"path"`
}

// handleAddFolder: POST /api/v1/folders  body {"path": "E:\\Peliculas"}.
func (s *Server) handleAddFolder(w http.ResponseWriter, r *http.Request) {
	var req addFolderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || strings.TrimSpace(req.Path) == "" {
		badRequest(w, "path no válido")
		return
	}
	abs, err := filepath.Abs(req.Path)
	if err != nil {
		badRequest(w, "path no válido")
		return
	}
	info, err := os.Stat(abs)
	if err != nil || !info.IsDir() {
		badRequest(w, "la ruta no existe o no es una carpeta")
		return
	}
	for _, f := range s.Config.MediaFolders {
		if localfs.NormalizePath(f) == localfs.NormalizePath(abs) {
			writeJSON(w, http.StatusConflict, map[string]string{"error": "la carpeta ya está registrada"})
			return
		}
	}

	s.Config.MediaFolders = append(s.Config.MediaFolders, abs)
	if err := s.Config.Save(); err != nil {
		s.fail(w, err)
		return
	}

	if err := s.Scanner.Start([]string{abs}); err != nil && !errors.Is(err, scan.ErrAlreadyRunning) {
		s.Log.Error("lanzar escaneo tras alta de carpeta", "path", abs, "err", err)
	}

	writeJSON(w, http.StatusOK, folderInfo{Path: abs, Connected: true})
}

// handleDeleteFolder: DELETE /api/v1/folders?path=... — la quita de config y
// borra sus fuentes localfs (los items se conservan; confirmación es cosa de
// la UI, no de la API).
func (s *Server) handleDeleteFolder(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Query().Get("path")
	if strings.TrimSpace(path) == "" {
		badRequest(w, "path requerido")
		return
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		badRequest(w, "path no válido")
		return
	}

	idx := -1
	for i, f := range s.Config.MediaFolders {
		if localfs.NormalizePath(f) == localfs.NormalizePath(abs) {
			idx = i
			break
		}
	}
	if idx == -1 {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "carpeta no registrada"})
		return
	}

	s.Config.MediaFolders = append(s.Config.MediaFolders[:idx], s.Config.MediaFolders[idx+1:]...)
	if err := s.Config.Save(); err != nil {
		s.fail(w, err)
		return
	}

	if _, err := scan.RemoveFolderSources(r.Context(), s.Catalog, abs); err != nil {
		s.Log.Error("eliminar fuentes de carpeta", "path", abs, "err", err)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "eliminada"})
}

// handleStartScan: POST /api/v1/scan — rescaneo asíncrono de todo lo
// registrado; 409 si ya hay uno en curso.
func (s *Server) handleStartScan(w http.ResponseWriter, r *http.Request) {
	if err := s.Scanner.Start(nil); err != nil {
		writeJSON(w, http.StatusConflict, map[string]string{"error": "ya hay un escaneo en curso"})
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"status": "escaneando"})
}

// handleScanStatus: GET /api/v1/scan/status.
func (s *Server) handleScanStatus(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, s.Scanner.Status())
}

// handleListVolumes: GET /api/v1/volumes — volúmenes conocidos con estado.
func (s *Server) handleListVolumes(w http.ResponseWriter, r *http.Request) {
	vols, err := s.Catalog.ListVolumes(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, vols)
}
