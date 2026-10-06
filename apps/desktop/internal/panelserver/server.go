// Package panelserver serves the desktop admin panel and its JSON API on a
// local-only HTTP server. It mounts the WhatsApp hook handlers so a single
// origin covers the panel and the webhooks (D24, D27).
//
// Access model (D27): a direct local request is the host; a request that
// arrives through the tunnel (Cloudflare forwarding headers) or the LAN is a
// spectator and must present a session cookie obtained by redeeming an invite.
// Reads are open to any authenticated role; writes are host only.
package panelserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/auth"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/bots"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/engine"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/notify"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/panelmodel"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/push"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/selftest"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/settings"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/update"
)

// sessionCookie is the spectator session cookie name.
const sessionCookie = "palorosa_session"

// Config configures the panel server.
type Config struct {
	// Port is the loopback port (PANEL_PORT).
	Port int
	// Settings is the persisted host configuration, already opened.
	Settings *settings.Store
	// Auth manages invites and spectator sessions. Nil disables remote access.
	Auth *auth.Manager
	// PanelBaseURL is the public origin used to build invite links (for
	// example https://cocina.example.com). Empty falls back to the request.
	PanelBaseURL string
	// Panel is the built Svelte single-file app served at /. Empty falls back
	// to a placeholder page.
	Panel []byte
	// Hooks re-mounts the WhatsApp routes under /hook/ and /debug/.
	Hooks http.Handler
	// Debug exposes the /debug/ routes when true.
	Debug bool
	// InstanceID identifies this running app in /health, so a second machine
	// can tell a live host from its own stale tunnel.
	InstanceID string
	// Machine is a short hash of the computer name, also reported in /health.
	Machine string
	// StationSecret is the effective webhook secret (settings or .env); the
	// station key spectator apps sign in with derives from it.
	StationSecret string
	// OnChange runs after a settings update so live services can reconnect.
	OnChange func()
	// Tunnel reports the named tunnel state for the panel. Nil disables the
	// /api/panel/tunnel route.
	Tunnel func() TunnelInfo
	// TunnelLogin starts the one-time cloudflared login. Nil disables the
	// /api/panel/tunnel/login route.
	TunnelLogin func() error
	// Snapshot returns the current kitchen state (WhatsApp status and days).
	Snapshot func() panelmodel.Snapshot
	// RemoveOrder drops an order from a day.
	RemoveOrder func(date, number string) error
	// OrderDetail resolves one order for the lazy ticket view. Nil disables
	// the order-details route.
	OrderDetail func(date, number string) (panelmodel.OrderDetail, error)
	// Restart relaunches the desktop app (host only). Nil disables the
	// restart route.
	Restart func()
	// SetNotifications updates the global notification control.
	SetNotifications func(panelmodel.Notifications) error
	// Targets manages notification targets and their schedules. Nil disables
	// the targets routes.
	Targets *notify.Manager
	// Push delivers Web Push notifications. Nil disables the push routes.
	Push *push.Manager
	// Bots manages the WhatsApp accounts. Nil disables the bots routes.
	Bots *bots.Manager
	// PublishList recomputes and stores the kitchen list for a date. Nil falls
	// back to the bots manager, so the panel can publish without one.
	PublishList func(date string) error
	// Autostart reports whether the app opens at login. Nil hides it.
	Autostart func() bool
	// SetAutostart enables or disables opening at login.
	SetAutostart func(bool) error
	// Diagnostics returns the running configuration for the advanced settings.
	Diagnostics func() Diagnostics
	// SelfTest runs the Ajustes → Avanzado health checks. Nil hides them.
	SelfTest *selftest.Runner
	// Update runs the GitHub Releases self-update shown in Ajustes. Nil
	// hides the update routes.
	Update *update.Manager
	// SyncOrders runs the store's WP All Export for the given delivery dates
	// (YYYY-MM-DD) and makes those days match it. Nil disables the sync.
	SyncOrders func(context.Context, []string) (any, error)
	// SyncInterval returns the automatic sync interval in minutes (0 = off).
	SyncInterval func() int
	// CatalogPath returns the catalog seed the kitchen uses. Nil or empty
	// disables the catalog editor routes.
	CatalogPath func() string
	// OnCatalogSaved swaps the live catalog after the editor saves it.
	OnCatalogSaved func(*engine.Catalog)
	// PrintTemplatePath is where the editable print sheet layout is stored.
	// Empty disables the print template routes.
	PrintTemplatePath string
	// LogPath is the app.log file offered for download in Ajustes. Empty
	// disables the download route.
	LogPath string
	// OpenLogFolder reveals the log directory in the file explorer. Nil hides it.
	OpenLogFolder func() error
	// PanelIcon is the PNG icon served at /icon.png for the PWA.
	PanelIcon []byte
	// Logger defaults to the standard logger.
	Logger *log.Logger
}

// Diagnostics is the read-only running configuration shown in Ajustes.
type Diagnostics struct {
	Version   string `json:"version"`
	Uptime    string `json:"uptime"`
	DataDir   string `json:"dataDir"`
	Catalog   string `json:"catalog"`
	PanelPort int    `json:"panelPort"`
	Debug     bool   `json:"debug"`
	Logs      string `json:"logs"`
	LogPath   string `json:"logPath,omitempty"`
}

