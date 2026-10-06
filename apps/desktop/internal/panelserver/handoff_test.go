package panelserver

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/auth"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/settings"
)

// handoffHarness builds a server with a spectator session already signed in.
func handoffHarness(t *testing.T) (*httptest.Server, *http.Client, *settings.Store, chan struct{}) {
	t.Helper()
	dir := t.TempDir()
	store, err := settings.Open(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Update(settings.Values{Domain: "palorosabreakfast.com", WebhookSecret: "wh"}); err != nil {
		t.Fatal(err)
	}
	authManager, err := auth.New(filepath.Join(dir, "sessions.json"))
	if err != nil {
		t.Fatal(err)
	}
	completed := make(chan struct{}, 1)
	server := New(Config{
		Port:       5211,
		Settings:   store,
		Auth:       authManager,
		InstanceID: "instance-1",
		Machine:    "machine-1",
		Version:    "9.9.9",
		HandoffBundle: func() (HandoffBundle, error) {
			return HandoffBundle{
				Settings: store.Values(),
				Catalog:  json.RawMessage(`{"units":[]}`),
			}, nil
		},
		OnHandoff: func() { completed <- struct{}{} },
	})
	testServer := httptest.NewServer(server.Handler())
	t.Cleanup(testServer.Close)

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	response := do(t, client, http.MethodPost, testServer.URL+"/api/panel/invites", `{"label":"Nueva PC"}`, false)
	var invite struct {
		Token string `json:"token"`
	}
	decodeBody(t, response, &invite)
	response = do(t, client, http.MethodPost, testServer.URL+"/api/panel/session", `{"token":"`+invite.Token+`"}`, true)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("redeem = %d", response.StatusCode)
	}
	response.Body.Close()
	return testServer, client, store, completed
}

func TestHandoffFlow(t *testing.T) {
	testServer, client, _, completed := handoffHarness(t)

	// The host mints the one-time code (a direct local request).
	response := do(t, client, http.MethodPost, testServer.URL+"/api/panel/handoff/code", "", false)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("code status = %d", response.StatusCode)
	}
	var code struct {
		Code string `json:"code"`
	}
	decodeBody(t, response, &code)
	if len(code.Code) != 6 {
		t.Fatalf("code = %q, want six digits", code.Code)
	}

	// A wrong code is rejected.
	wrong := "000000"
	if code.Code == wrong {
		wrong = "111111"
	}
	response = do(t, client, http.MethodPost, testServer.URL+"/api/panel/handoff/claim", `{"code":"`+wrong+`"}`, true)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("wrong code = %d, want 401", response.StatusCode)
	}
	response.Body.Close()

	// The spectator claims the code.
	response = do(t, client, http.MethodPost, testServer.URL+"/api/panel/handoff/claim", `{"code":"`+code.Code+`"}`, true)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("claim = %d", response.StatusCode)
	}
	var claim struct {
		Token string          `json:"token"`
		Host  HandoffIdentity `json:"host"`
	}
	decodeBody(t, response, &claim)
	if claim.Token == "" || claim.Host.Version != "9.9.9" || claim.Host.Instance != "instance-1" {
		t.Fatalf("claim = %+v", claim)
	}

	// The code is single use.
	response = do(t, client, http.MethodPost, testServer.URL+"/api/panel/handoff/claim", `{"code":"`+code.Code+`"}`, true)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("code reuse = %d, want 401", response.StatusCode)
	}
	response.Body.Close()

	// The bundle downloads once.
	response = do(t, client, http.MethodGet, testServer.URL+"/api/panel/handoff/bundle?token="+claim.Token, "", true)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("bundle = %d", response.StatusCode)
	}
	var bundle HandoffBundle
	decodeBody(t, response, &bundle)
	if bundle.Format != handoffFormat || bundle.Version != 1 || bundle.Host.Machine != "machine-1" {
		t.Fatalf("bundle = %+v", bundle)
	}
	if len(bundle.Catalog) == 0 || bundle.Settings.Domain != "palorosabreakfast.com" {
		t.Fatalf("bundle missing state: %+v", bundle)
	}
	response = do(t, client, http.MethodGet, testServer.URL+"/api/panel/handoff/bundle?token="+claim.Token, "", true)
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("bundle reuse = %d, want 403", response.StatusCode)
	}
	response.Body.Close()

	// Completing tells the host to step down.
	response = do(t, client, http.MethodPost, testServer.URL+"/api/panel/handoff/complete", `{"token":"`+claim.Token+`"}`, true)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("complete = %d", response.StatusCode)
	}
	response.Body.Close()
	select {
	case <-completed:
	case <-time.After(time.Second):
		t.Fatal("OnHandoff did not run")
	}
}

func TestHandoffCodeIsHostOnly(t *testing.T) {
	testServer, _, _, _ := handoffHarness(t)
	anonymous := &http.Client{}
	response := do(t, anonymous, http.MethodPost, testServer.URL+"/api/panel/handoff/code", "", true)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("anonymous code = %d, want 401", response.StatusCode)
	}
	response.Body.Close()
}

func TestStationRotateRevokesSessions(t *testing.T) {
	testServer, client, store, _ := handoffHarness(t)
	response := do(t, client, http.MethodPost, testServer.URL+"/api/panel/station/rotate", "", false)
	if response.StatusCode != http.StatusOK {
		t.Fatalf("rotate = %d", response.StatusCode)
	}
	var result struct {
		Revoked int `json:"revoked"`
	}
	decodeBody(t, response, &result)
	if result.Revoked != 1 {
		t.Fatalf("revoked = %d, want 1", result.Revoked)
	}
	if store.Values().StationSecret == "" {
		t.Fatal("station secret was not rotated")
	}
	// The old session cookie no longer identifies a spectator.
	response = do(t, client, http.MethodGet, testServer.URL+"/api/panel/state", "", true)
	if response.StatusCode != http.StatusUnauthorized {
		t.Fatalf("revoked session = %d, want 401", response.StatusCode)
	}
	response.Body.Close()
}

func TestTargetsAreHostOnly(t *testing.T) {
	testServer, client, _, _ := handoffHarness(t)
	response := do(t, client, http.MethodGet, testServer.URL+"/api/panel/targets", "", true)
	if response.StatusCode != http.StatusForbidden {
		t.Fatalf("spectator targets = %d, want 403", response.StatusCode)
	}
	response.Body.Close()
}

func TestApplyPatchAutoUpdate(t *testing.T) {
	off := false
	updated := applyPatch(settings.Values{}, settingsPatch{AutoUpdate: &off})
	if updated.AutoUpdate == nil || *updated.AutoUpdate {
		t.Fatalf("autoUpdate not applied: %+v", updated.AutoUpdate)
	}
	on := true
	updated = applyPatch(updated, settingsPatch{AutoUpdate: &on})
	if updated.AutoUpdate == nil || !*updated.AutoUpdate {
		t.Fatalf("autoUpdate not restored: %+v", updated.AutoUpdate)
	}
}
