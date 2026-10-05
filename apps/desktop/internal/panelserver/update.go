package panelserver

import (
	"context"
	"net/http"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/update"
)

// handleUpdateGet returns the current updater state.
func (s *Server) handleUpdateGet(w http.ResponseWriter, _ *http.Request) {
	if s.cfg.Update == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "No disponible"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "update": s.cfg.Update.State()})
}

// handleUpdateCheck resolves the latest release now and reports the state.
func (s *Server) handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Update == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "No disponible"})
		return
	}
	if _, _, err := s.cfg.Update.Check(r.Context()); err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{
			"ok":     false,
			"error":  err.Error(),
			"update": s.cfg.Update.State(),
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "update": s.cfg.Update.State()})
}

// handleUpdateApply downloads and replaces the executable in the background.
// The app restarts when the replacement finishes, so the response returns
// before the update completes.
func (s *Server) handleUpdateApply(w http.ResponseWriter, _ *http.Request) {
	if s.cfg.Update == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "No disponible"})
		return
	}
	state := s.cfg.Update.State()
	if state.Status == update.StatusDownloading || state.Status == update.StatusApplying {
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": "Ya hay una actualización en curso", "update": state})
		return
	}
	go func() {
		if _, _, err := s.cfg.Update.Update(context.Background()); err != nil {
			s.log.Printf("update: %v", err)
		}
	}()
	writeJSON(w, http.StatusAccepted, map[string]any{"ok": true, "started": true, "update": s.cfg.Update.State()})
}