// TunnelInfo is the tunnel state the panel reports. User text comes from the
// label maps; this stays machine readable.
type TunnelInfo struct {
	Hostname    string `json:"hostname"`
	URL         string `json:"url"`
	Service     string `json:"service"`
	Status      string `json:"status"`
	Detail      string `json:"detail"`
	CertPresent bool   `json:"certPresent"`
	// Token reports that the tunnel runs with a token instead of the account
	// certificate, so the panel hides the cloudflared login guidance.
	Token bool `json:"token,omitempty"`
}

// Server is the local panel HTTP server.
type Server struct {
	cfg Config
	log *log.Logger

	mu   sync.Mutex
	subs map[int]chan string
	next int

	syncer *orderSyncer
}

// New builds the panel server. It does not listen until Run is called.
func New(cfg Config) *Server {
	logger := cfg.Logger
	if logger == nil {
		logger = log.Default()
	}
	server := &Server{cfg: cfg, log: logger, subs: map[int]chan string{}}
	if cfg.SyncOrders != nil {
		server.syncer = &orderSyncer{run: cfg.SyncOrders}
	}
	return server
}

// Publish pushes an event name to every open SSE subscriber.
func (s *Server) Publish(event string) {
	s.mu.Lock()
	for _, channel := range s.subs {
		select {
		case channel <- event:
		default:
		}
	}
	s.mu.Unlock()
	if event == "orders" && s.cfg.Push != nil {
		go s.cfg.Push.Send()
	}
}

func (s *Server) subscribe() (int, chan string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.next++
	id := s.next
	channel := make(chan string, 8)
	s.subs[id] = channel
	return id, channel
}

func (s *Server) unsubscribe(id int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.subs, id)
}

// Handler is the panel router, exported for tests.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)

	mux.HandleFunc("GET /api/panel/session", s.handleSessionGet)
	mux.HandleFunc("POST /api/panel/session", s.handleSessionCreate)
	mux.HandleFunc("DELETE /api/panel/session", s.handleSessionDelete)

	mux.HandleFunc("GET /api/panel/settings", s.hostOnly(s.handleSettingsGet))
	mux.HandleFunc("PATCH /api/panel/settings", s.hostOnly(s.handleSettingsPatch))
	mux.HandleFunc("GET /api/panel/settings/export", s.hostOnly(s.handleSettingsExport))
	mux.HandleFunc("POST /api/panel/settings/import", s.hostOnly(s.handleSettingsImport))
	mux.HandleFunc("POST /api/panel/restart", s.hostOnly(s.handleRestart))
	mux.HandleFunc("GET /api/panel/station", s.handleStation)
	mux.HandleFunc("GET /api/panel/autostart", s.hostOnly(s.handleAutostartGet))
	mux.HandleFunc("POST /api/panel/autostart", s.hostOnly(s.handleAutostartSet))
	mux.HandleFunc("GET /api/panel/diagnostics", s.hostOnly(s.handleDiagnostics))
	mux.HandleFunc("GET /api/panel/update", s.hostOnly(s.handleUpdateGet))
	mux.HandleFunc("POST /api/panel/update/check", s.hostOnly(s.handleUpdateCheck))
	mux.HandleFunc("POST /api/panel/update/apply", s.hostOnly(s.handleUpdateApply))
	mux.HandleFunc("GET /api/panel/logs/download", s.hostOnly(s.handleLogDownload))
	mux.HandleFunc("POST /api/panel/logs/open", s.hostOnly(s.handleLogOpen))
	mux.HandleFunc("GET /api/panel/selftest", s.hostOnly(s.handleSelfTestList))
	mux.HandleFunc("POST /api/panel/selftest", s.hostOnly(s.handleSelfTestRunAll))
	mux.HandleFunc("POST /api/panel/selftest/{id}", s.hostOnly(s.handleSelfTestRun))
	mux.HandleFunc("GET /api/panel/catalog", s.hostOnly(s.handleCatalogGet))
	mux.HandleFunc("PUT /api/panel/catalog", s.hostOnly(s.handleCatalogPut))
	mux.HandleFunc("GET /api/panel/print-template", s.authenticated(s.handlePrintTemplateGet))
	mux.HandleFunc("PUT /api/panel/print-template", s.hostOnly(s.handlePrintTemplatePut))
	mux.HandleFunc("GET /api/panel/tunnel", s.authenticated(s.handleTunnel))
	mux.HandleFunc("POST /api/panel/tunnel/login", s.hostOnly(s.handleTunnelLogin))
	mux.HandleFunc("GET /api/panel/state", s.authenticated(s.handleState))
	mux.HandleFunc("GET /api/panel/list", s.authenticated(s.handleList))
	mux.HandleFunc("GET /api/panel/sync", s.authenticated(s.handleSyncGet))
	mux.HandleFunc("POST /api/panel/sync", s.authenticated(s.handleSyncRun))
	mux.HandleFunc("GET /api/panel/orders", s.authenticated(s.handleOrders))
	mux.HandleFunc("GET /api/panel/order-details", s.authenticated(s.handleOrderDetails))
	mux.HandleFunc("POST /api/panel/orders/{date}/{number}/remove", s.hostOnly(s.handleRemoveOrder))
	mux.HandleFunc("PATCH /api/panel/notifications", s.hostOnly(s.handleNotifications))
	mux.HandleFunc("GET /api/panel/targets", s.authenticated(s.handleTargets))
	mux.HandleFunc("POST /api/panel/targets", s.hostOnly(s.handleTargetCreate))
	mux.HandleFunc("PATCH /api/panel/targets/{id}", s.hostOnly(s.handleTargetUpdate))
	mux.HandleFunc("DELETE /api/panel/targets/{id}", s.hostOnly(s.handleTargetDelete))
	mux.HandleFunc("POST /api/panel/targets/{id}/test", s.hostOnly(s.handleTargetTest))
	mux.HandleFunc("POST /api/panel/targets/test", s.hostOnly(s.handleTargetTestAll))
	mux.HandleFunc("POST /api/panel/list/publish", s.hostOnly(s.handleListPublish))
	mux.HandleFunc("GET /api/panel/push", s.authenticated(s.handlePushGet))
	mux.HandleFunc("POST /api/panel/push", s.authenticated(s.handlePushCreate))
	mux.HandleFunc("DELETE /api/panel/push", s.authenticated(s.handlePushDelete))
	mux.HandleFunc("GET /api/panel/bots", s.authenticated(s.handleBots))
	mux.HandleFunc("POST /api/panel/bots", s.hostOnly(s.handleBotCreate))
	mux.HandleFunc("PATCH /api/panel/bots/{id}", s.hostOnly(s.handleBotUpdate))
	mux.HandleFunc("DELETE /api/panel/bots/{id}", s.hostOnly(s.handleBotDelete))
	mux.HandleFunc("GET /api/panel/bots/{id}/qr.png", s.hostOnly(s.handleBotQR))
	mux.HandleFunc("POST /api/panel/bots/{id}/pairing/retry", s.hostOnly(s.handleBotRetry))
	mux.HandleFunc("POST /api/panel/bots/{id}/logout", s.hostOnly(s.handleBotLogout))
	mux.HandleFunc("GET /api/panel/events", s.authenticated(s.handleEvents))

	mux.HandleFunc("GET /api/panel/invites", s.hostOnly(s.handleInviteList))
	mux.HandleFunc("POST /api/panel/invites", s.hostOnly(s.handleInviteCreate))
	mux.HandleFunc("DELETE /api/panel/invites/{id}", s.hostOnly(s.handleInviteDelete))
	mux.HandleFunc("GET /api/panel/sessions", s.hostOnly(s.handleSessionList))
	mux.HandleFunc("DELETE /api/panel/sessions/{id}", s.hostOnly(s.handleSessionRevoke))

	mux.HandleFunc("/", s.handleRoot)
	if s.cfg.Hooks != nil {
		mux.Handle("/hook/", s.cfg.Hooks)
		if s.cfg.Debug {
			mux.Handle("/debug/", s.cfg.Hooks)
		}
	}

	mux.HandleFunc("GET /manifest.webmanifest", s.handleManifest)
	mux.HandleFunc("GET /sw.js", s.handleServiceWorker)
	mux.HandleFunc("GET /icon.png", s.handleIcon)
	return mux
}

