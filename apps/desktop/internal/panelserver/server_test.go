package panelserver

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/auth"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/bots"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/notify"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/panelmodel"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/push"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/settings"
)

func newTestServer(t *testing.T) (*Server, *settings.Store, *auth.Manager) {
	t.Helper()
	dir := t.TempDir()
	store, err := settings.Open(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	manager, err := auth.New(filepath.Join(dir, "sessions.json"))
	if err != nil {
		t.Fatal(err)
	}
	server := New(Config{Port: 5211, Settings: store, Auth: manager, PanelBaseURL: "https://cocina.example.com"})
	return server, store, manager
}

func loopback(request *http.Request) *http.Request {
	request.RemoteAddr = "127.0.0.1:49000"
	return request
}

func decode(t *testing.T, recorder *httptest.ResponseRecorder, target any) {
	t.Helper()
	if err := json.Unmarshal(recorder.Body.Bytes(), target); err != nil {
		t.Fatalf("decode: %v (body %s)", err, recorder.Body)
	}
}

func TestSettingsGetIsRedacted(t *testing.T) {
	server, store, _ := newTestServer(t)
	if err := store.Update(settings.Values{
		Domain:        "palorosabreakfast.com",
		WP:            settings.WP{AdminPassword: "super-secret"},
		WebhookSecret: "wh-secret",
	}); err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodGet, "/api/panel/settings", nil)))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", recorder.Code, recorder.Body)
	}
	body := recorder.Body.String()
	if bytes.Contains([]byte(body), []byte("super-secret")) || bytes.Contains([]byte(body), []byte("wh-secret")) {
		t.Fatalf("GET leaked a secret: %s", body)
	}
	if !bytes.Contains([]byte(body), []byte("palorosabreakfast.com")) {
		t.Fatalf("GET lost the domain: %s", body)
	}
}

func TestSettingsPatchPersists(t *testing.T) {
	server, store, _ := newTestServer(t)

	patch := `{"domain":"cocina.example.com","wp":{"adminUser":"operador","adminPassword":"pw-1","consumerKey":"ck_1","consumerSecret":"cs_1"},"access":{"enabled":true}}`
	recorder := httptest.NewRecorder()
	request := loopback(httptest.NewRequest(http.MethodPatch, "/api/panel/settings", bytes.NewBufferString(patch)))
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", recorder.Code, recorder.Body)
	}

	values := store.Values()
	if values.Domain != "cocina.example.com" || values.WP.AdminUser != "operador" ||
		values.WP.AdminPassword != "pw-1" || !values.Access.Enabled {
		t.Fatalf("patch not applied: %+v", values)
	}

	recorder = httptest.NewRecorder()
	request = loopback(httptest.NewRequest(http.MethodPatch, "/api/panel/settings", bytes.NewBufferString(`{"wp":{"adminUser":"nuevo"}}`)))
	server.Handler().ServeHTTP(recorder, request)
	values = store.Values()
	if values.WP.AdminUser != "nuevo" || values.Domain != "cocina.example.com" || values.WP.AdminPassword != "pw-1" {
		t.Fatalf("omitted fields should be kept: %+v", values)
	}
}

func TestSettingsRejectsRemoteWithoutSession(t *testing.T) {
	server, _, _ := newTestServer(t)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/panel/settings", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
}

func TestSettingsRejectsBadJSON(t *testing.T) {
	server, _, _ := newTestServer(t)
	recorder := httptest.NewRecorder()
	request := loopback(httptest.NewRequest(http.MethodPatch, "/api/panel/settings", bytes.NewBufferString("{")))
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", recorder.Code)
	}
}

func TestForwardedRequestIsRemote(t *testing.T) {
	server, _, _ := newTestServer(t)
	request := loopback(httptest.NewRequest(http.MethodGet, "/api/panel/invites", nil))
	request.Header.Set("Cf-Connecting-Ip", "203.0.113.7")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("forwarded request should be remote; status = %d, want 401", recorder.Code)
	}
}

