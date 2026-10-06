package panelserver

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/settings"
)

// Handoff is the host-to-host transfer (D30): the host mints a short one-time
// code, the machine that wants to take over claims it with a spectator
// session, downloads the whole kitchen state except the WhatsApp sessions and
// asks the host to step down. Nothing is granted without the host code.
const (
	handoffFormat   = "palorosa-kitchen/handoff"
	handoffCodeTTL  = 10 * time.Minute
	handoffGrantTTL = 30 * time.Minute
	// handoffMaxAttempts invalidates the code after a few wrong guesses, so a
	// spectator session cannot brute-force the six digits.
	handoffMaxAttempts = 5
)

// HandoffIdentity tells the receiving machine who is handing over.
type HandoffIdentity struct {
	Instance string `json:"instance,omitempty"`
	Machine  string `json:"machine,omitempty"`
	Version  string `json:"version,omitempty"`
}

// HandoffBundle is the transferable state. The settings carry every secret;
// the catalog, print template, targets and bots travel as raw JSON files and
// are empty when the host has none. WhatsApp sessions never travel.
type HandoffBundle struct {
	Format        string          `json:"format"`
	Version       int             `json:"version"`
	ExportedAt    time.Time       `json:"exportedAt"`
	Host          HandoffIdentity `json:"host"`
	Settings      settings.Values `json:"settings"`
	Catalog       json.RawMessage `json:"catalog,omitempty"`
	PrintTemplate json.RawMessage `json:"printTemplate,omitempty"`
	Targets       json.RawMessage `json:"targets,omitempty"`
	Bots          json.RawMessage `json:"bots,omitempty"`
}

// handoffGrant is a claimed code, bound to the session that claimed it.
type handoffGrant struct {
	token     string
	sessionID string
	expires   time.Time
	fetched   bool
}

// handoffState is the live host transfer state: one code and one grant at a
// time, both short lived and in memory only.
type handoffState struct {
	mu          sync.Mutex
	code        string
	codeExpires time.Time
	attempts    int
	grant       *handoffGrant
}

func (s *Server) handoffEnabled() bool {
	return s.cfg.HandoffBundle != nil
}

func (s *Server) handoffIdentity() HandoffIdentity {
	return HandoffIdentity{
		Instance: s.cfg.InstanceID,
		Machine:  s.cfg.Machine,
		Version:  s.cfg.Version,
	}
}

// handleHandoffCode mints the one-time code the other machine types in. Host
// only.
func (s *Server) handleHandoffCode(w http.ResponseWriter, _ *http.Request) {
	if !s.handoffEnabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Traspaso no disponible"})
		return
	}
	code, err := handoffCode()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	expires := time.Now().Add(handoffCodeTTL)
	s.handoff.mu.Lock()
	s.handoff.code = code
	s.handoff.codeExpires = expires
	s.handoff.attempts = 0
	s.handoff.grant = nil
	s.handoff.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "code": code, "expiresAt": expires})
}

