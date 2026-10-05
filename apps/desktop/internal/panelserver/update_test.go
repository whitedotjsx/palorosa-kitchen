package panelserver

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/update"
)

// newUpdateManager wires an updater against a stub GitHub API.
func newUpdateManager(t *testing.T, tag string) (*update.Manager, *httptest.Server) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/o/r/releases/latest", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"tag_name": tag,
			"assets": []map[string]any{{
				"name":                 "palorosa-kitchen.exe",
				"browser_download_url": "https://example.invalid/palorosa-kitchen.exe",
				"digest":               "",
			}},
		})
	})
	api := httptest.NewServer(mux)
	t.Cleanup(api.Close)
	manager := update.New(update.Config{
		Repo:    "o/r",
		Current: "0.1.0",
		APIBase: api.URL,
		HTTP:    api.Client(),
		Logger:  log.New(io.Discard, "", 0),
	})
	return manager, api
}

func TestUpdateGetReportsState(t *testing.T) {
	manager, _ := newUpdateManager(t, "v0.2.0")
	server := New(Config{Update: manager})
	panel := httptest.NewServer(server.Handler())
	defer panel.Close()

	response, err := http.Get(panel.URL + "/api/panel/update")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	var body struct {
		OK     bool          `json:"ok"`
		Update update.State  `json:"update"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.OK || body.Update.Current != "0.1.0" || body.Update.Status != update.StatusIdle {
		t.Fatalf("body = %+v", body)
	}
}

func TestUpdateCheckFindsNewer(t *testing.T) {
	manager, _ := newUpdateManager(t, "v0.2.0")
	server := New(Config{Update: manager})
	panel := httptest.NewServer(server.Handler())
	defer panel.Close()

	response, err := http.Post(panel.URL+"/api/panel/update/check", "application/json", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", response.StatusCode)
	}
	var body struct {
		OK     bool         `json:"ok"`
		Update update.State `json:"update"`
	}
	if err := json.NewDecoder(response.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	if !body.OK || !body.Update.Available || body.Update.Latest != "0.2.0" {
		t.Fatalf("body = %+v", body)
	}
}

func TestUpdateRoutesDisabled(t *testing.T) {
	server := New(Config{})
	panel := httptest.NewServer(server.Handler())
	defer panel.Close()

	response, err := http.Get(panel.URL + "/api/panel/update")
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", response.StatusCode)
	}
}