func TestHostCreatesInviteAndSpectatorReadsTunnel(t *testing.T) {
	server, _, _ := newTestServer(t)

	recorder := httptest.NewRecorder()
	request := loopback(httptest.NewRequest(http.MethodPost, "/api/panel/invites", bytes.NewBufferString(`{"label":"Doña Marta"}`)))
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("invite status = %d, body %s", recorder.Code, recorder.Body)
	}
	var created struct {
		Token string `json:"token"`
		URL   string `json:"url"`
	}
	decode(t, recorder, &created)
	if created.Token == "" || created.URL == "" {
		t.Fatalf("invite missing token or url: %+v", created)
	}

	// A remote spectator redeems the invite for a session cookie.
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodPost, "/api/panel/session", bytes.NewBufferString(`{"token":"`+created.Token+`"}`))
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("redeem status = %d, body %s", recorder.Code, recorder.Body)
	}
	cookies := recorder.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatal("no session cookie set")
	}
	session := cookies[0]

	// The spectator may read the tunnel status.
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/panel/tunnel", nil)
	request.AddCookie(session)
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("spectator tunnel status = %d, want 200", recorder.Code)
	}

	// But cannot read or write the settings, nor manage invites.
	for _, test := range []struct {
		method string
		path   string
	}{{"GET", "/api/panel/settings"}, {"PATCH", "/api/panel/settings"}, {"POST", "/api/panel/invites"}, {"GET", "/api/panel/sessions"}} {
		recorder = httptest.NewRecorder()
		request = httptest.NewRequest(test.method, test.path, bytes.NewBufferString(`{}`))
		request.AddCookie(session)
		server.Handler().ServeHTTP(recorder, request)
		if recorder.Code != http.StatusForbidden {
			t.Fatalf("%s %s as spectator = %d, want 403", test.method, test.path, recorder.Code)
		}
	}

	// The session is identified in the session endpoint.
	recorder = httptest.NewRecorder()
	request = httptest.NewRequest(http.MethodGet, "/api/panel/session", nil)
	request.AddCookie(session)
	server.Handler().ServeHTTP(recorder, request)
	var who struct {
		Role  string `json:"role"`
		Local bool   `json:"local"`
	}
	decode(t, recorder, &who)
	if who.Role != auth.RoleSpectator || who.Local {
		t.Fatalf("session identity = %+v, want spectator remote", who)
	}
}

func TestLocalSessionIsHost(t *testing.T) {
	server, _, _ := newTestServer(t)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodGet, "/api/panel/session", nil)))
	var who struct {
		Role  string `json:"role"`
		Local bool   `json:"local"`
	}
	decode(t, recorder, &who)
	if who.Role != auth.RoleHost || !who.Local {
		t.Fatalf("local identity = %+v, want host local", who)
	}
}

func TestInviteAndSessionRevoke(t *testing.T) {
	server, _, manager := newTestServer(t)

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodPost, "/api/panel/invites", bytes.NewBufferString(`{}`))))
	var created struct {
		Token  string      `json:"token"`
		Invite auth.Invite `json:"invite"`
	}
	decode(t, recorder, &created)

	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodGet, "/api/panel/invites", nil)))
	var list struct {
		Invites []auth.Invite `json:"invites"`
	}
	decode(t, recorder, &list)
	if len(list.Invites) != 1 {
		t.Fatalf("invites = %d, want 1", len(list.Invites))
	}

	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodDelete, "/api/panel/invites/"+created.Invite.ID, nil)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("revoke invite status = %d", recorder.Code)
	}
	if _, _, err := manager.Redeem(created.Token, ""); err == nil {
		t.Fatal("revoked invite still redeemable")
	}
}

func TestHooksAreMounted(t *testing.T) {
	called := false
	server := New(Config{
		Port: 5211,
		Hooks: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			called = true
			w.WriteHeader(http.StatusOK)
		}),
	})
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/hook/woocommerce", nil))
	if !called || recorder.Code != http.StatusOK {
		t.Fatalf("hook not mounted: called=%v status=%d", called, recorder.Code)
	}
}