// Run listens on the loopback port until ctx is cancelled.
func (s *Server) Run(ctx context.Context) error {
	if s.cfg.Port == 0 {
		return nil
	}
	go s.syncLoop(ctx)
	server := &http.Server{
		Addr:              net.JoinHostPort("127.0.0.1", strconv.Itoa(s.cfg.Port)),
		Handler:           s.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	s.log.Printf("Panel server listening on http://127.0.0.1:%d/", s.cfg.Port)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// identity is the resolved caller.
type identity struct {
	Role    string
	Local   bool
	Session auth.Session
}

// identify resolves the caller: local requests are the host, forwarded or LAN
// requests need a session.
func (s *Server) identify(r *http.Request) identity {
	if isLocalRequest(r) {
		return identity{Role: auth.RoleHost, Local: true}
	}
	if s.cfg.Auth == nil {
		return identity{}
	}
	token := sessionToken(r)
	if token == "" {
		return identity{}
	}
	session, ok := s.cfg.Auth.Authenticate(token)
	if !ok {
		return identity{}
	}
	return identity{Role: session.Role, Session: session}
}

// hostOnly allows the host only.
func (s *Server) hostOnly(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := s.identify(r)
		switch {
		case id.Role == auth.RoleHost:
			next(w, r)
		case id.Role == "":
			writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "Unauthorized"})
		default:
			writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "Host only"})
		}
	}
}

// authenticated allows the host and any spectator session.
func (s *Server) authenticated(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.identify(r).Role == "" {
			writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "Unauthorized"})
			return
		}
		next(w, r)
	}
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	path := ""
	if s.cfg.Settings != nil {
		path = s.cfg.Settings.Path()
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":       true,
		"panel":    true,
		"hooks":    s.cfg.Hooks != nil,
		"auth":     s.cfg.Auth != nil,
		"settings": path,
		"instance": s.cfg.InstanceID,
		"machine":  s.cfg.Machine,
	})
}

// handleSessionGet reports the caller's own access, so the panel can show the
// invite screen when needed. It never leaks another user's data.
func (s *Server) handleSessionGet(w http.ResponseWriter, r *http.Request) {
	id := s.identify(r)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"authenticated": id.Role != "",
		"role":          id.Role,
		"local":         id.Local,
		"label":         id.Session.Label,
	})
}

