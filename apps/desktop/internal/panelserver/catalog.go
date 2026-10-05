package panelserver

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/engine"
)

// maxCatalogBytes bounds an uploaded catalog. The seed is ~175 KB today.
const maxCatalogBytes = 8 << 20

func (s *Server) catalogPath() string {
	if s.cfg.CatalogPath == nil {
		return ""
	}
	return s.cfg.CatalogPath()
}

// handleCatalogGet returns the raw catalog seed for the editor (host only).
func (s *Server) handleCatalogGet(w http.ResponseWriter, r *http.Request) {
	path := s.catalogPath()
	if path == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "No hay catálogo configurado"})
		return
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if !json.Valid(raw) {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "El catálogo en disco no es JSON válido"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "path": path, "catalog": json.RawMessage(raw)})
}

// handleCatalogPut validates and saves the catalog, then swaps it live.
func (s *Server) handleCatalogPut(w http.ResponseWriter, r *http.Request) {
	path := s.catalogPath()
	if path == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "No hay catálogo configurado"})
		return
	}
	var payload struct {
		Catalog json.RawMessage `json:"catalog"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxCatalogBytes)).Decode(&payload); err != nil || len(payload.Catalog) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Invalid JSON body"})
		return
	}
	catalog, err := engine.SaveCatalog(path, payload.Catalog)
	var invalid *engine.CatalogError
	if errors.As(err, &invalid) {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"ok": false, "error": "El catálogo tiene errores", "problems": invalid.Problems})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if s.cfg.OnCatalogSaved != nil {
		s.cfg.OnCatalogSaved(catalog)
	}
	s.log.Printf("catalog: saved %d products, %d recipes, %d units", len(catalog.Products), len(catalog.Recipes), len(catalog.Units))
	s.Publish("orders")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "products": len(catalog.Products), "recipes": len(catalog.Recipes), "units": len(catalog.Units)})
}

// handlePrintTemplateGet returns the saved print layout, or null when the
// panel should use its built-in default.
func (s *Server) handlePrintTemplateGet(w http.ResponseWriter, r *http.Request) {
	if s.cfg.PrintTemplatePath == "" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "template": nil})
		return
	}
	raw, err := os.ReadFile(s.cfg.PrintTemplatePath)
	if errors.Is(err, os.ErrNotExist) || (err == nil && !json.Valid(raw)) {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "template": nil})
		return
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "template": json.RawMessage(raw)})
}

// handlePrintTemplatePut stores the print layout. A null template resets it
// to the panel default.
func (s *Server) handlePrintTemplatePut(w http.ResponseWriter, r *http.Request) {
	if s.cfg.PrintTemplatePath == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "No disponible"})
		return
	}
	var payload struct {
		Template json.RawMessage `json:"template"`
	}
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Invalid JSON body"})
		return
	}
	if len(payload.Template) == 0 || string(payload.Template) == "null" {
		if err := os.Remove(s.cfg.PrintTemplatePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
		return
	}
	var shape struct {
		Left  []json.RawMessage `json:"left"`
		Right []json.RawMessage `json:"right"`
	}
	if err := json.Unmarshal(payload.Template, &shape); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Plantilla inválida"})
		return
	}
	if err := os.MkdirAll(filepath.Dir(s.cfg.PrintTemplatePath), 0o755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	temporary := s.cfg.PrintTemplatePath + ".tmp"
	err := os.WriteFile(temporary, payload.Template, 0o644)
	if err == nil {
		err = os.Rename(temporary, s.cfg.PrintTemplatePath)
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}