func TestApplyPatchKeepsUntouchedFields(t *testing.T) {
	current := settings.Values{
		Domain: "a.com",
		WP:     settings.WP{AdminUser: "u", AdminPassword: "p"},
	}
	user := "v"
	updated := applyPatch(current, settingsPatch{WP: &wpPatch{AdminUser: &user}})
	if updated.WP.AdminUser != "v" || updated.WP.AdminPassword != "p" || updated.Domain != "a.com" {
		t.Fatalf("applyPatch mismatch: %+v", updated)
	}
}

func TestHealthReportsPanel(t *testing.T) {
	server, _, _ := newTestServer(t)
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/health", nil))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	var payload map[string]any
	decode(t, recorder, &payload)
	if payload["panel"] != true || payload["auth"] != true {
		t.Fatalf("health payload = %+v", payload)
	}
}

func TestTunnelEndpoint(t *testing.T) {
	server := New(Config{Port: 5211, Tunnel: func() TunnelInfo {
		return TunnelInfo{Hostname: "cocina.example.com", URL: "https://cocina.example.com", Status: "running", CertPresent: true}
	}})
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodGet, "/api/panel/tunnel", nil)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	var info TunnelInfo
	decode(t, recorder, &info)
	if info.Status != "running" || !info.CertPresent {
		t.Fatalf("unexpected tunnel info: %+v", info)
	}
}

func TestTunnelDisabled(t *testing.T) {
	server := New(Config{Port: 5211})
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodGet, "/api/panel/tunnel", nil)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	var info TunnelInfo
	decode(t, recorder, &info)
	if info.Status != "disabled" {
		t.Fatalf("status = %q, want disabled", info.Status)
	}
}

func TestPanelIsServed(t *testing.T) {
	server := New(Config{Port: 5211, Panel: []byte("<!doctype html><title>panel</title>")})
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusOK || !bytes.Contains(recorder.Body.Bytes(), []byte("panel")) {
		t.Fatalf("panel not served: %d %s", recorder.Code, recorder.Body)
	}
}

func TestTunnelLoginEndpoint(t *testing.T) {
	called := false
	server := New(Config{Port: 5211, TunnelLogin: func() error { called = true; return nil }})
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodPost, "/api/panel/tunnel/login", nil)))
	if recorder.Code != http.StatusOK || !called {
		t.Fatalf("login status = %d called=%v", recorder.Code, called)
	}
}

func TestTunnelLoginUnavailable(t *testing.T) {
	server := New(Config{Port: 5211})
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodPost, "/api/panel/tunnel/login", nil)))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}

func TestTunnelLoginIsHostOnly(t *testing.T) {
	server := New(Config{Port: 5211, TunnelLogin: func() error { return nil }})
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/panel/tunnel/login", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", recorder.Code)
	}
}

func sampleSnapshot() panelmodel.Snapshot {
	return panelmodel.Snapshot{
		WhatsApp:      panelmodel.WhatsApp{Status: "connected", Label: "conectado", Linked: true, Allowlist: []string{"+57..."}},
		Notifications: panelmodel.Notifications{Enabled: true, New: true, Updated: true, Today: true, Tomorrow: true},
		Days: []panelmodel.Day{{
			Date:   "2026-10-02",
			Orders: []panelmodel.Order{{Number: "6476", Text: "x1 Gold Basic", Units: 2}},
			List:   panelmodel.List{Total: 3, Entries: []panelmodel.Entry{{Name: "Jugo", Quantity: 3, Category: "drink"}}},
		}},
	}
}

func TestStateIncludesSnapshot(t *testing.T) {
	server := New(Config{Port: 5211, Snapshot: sampleSnapshot})
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodGet, "/api/panel/state", nil)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	var state struct {
		Role     string              `json:"role"`
		Snapshot panelmodel.Snapshot `json:"snapshot"`
	}
	decode(t, recorder, &state)
	if state.Role != auth.RoleHost || len(state.Snapshot.Days) != 1 || state.Snapshot.Days[0].List.Total != 3 {
		t.Fatalf("unexpected state: %+v", state)
	}
}