// handleSessionCreate redeems an invite for a session cookie.
func (s *Server) handleSessionCreate(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Auth unavailable"})
		return
	}
	var payload struct {
		Token string `json:"token"`
		Label string `json:"label"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Invalid JSON body"})
		return
	}
	session, token, err := s.cfg.Auth.Redeem(payload.Token, payload.Label)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "Invitación no válida"})
		return
	}
	setSessionCookie(w, r, token, session.ExpiresAt)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "role": session.Role, "label": session.Label})
}

// handleSessionDelete logs a spectator out.
func (s *Server) handleSessionDelete(w http.ResponseWriter, r *http.Request) {
	id := s.identify(r)
	if id.Role == auth.RoleSpectator && id.Session.ID != "" && s.cfg.Auth != nil {
		_ = s.cfg.Auth.RevokeSession(id.Session.ID)
	}
	clearSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleSettingsGet(w http.ResponseWriter, _ *http.Request) {
	if s.cfg.Settings == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Settings unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, s.cfg.Settings.Values().Redacted())
}

func (s *Server) handleSettingsPatch(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Settings == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Settings unavailable"})
		return
	}
	var patch settingsPatch
	if err := decodeJSON(r, &patch); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Invalid JSON body"})
		return
	}
	updated := applyPatch(s.cfg.Settings.Values(), patch)
	if err := s.cfg.Settings.Update(updated); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	if s.cfg.OnChange != nil {
		s.cfg.OnChange()
	}
	writeJSON(w, http.StatusOK, updated.Redacted())
}

// handleTunnel reports the named tunnel state.
func (s *Server) handleTunnel(w http.ResponseWriter, _ *http.Request) {
	if s.cfg.Tunnel == nil {
		writeJSON(w, http.StatusOK, TunnelInfo{Status: "disabled"})
		return
	}
	writeJSON(w, http.StatusOK, s.cfg.Tunnel())
}

// handleTunnelLogin starts the one-time cloudflared login (host only).
func (s *Server) handleTunnelLogin(w http.ResponseWriter, _ *http.Request) {
	if s.cfg.TunnelLogin == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Túnel no configurado"})
		return
	}
	if err := s.cfg.TunnelLogin(); err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "started": true})
}

// NextSendInfo is the next scheduled notification, so the panel can show it.
type NextSendInfo struct {
	Time    string `json:"time"`
	Targets int    `json:"targets"`
}

// handleAutostartGet reports whether the app opens at login.
func (s *Server) handleAutostartGet(w http.ResponseWriter, _ *http.Request) {
	enabled := false
	if s.cfg.Autostart != nil {
		enabled = s.cfg.Autostart()
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": enabled})
}

// handleAutostartSet enables or disables opening at login.
func (s *Server) handleAutostartSet(w http.ResponseWriter, r *http.Request) {
	if s.cfg.SetAutostart == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "No disponible"})
		return
	}
	var payload struct {
		Enabled bool `json:"enabled"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Invalid JSON body"})
		return
	}
	if err := s.cfg.SetAutostart(payload.Enabled); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": payload.Enabled})
}

// handleDiagnostics returns the running configuration and the recent logs.
func (s *Server) handleDiagnostics(w http.ResponseWriter, _ *http.Request) {
	if s.cfg.Diagnostics == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "No disponible"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "diagnostics": s.cfg.Diagnostics()})
}

// handleLogDownload sends app.log (and the rotated app.log.1 before it) as a
// single text attachment.
func (s *Server) handleLogDownload(w http.ResponseWriter, _ *http.Request) {
	if s.cfg.LogPath == "" {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "No disponible"})
		return
	}
	var out []byte
	if old, err := os.ReadFile(s.cfg.LogPath + ".1"); err == nil {
		out = append(out, old...)
	}
	current, err := os.ReadFile(s.cfg.LogPath)
	if err != nil && len(out) == 0 {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "Todavía no hay registros guardados"})
		return
	}
	out = append(out, current...)
	name := "palorosa-registros-" + time.Now().Format("2006-01-02-1504") + ".log"
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Disposition", `attachment; filename="`+name+`"`)
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(out)
}

// handleLogOpen opens the log folder in the file explorer of the host machine.
func (s *Server) handleLogOpen(w http.ResponseWriter, _ *http.Request) {
	if s.cfg.OpenLogFolder == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "No disponible"})
		return
	}
	if err := s.cfg.OpenLogFolder(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleSelfTestList returns every check with its last result.
func (s *Server) handleSelfTestList(w http.ResponseWriter, _ *http.Request) {
	if s.cfg.SelfTest == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "No disponible"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "checks": s.cfg.SelfTest.Last()})
}

// handleSelfTestRunAll runs every check now.
func (s *Server) handleSelfTestRunAll(w http.ResponseWriter, r *http.Request) {
	if s.cfg.SelfTest == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "No disponible"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "checks": s.cfg.SelfTest.RunAll(r.Context())})
}

// handleSelfTestRun runs one check now.
func (s *Server) handleSelfTestRun(w http.ResponseWriter, r *http.Request) {
	if s.cfg.SelfTest == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "No disponible"})
		return
	}
	result, ok := s.cfg.SelfTest.Run(r.Context(), r.PathValue("id"))
	if !ok {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "Prueba desconocida"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "check": result})
}

// stateResponse is the panel read model plus the caller's access.
type stateResponse struct {
	OK       bool                `json:"ok"`
	Role     string              `json:"role"`
	Local    bool                `json:"local"`
	Tunnel   TunnelInfo          `json:"tunnel"`
	Snapshot panelmodel.Snapshot `json:"snapshot"`
	NextSend *NextSendInfo       `json:"nextSend,omitempty"`
}