// handleHandoffClaim exchanges a valid code for a one-time grant tied to the
// caller's spectator session.
func (s *Server) handleHandoffClaim(w http.ResponseWriter, r *http.Request) {
	if !s.handoffEnabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Traspaso no disponible"})
		return
	}
	id := s.identify(r)
	if id.Session.ID == "" {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "El anfitrión no toma el control de sí mismo"})
		return
	}
	var payload struct {
		Code string `json:"code"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Invalid JSON body"})
		return
	}
	now := time.Now()
	s.handoff.mu.Lock()
	valid := s.handoff.code != "" && now.Before(s.handoff.codeExpires) &&
		subtle.ConstantTimeCompare([]byte(strings.TrimSpace(payload.Code)), []byte(s.handoff.code)) == 1
	if !valid {
		s.handoff.attempts++
		if s.handoff.attempts >= handoffMaxAttempts {
			s.handoff.code = ""
		}
		s.handoff.mu.Unlock()
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "Código no válido o vencido"})
		return
	}
	token, err := handoffToken()
	if err != nil {
		s.handoff.mu.Unlock()
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	expires := now.Add(handoffGrantTTL)
	s.handoff.code = ""
	s.handoff.grant = &handoffGrant{token: token, sessionID: id.Session.ID, expires: expires}
	s.handoff.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":        true,
		"token":     token,
		"expiresAt": expires,
		"host":      s.handoffIdentity(),
	})
}

// grantValid consumes a grant: the token must match, belong to the caller's
// session and be unexpired. When fetch is true the bundle can only be read
// once.
func (s *Server) grantValid(token, sessionID string, fetch bool) bool {
	s.handoff.mu.Lock()
	defer s.handoff.mu.Unlock()
	grant := s.handoff.grant
	if grant == nil || sessionID == "" {
		return false
	}
	if time.Now().After(grant.expires) {
		s.handoff.grant = nil
		return false
	}
	if subtle.ConstantTimeCompare([]byte(token), []byte(grant.token)) != 1 {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(sessionID), []byte(grant.sessionID)) != 1 {
		return false
	}
	if fetch {
		if grant.fetched {
			return false
		}
		grant.fetched = true
	}
	return true
}

// handleHandoffBundle returns the whole transferable state once.
func (s *Server) handleHandoffBundle(w http.ResponseWriter, r *http.Request) {
	if !s.handoffEnabled() {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Traspaso no disponible"})
		return
	}
	id := s.identify(r)
	if !s.grantValid(r.URL.Query().Get("token"), id.Session.ID, true) {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "Permiso de traspaso no válido"})
		return
	}
	bundle, err := s.cfg.HandoffBundle()
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	bundle.Format = handoffFormat
	bundle.Version = 1
	bundle.ExportedAt = time.Now().UTC()
	bundle.Host = s.handoffIdentity()
	writeJSON(w, http.StatusOK, bundle)
}

// handleHandoffComplete is the new host saying "I have everything": the old
// host steps down and its window becomes a spectator.
func (s *Server) handleHandoffComplete(w http.ResponseWriter, r *http.Request) {
	if s.cfg.OnHandoff == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Traspaso no disponible"})
		return
	}
	var payload struct {
		Token string `json:"token"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Invalid JSON body"})
		return
	}
	id := s.identify(r)
	if !s.grantValid(payload.Token, id.Session.ID, false) {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "Permiso de traspaso no válido"})
		return
	}
	s.handoff.mu.Lock()
	s.handoff.grant = nil
	s.handoff.mu.Unlock()
	s.log.Printf("handoff: %s took over", id.Session.Label)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	go s.cfg.OnHandoff()
}

// handleStationRotate mints a fresh spectator key and drops every session, so
// a leaked key stops working without touching the WooCommerce webhook secret.
func (s *Server) handleStationRotate(w http.ResponseWriter, _ *http.Request) {
	if s.cfg.Settings == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Settings unavailable"})
		return
	}
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	values := s.cfg.Settings.Values()
	values.StationSecret = hex.EncodeToString(buffer)
	if err := s.cfg.Settings.Update(values); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	revoked := 0
	if s.cfg.Auth != nil {
		revoked = s.cfg.Auth.RevokeAllSessions()
	}
	if s.cfg.OnChange != nil {
		s.cfg.OnChange()
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "revoked": revoked})
}

// handleSessionsRevokeAll logs every spectator out (host only).
func (s *Server) handleSessionsRevokeAll(w http.ResponseWriter, _ *http.Request) {
	if s.cfg.Auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Auth unavailable"})
		return
	}
	revoked := s.cfg.Auth.RevokeAllSessions()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "revoked": revoked})
}

// handoffCode returns a six-digit code from crypto/rand, zero padded.
func handoffCode() (string, error) {
	buffer := make([]byte, 4)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	value := uint32(buffer[0])<<24 | uint32(buffer[1])<<16 | uint32(buffer[2])<<8 | uint32(buffer[3])
	digits := value % 1000000
	code := make([]byte, 6)
	for i := 5; i >= 0; i-- {
		code[i] = byte('0' + digits%10)
		digits /= 10
	}
	return string(code), nil
}

// handoffToken returns the random grant token.
func handoffToken() (string, error) {
	buffer := make([]byte, 32)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
