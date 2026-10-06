//go:build windows

package main

import (
	"bytes"
	"crypto/rand"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/config"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/panelserver"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/settings"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/window"
)

//go:embed assets/handoff.html
var handoffHTML []byte

// handoffNonce guards the local page binding: a page served later by the host
// (remote content) does not know it and cannot drive a takeover.
var handoffMu sync.Mutex

var handoffNonce string

// showHandoff opens the local "take over" page in the window.
func showHandoff() {
	nonce, err := randomHex(16)
	if err != nil {
		fmt.Fprintln(os.Stderr, "handoff:", err)
		return
	}
	handoffMu.Lock()
	handoffNonce = nonce
	handoffMu.Unlock()
	window.SetHtml(strings.ReplaceAll(string(handoffHTML), "{{nonce}}", nonce))
	window.Show()
}

// bindHandoff exposes the local transfer page to Go. Call it once when the app
// starts in spectator mode.
func bindHandoff() {
	if err := window.Bind("palorosaHandoff", func(nonce, code string) (string, error) {
		handoffMu.Lock()
		expected := handoffNonce
		handoffMu.Unlock()
		if expected == "" || nonce != expected {
			return "", errors.New("acción no autorizada")
		}
		if strings.TrimSpace(code) == "" {
			// "Volver": the operator changed their mind.
			window.Navigate(spectatorURL(config.Load()))
			return "", nil
		}
		return performHandoff(code)
	}); err != nil {
		fmt.Fprintln(os.Stderr, "handoff bind:", err)
	}
}

// performHandoff claims the host code, downloads the whole kitchen state and
// installs it here. On success the app relaunches as the host.
func performHandoff(code string) (string, error) {
	cfg := config.Load()
	client, base, err := handoffSession(cfg)
	if err != nil {
		return "", err
	}
	var claim struct {
		Token string                      `json:"token"`
		Host  panelserver.HandoffIdentity `json:"host"`
	}
	if err := handoffCall(client, http.MethodPost, base+"/api/panel/handoff/claim", map[string]string{"code": strings.TrimSpace(code)}, &claim); err != nil {
		return "", err
	}
	if claim.Token == "" {
		return "", errors.New("el host no entregó un permiso")
	}
	var bundle panelserver.HandoffBundle
	if err := handoffCall(client, http.MethodGet, base+"/api/panel/handoff/bundle?token="+url.QueryEscape(claim.Token), nil, &bundle); err != nil {
		return "", fmt.Errorf("no pudimos descargar la configuración: %w", err)
	}
	if err := applyHandoffBundle(cfg, bundle); err != nil {
		return "", err
	}
	// Ask the old host to step down, then wait for its tunnel to stop before
	// this machine opens the same one.
	_ = handoffCall(client, http.MethodPost, base+"/api/panel/handoff/complete", map[string]string{"token": claim.Token}, nil)
	waitForHostDown(base, 60*time.Second, claim.Host.Instance)
	// The imported settings point at this machine's catalog copy.
	restartApp()
	return claim.Host.Version, nil
}

// handoffSession signs this app in at the host with the station key and
// returns a client that keeps the session cookie.
func handoffSession(cfg config.Config) (*http.Client, string, error) {
	if cfg.TunnelHostname == "" {
		return nil, "", errors.New("este equipo no tiene el host configurado")
	}
	secret := cfg.StationSecret
	if secret == "" {
		secret = cfg.WebhookSecret
	}
	key := panelserver.StationKey(secret)
	if key == "" {
		return nil, "", errors.New("este equipo no tiene la clave del host")
	}
	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, "", err
	}
	client := &http.Client{
		Jar:     jar,
		Timeout: 90 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	base := "https://" + cfg.TunnelHostname
	label, _ := os.Hostname()
	if label == "" {
		label = "Estación"
	}
	endpoint := base + "/api/panel/station?key=" + url.QueryEscape(key) + "&label=" + url.QueryEscape(label)
	response, err := client.Get(endpoint)
	if err != nil {
		return nil, "", fmt.Errorf("no pudimos llegar al host: %w", err)
	}
	response.Body.Close()
	if !hasSessionCookie(client, base) {
		return nil, "", errors.New("este equipo ya no tiene la clave del host; pide una invitación nueva")
	}
	return client, base, nil
}