// bogota is the kitchen timezone.
var bogota = func() *time.Location {
	if location, err := time.LoadLocation("America/Bogota"); err == nil {
		return location
	}
	return time.FixedZone("America/Bogota", -5*60*60)
}()

func (s *Server) nextSend() *NextSendInfo {
	if s.cfg.Targets == nil {
		return nil
	}
	when, targets, ok := s.cfg.Targets.NextSend(time.Now())
	if !ok {
		return nil
	}
	return &NextSendInfo{Time: when.In(bogota).Format("15:04"), Targets: targets}
}

func (s *Server) snapshot() panelmodel.Snapshot {
	snapshot := panelmodel.Snapshot{}
	if s.cfg.Snapshot != nil {
		snapshot = s.cfg.Snapshot()
	}
	// Normalize nil slices so JSON never carries null where the panel expects a
	// list.
	if snapshot.Days == nil {
		snapshot.Days = []panelmodel.Day{}
	}
	if snapshot.WhatsApp.Allowlist == nil {
		snapshot.WhatsApp.Allowlist = []string{}
	}
	for index := range snapshot.Days {
		if snapshot.Days[index].Orders == nil {
			snapshot.Days[index].Orders = []panelmodel.Order{}
		}
		if snapshot.Days[index].List.Entries == nil {
			snapshot.Days[index].List.Entries = []panelmodel.Entry{}
		}
	}
	return snapshot
}

func (s *Server) tunnelInfo() TunnelInfo {
	if s.cfg.Tunnel == nil {
		return TunnelInfo{Status: "disabled"}
	}
	return s.cfg.Tunnel()
}

// handleState is the single read model the panel boots from.
func (s *Server) handleState(w http.ResponseWriter, r *http.Request) {
	id := s.identify(r)
	writeJSON(w, http.StatusOK, stateResponse{
		OK:       true,
		Role:     id.Role,
		Local:    id.Local,
		Tunnel:   s.tunnelInfo(),
		Snapshot: s.snapshot(),
		NextSend: s.nextSend(),
	})
}

// handleList returns the aggregated kitchen list for a date. A date without a
// list is an empty list, not an error, so the panel shows its empty state.
func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	date := dateParam(r)
	day := s.snapshot().Day(date)
	list := panelmodel.List{Entries: []panelmodel.Entry{}}
	if day != nil {
		list = day.List
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "date": date, "list": list})
}

// handleOrders returns the orders of a date.
func (s *Server) handleOrders(w http.ResponseWriter, r *http.Request) {
	date := dateParam(r)
	day := s.snapshot().Day(date)
	orders := []panelmodel.Order{}
	if day != nil {
		orders = day.Orders
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "date": date, "orders": orders})
}

// maxOrderDetails caps one order-details request so a merged group cannot
// make the server resolve an unbounded list.
const maxOrderDetails = 1000

// handleOrderDetails resolves the requested orders of a date. The panel calls
// it lazily: one request per expanded ticket, or one for a whole attached
// group.
func (s *Server) handleOrderDetails(w http.ResponseWriter, r *http.Request) {
	if s.cfg.OrderDetail == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Sin bot"})
		return
	}
	date := dateParam(r)
	orders := []panelmodel.OrderDetail{}
	for _, value := range strings.Split(r.URL.Query().Get("numbers"), ",") {
		number := strings.TrimSpace(value)
		if number == "" {
			continue
		}
		if len(orders) >= maxOrderDetails {
			break
		}
		detail, err := s.cfg.OrderDetail(date, number)
		if err != nil {
			continue
		}
		orders = append(orders, detail)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "date": date, "orders": orders})
}

// handleRemoveOrder drops an order from a day (host only).
func (s *Server) handleRemoveOrder(w http.ResponseWriter, r *http.Request) {
	if s.cfg.RemoveOrder == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Sin bot"})
		return
	}
	date := r.PathValue("date")
	number := r.PathValue("number")
	if err := s.cfg.RemoveOrder(date, number); err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.Publish("orders")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleNotifications updates the global notification control (host only).
func (s *Server) handleNotifications(w http.ResponseWriter, r *http.Request) {
	if s.cfg.SetNotifications == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Sin bot"})
		return
	}
	var notifications panelmodel.Notifications
	if err := decodeJSON(r, &notifications); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Invalid JSON body"})
		return
	}
	if err := s.cfg.SetNotifications(notifications); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.Publish("settings")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleTargets lists the notification targets.
func (s *Server) handleTargets(w http.ResponseWriter, _ *http.Request) {
	targets := []notify.Target{}
	if s.cfg.Targets != nil {
		targets = s.cfg.Targets.Targets()
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "targets": targets})
}

// normalizePhone keeps the digits of a phone number. Targets store the JID
// user form ("573001112233") no matter how the operator typed the number, so
// the sender can address it (including the bot's own number for loopback).
func normalizePhone(value string) string {
	var builder strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			builder.WriteRune(r)
		}
	}
	return builder.String()
}

// handleTargetCreate adds a target (host only).
func (s *Server) handleTargetCreate(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Targets == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Sin gestor de avisos"})
		return
	}
	var target notify.Target
	if err := decodeJSON(r, &target); err != nil || normalizePhone(target.Phone) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Falta el número"})
		return
	}
	target.Phone = normalizePhone(target.Phone)
	created, err := s.cfg.Targets.AddTarget(target)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.Publish("settings")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "target": created})
}

