package panelserver

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/auth"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/settings"
)

// TestEndToEndInviteAndRoles drives the whole flow over real HTTP: the host
// mints an invite, a remote spectator (simulated with a Cloudflare forwarding
// header) redeems it for a session cookie, reads what it may and is rejected
// from what it may not.
func TestEndToEndInviteAndRoles(t *testing.T) {
	dir := t.TempDir()
	store, err := settings.Open(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(settings.Values{Domain: "palorosabreakfast.com", WP: settings.WP{AdminPassword: "super-secret"}}); err != nil {
		t.Fatal(err)
	}
	authManager, err := auth.New(filepath.Join(dir, "sessions.json"))
	if err != nil {
		t.Fatal(err)
	}
	server := New(Config{
		Port:         5211,
		Settings:     store,
		Auth:         authManager,
		PanelBaseURL: "https://cocina.whitesu.dev",
		Snapshot:     sampleSnapshot,
	})
	testServer := httptest.NewServer(server.Handler())
	defer testServer.Close()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	// 1. Host (a direct local request) mints an invite.
	response := do(t, client, http.MethodPost, testServer.URL+"/api/panel/invites", `{"label":"Marta"}`, false)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("invite status = %d", response.StatusCode)
	}
	var invite struct {
		Token string `json:"token"`
		URL   string `json:"url"`
	}
	decodeBody(t, response, &invite)
	if invite.Token == "" || invite.URL == "" {
		t.Fatalf("empty invite: %+v", invite)
	}

	// 2. Anonymous remote cannot read the settings.
	response = do(t, client, http.MethodGet, testServer.URL+"/api/panel/settings", "", true)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous remote = %d, want 401", response.StatusCode)
	}

	// 3. A remote spectator redeems the invite for a session cookie.
	response = do(t, client, http.MethodPost, testServer.URL+"/api/panel/session", `{"token":"`+invite.Token+`"}`, true)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("redeem = %d", response.StatusCode)
	}
	response.Body.Close()

	// 4. The same cookie now identifies a remote spectator.
	response = do(t, client, http.MethodGet, testServer.URL+"/api/panel/session", "", true)
	var who struct {
		Role  string `json:"role"`
		Local bool   `json:"local"`
	}
	decodeBody(t, response, &who)
	if who.Role != auth.RoleSpectator || who.Local {
		t.Fatalf("identity = %+v, want remote spectator", who)
	}

	// 5. The spectator reads the state and the tunnel but not the settings.
	response = do(t, client, http.MethodGet, testServer.URL+"/api/panel/state", "", true)
	var state stateResponse
	decodeBody(t, response, &state)
	if state.Role != auth.RoleSpectator || len(state.Snapshot.Days) != 1 {
		t.Fatalf("state = %+v", state)
	}
	response = do(t, client, http.MethodGet, testServer.URL+"/api/panel/tunnel", "", true)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("spectator tunnel = %d", response.StatusCode)
	}
	response.Body.Close()
	response = do(t, client, http.MethodGet, testServer.URL+"/api/panel/settings", "", true)
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("spectator settings = %d, want 403", response.StatusCode)
	}
	if strings.Contains(readBody(t, response), "super-secret") {
		t.Fatal("settings leaked a secret")
	}

	// 6. The invite is one-time.
	response = do(t, client, http.MethodPost, testServer.URL+"/api/panel/session", `{"token":"`+invite.Token+`"}`, true)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("invite reuse = %d, want 401", response.StatusCode)
	}
	response.Body.Close()
}

func do(t *testing.T, client *http.Client, method, url, body string, remote bool) *http.Response {
	t.Helper()
	var reader *strings.Reader
	if body == "" {
		reader = strings.NewReader("")
	} else {
		reader = strings.NewReader(body)
	}
	request, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	if body != "" {
		request.Header.Set("content-type", "application/json")
	}
	if remote {
		// cloudflared connects from loopback; the forwarding header marks it as
		// a tunneled request.
		request.Header.Set("Cf-Connecting-Ip", "203.0.113.7")
	}
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func decodeBody(t *testing.T, response *http.Response, target any) {
	t.Helper()
	defer response.Body.Close()
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}

func readBody(t *testing.T, response *http.Response) string {
	t.Helper()
	defer response.Body.Close()
	var builder bytes.Buffer
	if _, err := builder.ReadFrom(response.Body); err != nil {
		t.Fatal(err)
	}
	return builder.String()
}
