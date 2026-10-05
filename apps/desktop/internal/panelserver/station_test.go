package panelserver

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/settings"
)

func TestSettingsExportImportRoundTrip(t *testing.T) {
	server, store, _ := newTestServer(t)
	original := settings.Values{
		Domain:         "palorosabreakfast.com",
		TunnelHostname: "cocina.palorosabreakfast.com",
		TunnelToken:    "token-123",
		WP:             settings.WP{AdminUser: "op", AdminPassword: "pw", ExportCronKey: "ck"},
		WebhookSecret:  "wh",
	}
	if err := store.Update(original); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodGet, "/api/panel/settings/export", nil)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("export status = %d, body %s", recorder.Code, recorder.Body)
	}
	exported := recorder.Body.Bytes()
	if !bytes.Contains(exported, []byte(`"pw"`)) || !bytes.Contains(exported, []byte(`"token-123"`)) || !strings.Contains(recorder.Header().Get("Content-Disposition"), "attachment") {
		t.Fatalf("export should carry secrets as a download: %s", exported)
	}

	other, otherStore, _ := newTestServer(t)
	recorder = httptest.NewRecorder()
	other.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodPost, "/api/panel/settings/import", bytes.NewReader(exported))))
	if recorder.Code != http.StatusOK {
		t.Fatalf("import status = %d, body %s", recorder.Code, recorder.Body)
	}
	got := otherStore.Values()
	if got.WP.AdminPassword != "pw" || got.WebhookSecret != "wh" || got.TunnelHostname != original.TunnelHostname || got.TunnelToken != "token-123" {
		t.Fatalf("import lost values: %+v", got)
	}
}

func TestSettingsPatchMasksTunnelToken(t *testing.T) {
	server, store, _ := newTestServer(t)
	recorder := httptest.NewRecorder()
	body := strings.NewReader(`{"tunnelToken":"secret-token-1234"}`)
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodPatch, "/api/panel/settings", body)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("patch status = %d, body %s", recorder.Code, recorder.Body)
	}
	if store.Values().TunnelToken != "secret-token-1234" {
		t.Fatalf("token not stored: %+v", store.Values())
	}
	if strings.Contains(recorder.Body.String(), "secret-token-1234") || !strings.Contains(recorder.Body.String(), "1234") {
		t.Fatalf("response should mask the token: %s", recorder.Body)
	}
}

func TestSettingsImportRejectsForeignFile(t *testing.T) {
	server, _, _ := newTestServer(t)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodPost, "/api/panel/settings/import", bytes.NewBufferString(`{"domain":"x"}`))))
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestSettingsExportIsHostOnly(t *testing.T) {
	server, _, _ := newTestServer(t)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/panel/settings/export", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
}

func TestStationSignsInWithKey(t *testing.T) {
	server, store, _ := newTestServer(t)
	if err := store.Update(settings.Values{WebhookSecret: "wh"}); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/panel/station?key=bad", nil))
	if recorder.Code != http.StatusFound || len(recorder.Result().Cookies()) != 0 {
		t.Fatalf("bad key: status %d cookies %v", recorder.Code, recorder.Result().Cookies())
	}

	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/panel/station?key="+StationKey("wh")+"&label=PC2", nil))
	cookies := recorder.Result().Cookies()
	if recorder.Code != http.StatusFound || len(cookies) == 0 {
		t.Fatalf("good key: status %d cookies %v", recorder.Code, cookies)
	}

	request := httptest.NewRequest(http.MethodGet, "/api/panel/session", nil)
	request.AddCookie(cookies[0])
	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if !strings.Contains(recorder.Body.String(), `"role":"spectator"`) {
		t.Fatalf("station session should be a spectator: %s", recorder.Body)
	}
}