// handleTargetUpdate edits a target (host only).
func (s *Server) handleTargetUpdate(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Targets == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Sin gestor de avisos"})
		return
	}
	var target notify.Target
	if err := decodeJSON(r, &target); err != nil || normalizePhone(target.Phone) == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Falta el número"})
		return
	}
	target.ID = r.PathValue("id")
	target.Phone = normalizePhone(target.Phone)
	if err := s.cfg.Targets.UpdateTarget(target); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.Publish("settings")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "target": target})
}

// handleTargetDelete removes a target (host only).
func (s *Server) handleTargetDelete(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Targets == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Sin gestor de avisos"})
		return
	}
	if err := s.cfg.Targets.RemoveTarget(r.PathValue("id")); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.Publish("settings")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleTargetTest sends a test message to a target (host only).
func (s *Server) handleTargetTest(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Targets == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Sin gestor de avisos"})
		return
	}
	if err := s.cfg.Targets.Test(r.PathValue("id")); err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleTargetTestAll sends a test message to every enabled target.
func (s *Server) handleTargetTestAll(w http.ResponseWriter, _ *http.Request) {
	if s.cfg.Targets == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Sin gestor de avisos"})
		return
	}
	sent := s.cfg.Targets.TestAll()
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sent": sent})
}

// handleListPublish recomputes and stores the kitchen list for a date.
func (s *Server) handleListPublish(w http.ResponseWriter, r *http.Request) {
	publish := s.cfg.PublishList
	if publish == nil && s.cfg.Bots != nil {
		publish = s.cfg.Bots.PublishList
	}
	if publish == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Cocina no disponible"})
		return
	}
	date := dateParam(r)
	if err := publish(date); err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.Publish("orders")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "date": date})
}

// handlePushGet returns the VAPID public key a browser needs to subscribe.
func (s *Server) handlePushGet(w http.ResponseWriter, _ *http.Request) {
	if s.cfg.Push == nil {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": false, "publicKey": "", "count": 0})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "enabled": true, "publicKey": s.cfg.Push.PublicKey(), "count": s.cfg.Push.Count()})
}

// handlePushCreate stores a browser push subscription.
func (s *Server) handlePushCreate(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Push == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Sin push"})
		return
	}
	var subscription push.Subscription
	if err := decodeJSON(r, &subscription); err != nil || subscription.Endpoint == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Suscripción inválida"})
		return
	}
	if err := s.cfg.Push.Subscribe(subscription); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handlePushDelete removes a browser push subscription.
func (s *Server) handlePushDelete(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Push == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Sin push"})
		return
	}
	var payload struct {
		Endpoint string `json:"endpoint"`
	}
	if err := decodeJSON(r, &payload); err != nil || payload.Endpoint == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Falta el endpoint"})
		return
	}
	if err := s.cfg.Push.Unsubscribe(payload.Endpoint); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// botView is a bot plus its live status.
type botView struct {
	bots.Bot
	Status string `json:"status"`
}

// handleBots lists the accounts with their status.
func (s *Server) handleBots(w http.ResponseWriter, _ *http.Request) {
	views := []botView{}
	if s.cfg.Bots != nil {
		for _, bot := range s.cfg.Bots.Bots() {
			view := botView{Bot: bot, Status: s.cfg.Bots.Status(bot.ID)}
			// The number only lives in the live session; the registry can be
			// empty even while the account is connected.
			if phone := s.cfg.Bots.Phone(bot.ID); phone != "" {
				view.Bot.Phone = phone
			}
			views = append(views, view)
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "bots": views})
}

// handleBotCreate registers an account (host only).
func (s *Server) handleBotCreate(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Bots == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Sin gestor de bots"})
		return
	}
	var bot bots.Bot
	if err := decodeJSON(r, &bot); err != nil || bot.Name == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Falta el nombre"})
		return
	}
	created, err := s.cfg.Bots.Add(bot)
	if err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.Publish("whatsapp")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "bot": created})
}

// handleBotDelete removes an account (host only).
func (s *Server) handleBotDelete(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Bots == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Sin gestor de bots"})
		return
	}
	if err := s.cfg.Bots.Remove(r.PathValue("id")); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.Publish("whatsapp")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleBotUpdate edits an account: rename, enable/disable and allowlist.
func (s *Server) handleBotUpdate(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Bots == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Sin gestor de bots"})
		return
	}
	var bot bots.Bot
	if err := decodeJSON(r, &bot); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Invalid JSON body"})
		return
	}
	bot.ID = r.PathValue("id")
	updated, err := s.cfg.Bots.Update(bot)
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.Publish("whatsapp")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "bot": updated})
}

// handleBotRetry starts a fresh QR cycle for an unlinked account.
func (s *Server) handleBotRetry(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Bots == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Sin gestor de bots"})
		return
	}
	if err := s.cfg.Bots.RetryPairing(r.PathValue("id")); err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.Publish("whatsapp")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleBotLogout unlinks an account's WhatsApp session.
func (s *Server) handleBotLogout(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Bots == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Sin gestor de bots"})
		return
	}
	if err := s.cfg.Bots.Logout(r.PathValue("id")); err != nil {
		writeJSON(w, http.StatusConflict, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	s.Publish("whatsapp")
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// handleBotQR serves the current pairing QR of an account as a PNG.
func (s *Server) handleBotQR(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Bots == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Sin gestor de bots"})
		return
	}
	png, err := s.cfg.Bots.QRPNG(r.PathValue("id"))
	if err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "Sin código QR"})
		return
	}
	w.Header().Set("content-type", "image/png")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(png)
}