func TestListAndOrders(t *testing.T) {
	server := New(Config{Port: 5211, Snapshot: sampleSnapshot})

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodGet, "/api/panel/list?date=2026-10-02", nil)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("list status = %d", recorder.Code)
	}
	var list struct {
		List panelmodel.List `json:"list"`
	}
	decode(t, recorder, &list)
	if list.List.Total != 3 {
		t.Fatalf("list total = %d, want 3", list.List.Total)
	}

	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodGet, "/api/panel/orders?date=2026-10-02", nil)))
	var orders struct {
		Orders []panelmodel.Order `json:"orders"`
	}
	decode(t, recorder, &orders)
	if len(orders.Orders) != 1 || orders.Orders[0].Number != "6476" {
		t.Fatalf("orders = %+v", orders.Orders)
	}

	// A date without a list is an empty 200, not a 404.
	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodGet, "/api/panel/list?date=2000-01-01", nil)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("missing list status = %d, want 200", recorder.Code)
	}
	var empty struct {
		List panelmodel.List `json:"list"`
	}
	decode(t, recorder, &empty)
	if len(empty.List.Entries) != 0 {
		t.Fatalf("missing list should be empty: %+v", empty.List)
	}
}

func TestOrderDetailsRoute(t *testing.T) {
	requested := []string{}
	server := New(Config{
		Port: 5211,
		OrderDetail: func(date, number string) (panelmodel.OrderDetail, error) {
			requested = append(requested, date+"/"+number)
			if number == "0" {
				return panelmodel.OrderDetail{}, errors.New("pedido no encontrado")
			}
			return panelmodel.OrderDetail{
				Number: number,
				Lines:  []panelmodel.OrderLine{{ProductText: "Gold Basic", Quantity: 1, Options: []string{"Rosado"}}},
			}, nil
		},
	})
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodGet, "/api/panel/order-details?date=2026-10-02&numbers=6476,0,6478", nil)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d", recorder.Code)
	}
	var payload struct {
		Orders []panelmodel.OrderDetail `json:"orders"`
	}
	decode(t, recorder, &payload)
	if len(payload.Orders) != 2 || payload.Orders[0].Number != "6476" || payload.Orders[1].Number != "6478" {
		t.Fatalf("orders = %+v", payload.Orders)
	}
	if len(requested) != 3 || requested[0] != "2026-10-02/6476" {
		t.Fatalf("requested = %v", requested)
	}

	// A hostile numbers list is capped instead of resolving without bound.
	requested = requested[:0]
	numbers := make([]string, 0, maxOrderDetails+5)
	for i := 0; i < maxOrderDetails+5; i++ {
		numbers = append(numbers, strconv.Itoa(i+1))
	}
	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodGet, "/api/panel/order-details?date=2026-10-02&numbers="+strings.Join(numbers, ","), nil)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("bulk status = %d", recorder.Code)
	}
	var bulk struct {
		Orders []panelmodel.OrderDetail `json:"orders"`
	}
	decode(t, recorder, &bulk)
	if len(bulk.Orders) != maxOrderDetails {
		t.Fatalf("bulk orders = %d, want %d", len(bulk.Orders), maxOrderDetails)
	}

	// Unavailable when the bot is off.
	off := New(Config{Port: 5211})
	recorder = httptest.NewRecorder()
	off.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodGet, "/api/panel/order-details?date=2026-10-02&numbers=1", nil)))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("details without bot = %d, want 503", recorder.Code)
	}
}

func TestRestartRoute(t *testing.T) {
	restarted := make(chan struct{}, 1)
	server := New(Config{Port: 5211, Restart: func() { restarted <- struct{}{} }})
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodPost, "/api/panel/restart", nil)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("restart status = %d", recorder.Code)
	}
	select {
	case <-restarted:
	case <-time.After(time.Second):
		t.Fatal("restart callback did not run")
	}

	// A remote caller cannot restart the host machine.
	remote := New(Config{Port: 5211, Restart: func() {}})
	recorder = httptest.NewRecorder()
	remote.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/panel/restart", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("remote restart = %d, want 401", recorder.Code)
	}

	// Without the desktop callback the route says so instead of failing.
	off := New(Config{Port: 5211})
	recorder = httptest.NewRecorder()
	off.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodPost, "/api/panel/restart", nil)))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("restart without callback = %d, want 503", recorder.Code)
	}
}