// hasSessionCookie reports whether the station call set the session cookie.
func hasSessionCookie(client *http.Client, base string) bool {
	parsed, err := url.Parse(base)
	if err != nil {
		return false
	}
	for _, cookie := range client.Jar.Cookies(parsed) {
		if cookie.Name == "palorosa_session" && cookie.Value != "" {
			return true
		}
	}
	return false
}

// handoffCall performs a JSON request and decodes the JSON response. Server
// errors arrive as {"error": "..."}.
func handoffCall(client *http.Client, method, endpoint string, body any, out any) error {
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(raw)
	}
	request, err := http.NewRequest(method, endpoint, reader)
	if err != nil {
		return err
	}
	if body != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	response, err := client.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 8<<20))
	if err != nil {
		return err
	}
	if response.StatusCode < 200 || response.StatusCode > 299 {
		var failure struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(raw, &failure)
		if failure.Error != "" {
			return errors.New(failure.Error)
		}
		return fmt.Errorf("HTTP %d", response.StatusCode)
	}
	if out == nil || len(raw) == 0 {
		return nil
	}
	return json.Unmarshal(raw, out)
}

// applyHandoffBundle writes the downloaded state to this machine. WhatsApp
// sessions never arrive, so the accounts stay unlinked until they are paired
// again with their QR.
func applyHandoffBundle(cfg config.Config, bundle panelserver.HandoffBundle) error {
	if bundle.Format != "palorosa-kitchen/handoff" {
		return errors.New("el host envió un paquete desconocido")
	}
	files := []struct {
		name string
		raw  json.RawMessage
	}{
		{"catalog.json", bundle.Catalog},
		{"print-template.json", bundle.PrintTemplate},
		{"targets.json", bundle.Targets},
		{"bots.json", bundle.Bots},
	}
	for _, file := range files {
		if len(file.raw) == 0 {
			continue
		}
		if err := os.WriteFile(filepath.Join(cfg.DataDir, file.name), file.raw, 0o644); err != nil {
			return fmt.Errorf("guardar %s: %w", file.name, err)
		}
	}
	store, err := settings.Open(cfg.SettingsPath)
	if err != nil {
		return fmt.Errorf("leer la configuración: %w", err)
	}
	values := bundle.Settings
	// The catalog travels in the package; point at this machine's copy.
	values.CatalogPath = filepath.Join(cfg.DataDir, "catalog.json")
	if err := store.Update(values); err != nil {
		return fmt.Errorf("guardar la configuración: %w", err)
	}
	return nil
}

// waitForHostDown waits until the tunnel stops answering as the old host, so
// the same tunnel token never runs on two machines at once.
func waitForHostDown(base string, timeout time.Duration, instance string) {
	client := &http.Client{Timeout: 5 * time.Second}
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		response, err := client.Get(base + "/health")
		if err != nil {
			return
		}
		var health struct {
			Instance string `json:"instance"`
		}
		_ = json.NewDecoder(io.LimitReader(response.Body, 4096)).Decode(&health)
		response.Body.Close()
		if response.StatusCode != http.StatusOK || (instance != "" && health.Instance != instance) {
			return
		}
		time.Sleep(2 * time.Second)
	}
}

// randomHex returns n random bytes as hex, for the page nonce.
func randomHex(n int) (string, error) {
	buffer := make([]byte, n)
	if _, err := rand.Read(buffer); err != nil {
		return "", err
	}
	return hex.EncodeToString(buffer), nil
}