// handleEvents is the server-sent events stream the panel listens to.
func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": "Streaming unsupported"})
		return
	}
	header := w.Header()
	header.Set("content-type", "text/event-stream")
	header.Set("cache-control", "no-cache")
	header.Set("connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
	_, _ = fmt.Fprint(w, "event: hello\ndata: {}\n\n")
	flusher.Flush()

	id, channel := s.subscribe()
	defer s.unsubscribe(id)

	keepalive := time.NewTicker(25 * time.Second)
	defer keepalive.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-keepalive.C:
			_, _ = fmt.Fprint(w, ": ping\n\n")
			flusher.Flush()
		case event := <-channel:
			_, _ = fmt.Fprintf(w, "event: %s\ndata: {}\n\n", event)
			flusher.Flush()
		}
	}
}

func (s *Server) handleManifest(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("content-type", "application/manifest+json")
	_, _ = w.Write([]byte(`{
  "name": "Panel de cocina Palorosa",
  "short_name": "Palorosa",
  "start_url": "/",
  "scope": "/",
  "display": "standalone",
  "background_color": "#f9eadc",
  "theme_color": "#f9eadc",
  "icons": [
    { "src": "/icon.png", "sizes": "1024x1024", "type": "image/png", "purpose": "any" }
  ]
}`))
}

func (s *Server) handleServiceWorker(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("content-type", "text/javascript")
	w.Header().Set("service-worker-allowed", "/")
	_, _ = w.Write([]byte(`const CACHE = 'palorosa-panel-v1'
self.addEventListener('install', () => self.skipWaiting())
self.addEventListener('activate', (event) => event.waitUntil(self.clients.claim()))
self.addEventListener('push', (event) => {
  let body = 'Hay novedades en la cocina.'
  try {
    const data = event.data && event.data.json()
    if (data && data.body) body = data.body
  } catch (e) {}
  event.waitUntil(self.registration.showNotification('Panel de cocina', {
    body: body,
    icon: '/icon.png',
    badge: '/icon.png',
  }))
})
self.addEventListener('notificationclick', (event) => {
  event.notification.close()
  event.waitUntil(self.clients.matchAll({ type: 'window', includeUncontrolled: true }).then((list) => {
    for (const client of list) {
      if ('focus' in client) return client.focus()
    }
    return self.clients.openWindow('/')
  }))
})
self.addEventListener('fetch', (event) => {
  const request = event.request
  const url = new URL(request.url)
  if (request.method !== 'GET' || url.pathname.startsWith('/api/') || url.pathname.startsWith('/hook/')) return
  event.respondWith(
    fetch(request)
      .then((response) => {
        const copy = response.clone()
        caches.open(CACHE).then((cache) => cache.put(request, copy)).catch(() => {})
        return response
      })
      .catch(() => caches.match(request).then((cached) => cached || caches.match('/')))
  )
})`))
}

func (s *Server) handleIcon(w http.ResponseWriter, _ *http.Request) {
	if len(s.cfg.PanelIcon) == 0 {
		http.NotFound(w, nil)
		return
	}
	w.Header().Set("content-type", "image/png")
	_, _ = w.Write(s.cfg.PanelIcon)
}

// dateParam reads ?date=YYYY-MM-DD or falls back to today in Bogota.
func dateParam(r *http.Request) string {
	date := r.URL.Query().Get("date")
	if len(date) == 10 {
		return date
	}
	return time.Now().UTC().Add(-5 * time.Hour).Format("2006-01-02")
}

func (s *Server) handleInviteList(w http.ResponseWriter, _ *http.Request) {
	if s.cfg.Auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Auth unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "invites": s.cfg.Auth.ListInvites()})
}