func TestRemoveOrderAndNotifications(t *testing.T) {
	removed := ""
	server := New(Config{
		Port:             5211,
		RemoveOrder:      func(date, number string) error { removed = date + "/" + number; return nil },
		SetNotifications: func(panelmodel.Notifications) error { return nil },
	})

	recorder := httptest.NewRecorder()
	request := loopback(httptest.NewRequest(http.MethodPost, "/api/panel/orders/2026-10-02/6476/remove", nil))
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || removed != "2026-10-02/6476" {
		t.Fatalf("remove: status=%d removed=%q", recorder.Code, removed)
	}

	recorder = httptest.NewRecorder()
	request = loopback(httptest.NewRequest(http.MethodPatch, "/api/panel/notifications", bytes.NewBufferString(`{"enabled":true,"new":false}`)))
	server.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("notifications status = %d", recorder.Code)
	}

	// Unavailable when the bot is off.
	off := New(Config{Port: 5211})
	recorder2 := httptest.NewRecorder()
	off.Handler().ServeHTTP(recorder2, loopback(httptest.NewRequest(http.MethodPost, "/api/panel/orders/2026-10-02/1/remove", nil)))
	if recorder2.Code != http.StatusServiceUnavailable {
		t.Fatalf("remove without bot = %d, want 503", recorder2.Code)
	}
}

func TestEventsRequiresAuth(t *testing.T) {
	server := New(Config{Port: 5211})
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/panel/events", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("events status = %d, want 401", recorder.Code)
	}
}

func TestTargetsCrud(t *testing.T) {
	manager, err := notify.New(filepath.Join(t.TempDir(), "targets.json"), func(string, string, string) error { return nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	server := New(Config{Port: 5211, Targets: manager})

	recorder := httptest.NewRecorder()
	body := `{"label":"Doña Marta","phone":"+57300","kinds":["new"],"schedule":{"mode":"times","times":["06:30"]},"enabled":true}`
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodPost, "/api/panel/targets", bytes.NewBufferString(body))))
	if recorder.Code != http.StatusOK {
		t.Fatalf("create status = %d, body %s", recorder.Code, recorder.Body)
	}
	var created struct {
		Target notify.Target `json:"target"`
	}
	decode(t, recorder, &created)
	if created.Target.ID == "" {
		t.Fatal("no target id")
	}
	if created.Target.Phone != "57300" {
		t.Fatalf("phone = %q, want 57300 (digits only)", created.Target.Phone)
	}

	recorder = httptest.NewRecorder()
	update := `{"label":"Doña Marta","phone":"+57 300 123 4567","kinds":["new"],"schedule":{"mode":"immediate"},"enabled":true}`
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodPatch, "/api/panel/targets/"+created.Target.ID, bytes.NewBufferString(update))))
	if recorder.Code != http.StatusOK {
		t.Fatalf("update status = %d, body %s", recorder.Code, recorder.Body)
	}
	decode(t, recorder, &created)
	if created.Target.Phone != "573001234567" {
		t.Fatalf("updated phone = %q, want 573001234567", created.Target.Phone)
	}

	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodGet, "/api/panel/targets", nil)))
	var list struct {
		Targets []notify.Target `json:"targets"`
	}
	decode(t, recorder, &list)
	if len(list.Targets) != 1 {
		t.Fatalf("targets = %d, want 1", len(list.Targets))
	}

	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodPost, "/api/panel/targets/"+created.Target.ID+"/test", nil)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("test status = %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodDelete, "/api/panel/targets/"+created.Target.ID, nil)))
	if recorder.Code != http.StatusOK {
		t.Fatalf("delete status = %d", recorder.Code)
	}
	if len(manager.Targets()) != 0 {
		t.Fatal("target not removed")
	}

	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/panel/targets", bytes.NewBufferString("{}")))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("remote create = %d, want 401", recorder.Code)
	}
}

