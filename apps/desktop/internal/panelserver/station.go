package panelserver

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/settings"
)

// exportFormat tags a configuration file so the import can reject anything else.
const exportFormat = "palorosa-kitchen/settings"

// configFile is the exported configuration: the full settings, secrets in clear.
type configFile struct {
	Format     string          `json:"format"`
	Version    int             `json:"version"`
	ExportedAt time.Time       `json:"exportedAt"`
	Settings   settings.Values `json:"settings"`
}

// StationKey derives the key a spectator app presents to the host. Both apps
// share the webhook secret through the exported configuration; the key does
// not reveal it. Empty when there is no secret.
func StationKey(webhookSecret string) string {
	if webhookSecret == "" {
		return ""
	}
	mac := hmac.New(sha256.New, []byte(webhookSecret))
	mac.Write([]byte("palorosa-station"))
	return hex.EncodeToString(mac.Sum(nil))
}

// handleSettingsExport downloads the whole configuration as a JSON file, so
// another computer can import it. Host only: it carries every secret.
func (s *Server) handleSettingsExport(w http.ResponseWriter, _ *http.Request) {
	if s.cfg.Settings == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Settings unavailable"})
		return
	}
	file := configFile{Format: exportFormat, Version: 1, ExportedAt: time.Now().UTC(), Settings: s.cfg.Settings.Values()}
	body, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	name := fmt.Sprintf("palorosa-configuracion-%s.json", time.Now().Format("2006-01-02"))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(body)
}

// handleSettingsImport replaces the configuration with an exported file.
func (s *Server) handleSettingsImport(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Settings == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Settings unavailable"})
		return
	}
	raw, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "No pudimos leer el archivo"})
		return
	}
	var file configFile
	if err := json.Unmarshal(raw, &file); err != nil || file.Format != exportFormat {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "El archivo no es una configuración de Palorosa Kitchen"})
		return
	}
	if err := s.cfg.Settings.Update(file.Settings); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if s.cfg.OnChange != nil {
		s.cfg.OnChange()
	}
	writeJSON(w, http.StatusOK, file.Settings.Redacted())
}

// handleRestart relaunches the desktop app (host only). The response is sent
// first and the process quits a moment later, so the panel can show a notice.
func (s *Server) handleRestart(w http.ResponseWriter, _ *http.Request) {
	if s.cfg.Restart == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Sin reinicio"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	go s.cfg.Restart()
}

// handleStation signs a spectator app in: it presents the station key derived
// from the shared webhook secret and gets a spectator cookie, then lands on
// the panel.
func (s *Server) handleStation(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Auth == nil {
		http.Error(w, "Auth unavailable", http.StatusServiceUnavailable)
		return
	}
	secret := s.cfg.StationSecret
	if s.cfg.Settings != nil {
		values := s.cfg.Settings.Values()
		if values.StationSecret != "" {
			secret = values.StationSecret
		} else if values.WebhookSecret != "" {
			secret = values.WebhookSecret
		}
	}
	expected := StationKey(secret)
	given := r.URL.Query().Get("key")
	if expected == "" || !hmac.Equal([]byte(given), []byte(expected)) {
		http.Redirect(w, r, "/", http.StatusFound)
		return
	}
	label := r.URL.Query().Get("label")
	if label == "" {
		label = "Estación"
	}
	_, invite, err := s.cfg.Auth.CreateInvite(label, time.Minute)
	if err == nil {
		session, token, redeemErr := s.cfg.Auth.Redeem(invite, label)
		if redeemErr == nil {
			setSessionCookie(w, r, token, session.ExpiresAt)
		}
	}
	http.Redirect(w, r, "/", http.StatusFound)
}