func (s *Server) handleInviteCreate(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Auth unavailable"})
		return
	}
	var payload struct {
		Label      string `json:"label"`
		TTLMinutes int    `json:"ttlMinutes"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Invalid JSON body"})
		return
	}
	ttl := time.Duration(payload.TTLMinutes) * time.Minute
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	invite, token, err := s.cfg.Auth.CreateInvite(payload.Label, ttl)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	invite.TokenHash = ""
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":     true,
		"invite": invite,
		"token":  token,
		"url":    s.inviteURL(r, token),
	})
}

func (s *Server) handleInviteDelete(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Auth unavailable"})
		return
	}
	if err := s.cfg.Auth.RevokeInvite(r.PathValue("id")); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "Invitación no encontrada"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) handleSessionList(w http.ResponseWriter, _ *http.Request) {
	if s.cfg.Auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Auth unavailable"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "sessions": s.cfg.Auth.ListSessions()})
}

func (s *Server) handleSessionRevoke(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Auth == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "Auth unavailable"})
		return
	}
	if err := s.cfg.Auth.RevokeSession(r.PathValue("id")); err != nil {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "Sesión no encontrada"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) inviteURL(r *http.Request, token string) string {
	base := strings.TrimRight(s.cfg.PanelBaseURL, "/")
	if base == "" {
		scheme := "http"
		if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
			scheme = "https"
		}
		base = scheme + "://" + r.Host
	}
	return base + "/?invite=" + url.QueryEscape(token)
}

// handleRoot serves the built panel, or a placeholder when it is not built.
func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		writeJSON(w, http.StatusNotFound, map[string]any{"ok": false, "error": "Not found"})
		return
	}
	w.Header().Set("content-type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusOK)
	if len(s.cfg.Panel) > 0 {
		_, _ = w.Write(s.cfg.Panel)
		return
	}
	_, _ = w.Write([]byte(`<!doctype html>
<html lang="es"><head><meta charset="utf-8"><title>Panel Palorosa</title>
<style>body{font-family:system-ui,sans-serif;background:#f9eadc;color:#6a4b20;display:grid;place-items:center;min-height:100vh;margin:0}
main{background:#fffaf5;border:1px solid #f3d1c7;border-radius:10px;padding:24px 28px;max-width:520px}
h1{font-size:18px;margin:0 0 8px}p{margin:0;color:#8a6a54}code{background:#f6d5cd;padding:.15rem .4rem;border-radius:.3rem}</style></head>
<body><main><h1>Panel de la cocina</h1>
<p>El panel no est&aacute; compilado. Ejecuta <code>pnpm build:panel</code> y reinicia.</p></main></body></html>`))
}

// settingsPatch is a partial settings update: only the fields present in the
// JSON are applied, so a missing key keeps the stored value and an explicit
// empty string clears it.
type settingsPatch struct {
	Domain         *string      `json:"domain"`
	TunnelHostname *string      `json:"tunnelHostname"`
	TunnelToken    *string      `json:"tunnelToken"`
	CatalogPath    *string      `json:"catalogPath"`
	PanelPort      *int         `json:"panelPort"`
	Debug          *bool        `json:"debug"`
	WP             *wpPatch     `json:"wp"`
	WebhookSecret  *string      `json:"webhookSecret"`
	Access         *accessPatch `json:"access"`
	SyncMinutes    *int         `json:"syncMinutes"`
}

type wpPatch struct {
	AdminURL       *string `json:"adminUrl"`
	AdminUser      *string `json:"adminUser"`
	AdminPassword  *string `json:"adminPassword"`
	ConsumerKey    *string `json:"consumerKey"`
	ConsumerSecret *string `json:"consumerSecret"`
	ExportID       *string `json:"exportId"`
	ExportCronKey  *string `json:"exportCronKey"`
}

type accessPatch struct {
	Enabled *bool `json:"enabled"`
}

func applyPatch(current settings.Values, patch settingsPatch) settings.Values {
	out := current
	if patch.Domain != nil {
		out.Domain = *patch.Domain
	}
	if patch.TunnelHostname != nil {
		out.TunnelHostname = *patch.TunnelHostname
	}
	if patch.TunnelToken != nil {
		out.TunnelToken = *patch.TunnelToken
	}
	if patch.CatalogPath != nil {
		out.CatalogPath = *patch.CatalogPath
	}
	if patch.PanelPort != nil && *patch.PanelPort > 0 && *patch.PanelPort <= 65535 {
		out.PanelPort = *patch.PanelPort
	}
	if patch.Debug != nil {
		out.Debug = *patch.Debug
	}
	if patch.WebhookSecret != nil {
		out.WebhookSecret = *patch.WebhookSecret
	}
	if patch.SyncMinutes != nil && *patch.SyncMinutes >= 0 && *patch.SyncMinutes <= 1440 {
		minutes := *patch.SyncMinutes
		out.SyncMinutes = &minutes
	}
	if patch.Access != nil && patch.Access.Enabled != nil {
		out.Access.Enabled = *patch.Access.Enabled
	}
	if patch.WP != nil {
		if patch.WP.AdminURL != nil {
			out.WP.AdminURL = *patch.WP.AdminURL
		}
		if patch.WP.AdminUser != nil {
			out.WP.AdminUser = *patch.WP.AdminUser
		}
		if patch.WP.AdminPassword != nil {
			out.WP.AdminPassword = *patch.WP.AdminPassword
		}
		if patch.WP.ConsumerKey != nil {
			out.WP.ConsumerKey = *patch.WP.ConsumerKey
		}
		if patch.WP.ConsumerSecret != nil {
			out.WP.ConsumerSecret = *patch.WP.ConsumerSecret
		}
		if patch.WP.ExportID != nil {
			out.WP.ExportID = *patch.WP.ExportID
		}
		if patch.WP.ExportCronKey != nil {
			out.WP.ExportCronKey = *patch.WP.ExportCronKey
		}
	}
	return out
}

// isLocalRequest reports a direct local connection. A request that carries
// Cloudflare or proxy forwarding headers is treated as remote even though
// cloudflared connects from loopback.
func isLocalRequest(r *http.Request) bool {
	if r.Header.Get("Cf-Connecting-Ip") != "" || r.Header.Get("CF-Ray") != "" || r.Header.Get("X-Forwarded-For") != "" {
		return false
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func sessionToken(r *http.Request) string {
	if cookie, err := r.Cookie(sessionCookie); err == nil && cookie.Value != "" {
		return cookie.Value
	}
	if header := r.Header.Get("Authorization"); strings.HasPrefix(header, "Bearer ") {
		return strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
	}
	return ""
}

func setSessionCookie(w http.ResponseWriter, r *http.Request, token string, expires time.Time) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    token,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"),
		Expires:  expires,
	})
}

func clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https"),
		MaxAge:   -1,
	})
}

func decodeJSON(r *http.Request, target any) error {
	err := json.NewDecoder(r.Body).Decode(target)
	if errors.Is(err, io.EOF) {
		return nil
	}
	return err
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("content-type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