func TestPushEndpoints(t *testing.T) {
	manager, err := push.New(filepath.Join(t.TempDir(), "push.json"), nil)
	if err != nil {
		t.Fatal(err)
	}
	server := New(Config{Port: 5211, Push: manager})

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodGet, "/api/panel/push", nil)))
	var info struct {
		PublicKey string `json:"publicKey"`
	}
	decode(t, recorder, &info)
	if info.PublicKey == "" {
		t.Fatal("no VAPID public key")
	}

	recorder = httptest.NewRecorder()
	body := `{"endpoint":"https://push.example.com/x","keys":{"p256dh":"a","auth":"b"}}`
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodPost, "/api/panel/push", bytes.NewBufferString(body))))
	if recorder.Code != http.StatusOK || manager.Count() != 1 {
		t.Fatalf("subscribe: status=%d count=%d", recorder.Code, manager.Count())
	}

	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodDelete, "/api/panel/push", bytes.NewBufferString(`{"endpoint":"https://push.example.com/x"}`))))
	if recorder.Code != http.StatusOK || manager.Count() != 0 {
		t.Fatalf("unsubscribe: status=%d count=%d", recorder.Code, manager.Count())
	}
}

func TestBotsEndpoints(t *testing.T) {
	dir := t.TempDir()
	raw := `[{"id":"default","name":"Cocina","phone":"+573001112233","enabled":false}]`
	if err := os.WriteFile(filepath.Join(dir, "bots.json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	manager, err := bots.New(bots.Common{BaseDir: dir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	server := New(Config{Port: 5211, Bots: manager})

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodGet, "/api/panel/bots", nil)))
	var list struct {
		Bots []botView `json:"bots"`
	}
	decode(t, recorder, &list)
	if len(list.Bots) != 1 || list.Bots[0].ID != "default" || list.Bots[0].Phone != "+573001112233" {
		t.Fatalf("bots = %+v", list.Bots)
	}

	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodPost, "/api/panel/bots", bytes.NewBufferString(`{"id":"caja","name":"Caja"}`))))
	if recorder.Code != http.StatusOK {
		t.Fatalf("create bot status = %d", recorder.Code)
	}

	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodDelete, "/api/panel/bots/caja", nil)))
	if recorder.Code != http.StatusOK || len(manager.Bots()) != 1 {
		t.Fatalf("delete bot: status=%d bots=%d", recorder.Code, len(manager.Bots()))
	}

	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, "/api/panel/bots", bytes.NewBufferString(`{"name":"X"}`)))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("remote create = %d, want 401", recorder.Code)
	}
}

func TestStateIncludesNextSend(t *testing.T) {
	manager, err := notify.New(filepath.Join(t.TempDir(), "targets.json"), func(string, string, string) error { return nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.AddTarget(notify.Target{Phone: "+1", Kinds: []string{notify.KindNew}, Schedule: notify.Schedule{Mode: "times", Times: []string{"12:00"}, Timezone: "America/Bogota"}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	server := New(Config{Port: 5211, Targets: manager})

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodGet, "/api/panel/state", nil)))
	var state struct {
		NextSend *NextSendInfo `json:"nextSend"`
	}
	decode(t, recorder, &state)
	if state.NextSend == nil || state.NextSend.Time != "12:00" || state.NextSend.Targets != 1 {
		t.Fatalf("nextSend = %+v", state.NextSend)
	}
}

func TestBotUpdateAndPairingRoutes(t *testing.T) {
	manager, err := bots.New(bots.Common{BaseDir: t.TempDir()}, nil)
	if err != nil {
		t.Fatal(err)
	}
	server := New(Config{Port: 5211, Bots: manager})

	// Disabled keeps the account from starting a real client in the test.
	recorder := httptest.NewRecorder()
	body := `{"name":"Caja","enabled":false,"allowlist":["57300"]}`
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodPatch, "/api/panel/bots/default", bytes.NewBufferString(body))))
	if recorder.Code != http.StatusOK {
		t.Fatalf("patch status = %d, body %s", recorder.Code, recorder.Body)
	}
	list := manager.Bots()
	if list[0].Name != "Caja" || len(list[0].Allowlist) != 1 || list[0].Enabled {
		t.Fatalf("bot not updated: %+v", list[0])
	}

	// Retry, reconnect and logout need a running account; none is started in
	// the test.
	for _, action := range []string{"pairing/retry", "reconnect", "logout"} {
		recorder = httptest.NewRecorder()
		server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodPost, "/api/panel/bots/default/"+action, nil)))
		if recorder.Code != http.StatusConflict {
			t.Fatalf("%s status = %d, want 409", action, recorder.Code)
		}
	}
}

