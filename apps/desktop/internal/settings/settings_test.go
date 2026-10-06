package settings

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if store.Exists() {
		t.Fatal("expected no settings file yet")
	}

	want := Values{
		Domain:         "palorosabreakfast.com",
		TunnelHostname: "cocina.palorosabreakfast.com",
		TunnelToken:    "token-123",
		WP: WP{
			AdminURL:       "https://palorosabreakfast.com",
			AdminUser:      "operador",
			AdminPassword:  "s3cret-pass",
			ConsumerKey:    "ck_public",
			ConsumerSecret: "cs_secret",
			ExportID:       "1",
			ExportCronKey:  "cron-1",
		},
		WebhookSecret: "wh-1",
		Access:        Access{Enabled: true},
	}
	if err := store.Update(want); err != nil {
		t.Fatal(err)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if got := reopened.Values(); got != want {
		t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", got, want)
	}
}

func TestSecretsAreNotStoredInClear(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	values := Values{
		WP: WP{
			AdminPassword:  "admin-password",
			ConsumerSecret: "consumer-secret",
			ExportCronKey:  "cron-secret",
		},
		WebhookSecret: "webhook-secret",
		TunnelToken:   "tunnel-secret",
	}
	if err := store.Update(values); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{"admin-password", "consumer-secret", "cron-secret", "webhook-secret", "tunnel-secret"} {
		if bytes.Contains(raw, []byte(secret)) {
			t.Fatalf("secret %q stored in clear:\n%s", secret, raw)
		}
	}
	if !bytes.Contains(raw, []byte(secretPrefix)) {
		t.Fatalf("expected the %q marker in the file:\n%s", secretPrefix, raw)
	}
}

func TestFromEnv(t *testing.T) {
	env := map[string]string{
		"WP_ADMIN_URL":               "https://palorosabreakfast.com",
		"WP_ADMIN_USER":              "operador",
		"WP_ADMIN_PASSWORD":          "pw",
		"WOOCOMERCE_CONSUMER_KEY":    "ck",
		"WOOCOMERCE_CONSUMER_SECRET": "cs",
		"WP_EXPORT_ID":               "1",
		"WC_WEBHOOK_SECRET":          "wh",
		"TUNNEL_HOSTNAME":            "cocina.palorosabreakfast.com",
		"TUNNEL_TOKEN":               "tok",
		"KITCHEN_AUTO_UPDATE":        "off",
	}
	getenv := func(key string) string { return env[key] }

	got := FromEnv(getenv)
	if got.WP.ConsumerKey != "ck" || got.WP.ConsumerSecret != "cs" {
		t.Fatalf("consumer credentials not migrated: %+v", got.WP)
	}
	if got.WebhookSecret != "wh" || got.TunnelHostname != "cocina.palorosabreakfast.com" || got.TunnelToken != "tok" {
		t.Fatalf("webhook or tunnel not migrated: %+v", got)
	}
	if got.AutoUpdate == nil || *got.AutoUpdate {
		t.Fatalf("KITCHEN_AUTO_UPDATE=off not migrated: %+v", got.AutoUpdate)
	}
	if FromEnv(func(string) string { return "" }).Empty() != true {
		t.Fatal("an empty environment should produce empty values")
	}
}

// TestNewSettingsFieldsRoundTrip covers the fields added after v1: they must
// survive the encrypted round trip like the rest of the store.
func TestNewSettingsFieldsRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	autoUpdate, autostart := false, true
	want := Values{
		AutoUpdate:    &autoUpdate,
		Autostart:     &autostart,
		StationSecret: "station-1",
	}
	if err := store.Update(want); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	got := reopened.Values()
	if got.StationSecret != "station-1" {
		t.Fatalf("station secret lost: %q", got.StationSecret)
	}
	if got.AutoUpdate == nil || *got.AutoUpdate || got.Autostart == nil || !*got.Autostart {
		t.Fatalf("switches lost: %+v %+v", got.AutoUpdate, got.Autostart)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte("station-1")) {
		t.Fatalf("station secret stored in clear:\n%s", raw)
	}
}

func TestMask(t *testing.T) {
	if got := Mask(""); got != "" {
		t.Fatalf("Mask(\"\") = %q, want empty", got)
	}
	if got := Mask("abcd"); got == "abcd" || !strings.Contains(got, "\u2022") {
		t.Fatalf("short secret not masked: %q", got)
	}
	got := Mask("abcdefgh")
	if !strings.HasSuffix(got, "efgh") || strings.Contains(got, "abcd") {
		t.Fatalf("Mask kept the wrong part: %q", got)
	}
}

func TestRedactedHidesSecrets(t *testing.T) {
	values := Values{
		WP:            WP{AdminPassword: "pw", ConsumerSecret: "cs", ExportCronKey: "ck"},
		WebhookSecret: "wh",
		TunnelToken:   "tok",
	}
	redacted := values.Redacted()
	if redacted.WP.AdminPassword == "pw" || redacted.WP.ConsumerSecret == "cs" ||
		redacted.WP.ExportCronKey == "ck" || redacted.WebhookSecret == "wh" || redacted.TunnelToken == "tok" {
		t.Fatalf("Redacted leaked a secret: %+v", redacted)
	}
}
