//go:build windows

package main

import (
	"strings"
	"testing"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/settings"
)

func TestParseRestartWait(t *testing.T) {
	cases := map[string]uint32{
		"":                              0,
		"--other":                       0,
		"--wait-pid":                    0,
		"--wait-pid abc":                0,
		"--wait-pid 4321":               4321,
		"--wait-pid 4321 --debug":       4321,
		"--something --wait-pid 99":     99,
		"--wait-pid -1":                 0,
		"--wait-pid 4294967296":         0,
		"--wait-pid 4294967295 --other": 4294967295,
	}
	for input, want := range cases {
		if got := parseRestartWait(strings.Fields(input)); got != want {
			t.Errorf("parseRestartWait(%q) = %d, want %d", input, got, want)
		}
	}
}

func TestApplySetupMergesAndKeepsSecrets(t *testing.T) {
	current := settings.Values{TunnelHostname: "cocina.viejo.dev", WP: settings.WP{AdminUser: "viejo"}}

	// Empty values leave the stored configuration alone.
	kept := applySetup(current, setupPayload{})
	if kept.TunnelHostname != "cocina.viejo.dev" || kept.WP.AdminUser != "viejo" {
		t.Fatalf("empty payload changed values: %+v", kept)
	}

	// Imported values fill in; the skip flag changes nothing.
	skipped := applySetup(current, setupPayload{Skip: true, TunnelHostname: "otro.dev"})
	if skipped.TunnelHostname != "cocina.viejo.dev" {
		t.Fatalf("skip changed values: %+v", skipped)
	}

	merged := applySetup(settings.Values{}, setupPayload{
		TunnelHostname: "  cocina.nuevo.dev  ",
		Domain:         " palorosabreakfast.com ",
		AdminURL:       " https://palorosabreakfast.com/wp-admin ",
		AdminUser:      " cocina ",
		AdminPassword:  "  secreto con espacios  ",
		ConsumerKey:    " ck_1 ",
		ConsumerSecret: " cs_1 ",
		ExportID:       " 12 ",
		ExportCronKey:  " cron ",
		WebhookSecret:  " hook ",
	})
	if merged.TunnelHostname != "cocina.nuevo.dev" || merged.Domain != "palorosabreakfast.com" {
		t.Fatalf("hostnames were not trimmed: %+v", merged)
	}
	if merged.WP.AdminURL != "https://palorosabreakfast.com/wp-admin" || merged.WP.AdminUser != "cocina" || merged.WP.ExportID != "12" {
		t.Fatalf("identifiers were not trimmed: %+v", merged.WP)
	}
	// Passwords and secrets keep exactly what was pasted.
	if merged.WP.AdminPassword != "  secreto con espacios  " || merged.WP.ConsumerSecret != " cs_1 " || merged.WP.ExportCronKey != " cron " || merged.WebhookSecret != " hook " {
		t.Fatalf("secrets were altered: %+v %q", merged.WP, merged.WebhookSecret)
	}
}

func TestApplySetupImportsHiddenFields(t *testing.T) {
	minutes := 15
	imported := settings.Values{
		Domain:         "palorosabreakfast.com",
		TunnelHostname: "cocina.wsuites.dev",
		TunnelToken:    "token-importado",
		CatalogPath:    `C:\otro\equipo\seed.json`,
		SyncMinutes:    &minutes,
		Access:         settings.Access{Enabled: true},
		WebhookSecret:  "hook-importado",
		WP:             settings.WP{AdminPassword: "secreta"},
	}
	merged := applySetup(settings.Values{}, setupPayload{Imported: &imported})
	if merged.TunnelToken != "token-importado" || merged.CatalogPath != `C:\otro\equipo\seed.json` {
		t.Fatalf("hidden fields were dropped: %+v", merged)
	}
	if merged.SyncMinutes == nil || *merged.SyncMinutes != 15 || !merged.Access.Enabled {
		t.Fatalf("sync/access were dropped: %+v", merged)
	}
	if merged.WP.AdminPassword != "secreta" {
		t.Fatalf("imported secret was dropped: %+v", merged.WP)
	}

	// The visible form wins over the imported file; the rest still arrives.
	overridden := applySetup(settings.Values{}, setupPayload{Imported: &imported, TunnelHostname: "cocina.nueva.dev"})
	if overridden.TunnelHostname != "cocina.nueva.dev" || overridden.TunnelToken != "token-importado" {
		t.Fatalf("form or token precedence wrong: %+v", overridden)
	}
}