func TestAutostartAndDiagnostics(t *testing.T) {
	auto := false
	server := New(Config{
		Port:         5211,
		Autostart:    func() bool { return auto },
		SetAutostart: func(value bool) error { auto = value; return nil },
		Diagnostics:  func() Diagnostics { return Diagnostics{Version: "x", PanelPort: 5211} },
	})

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodGet, "/api/panel/autostart", nil)))
	var got struct {
		Enabled bool `json:"enabled"`
	}
	decode(t, recorder, &got)
	if got.Enabled {
		t.Fatal("autostart should start off")
	}

	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodPost, "/api/panel/autostart", bytes.NewBufferString(`{"enabled":true}`))))
	if recorder.Code != http.StatusOK || !auto {
		t.Fatalf("autostart set: status=%d auto=%v", recorder.Code, auto)
	}

	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodGet, "/api/panel/diagnostics", nil)))
	var diag struct {
		Diagnostics Diagnostics `json:"diagnostics"`
	}
	decode(t, recorder, &diag)
	if diag.Diagnostics.Version != "x" {
		t.Fatalf("diagnostics = %+v", diag.Diagnostics)
	}
}

func TestTargetTestAllRoute(t *testing.T) {
	manager, err := notify.New(filepath.Join(t.TempDir(), "targets.json"), func(string, string, string) error { return nil }, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.AddTarget(notify.Target{Phone: "+1", Kinds: []string{notify.KindNew}, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	server := New(Config{Port: 5211, Targets: manager})

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodPost, "/api/panel/targets/test", nil)))
	var got struct {
		Sent int `json:"sent"`
	}
	decode(t, recorder, &got)
	if recorder.Code != http.StatusOK || got.Sent != 1 {
		t.Fatalf("test all: status=%d %+v", recorder.Code, got)
	}
}

func TestListPublishRequiresBot(t *testing.T) {
	server := New(Config{Port: 5211})
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodPost, "/api/panel/list/publish", nil)))
	if recorder.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", recorder.Code)
	}
}

func TestListPublishUsesCallback(t *testing.T) {
	published := ""
	server := New(Config{Port: 5211, PublishList: func(date string) error {
		published = date
		return nil
	}})
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, loopback(httptest.NewRequest(http.MethodPost, "/api/panel/list/publish?date=2026-10-04", nil)))
	if recorder.Code != http.StatusOK || published != "2026-10-04" {
		t.Fatalf("publish: status=%d date=%q", recorder.Code, published)
	}
}

func TestPwaAssets(t *testing.T) {
	server := New(Config{Port: 5211, PanelIcon: []byte("pngbytes")})

	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/manifest.webmanifest", nil))
	if recorder.Code != http.StatusOK || !bytes.Contains(recorder.Body.Bytes(), []byte("Palorosa")) {
		t.Fatalf("manifest = %d %s", recorder.Code, recorder.Body)
	}

	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/sw.js", nil))
	if recorder.Code != http.StatusOK || recorder.Header().Get("content-type") != "text/javascript" {
		t.Fatalf("sw = %d %q", recorder.Code, recorder.Header().Get("content-type"))
	}

	recorder = httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/icon.png", nil))
	if recorder.Code != http.StatusOK || recorder.Body.String() != "pngbytes" {
		t.Fatalf("icon = %d %q", recorder.Code, recorder.Body)
	}
}
