//go:build windows

package main

import (
	_ "embed"
	"strings"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/settings"
)

//go:embed assets/setup.html
var setupHTML []byte

// setupPayload is what the first-run page sends. Empty values leave the stored
// configuration untouched, so importing a partial file never wipes anything.
type setupPayload struct {
	Skip           bool   `json:"skip"`
	TunnelHostname string `json:"tunnelHostname"`
	Domain         string `json:"domain"`
	AdminURL       string `json:"adminUrl"`
	AdminUser      string `json:"adminUser"`
	AdminPassword  string `json:"adminPassword"`
	ConsumerKey    string `json:"consumerKey"`
	ConsumerSecret string `json:"consumerSecret"`
	ExportID       string `json:"exportId"`
	ExportCronKey  string `json:"exportCronKey"`
	WebhookSecret  string `json:"webhookSecret"`
	// Imported is the settings object of an exported configuration file. The
	// first-run form only shows some fields, so the hidden ones (tunnel token,
	// catalog path, sync interval) travel here and reach the new machine too.
	Imported *settings.Values `json:"imported"`
}

// applySetup merges the first-run form over the stored values. Skip marks the
// setup as seen without changing anything. An imported file fills every field
// it carries; the visible form then wins over it. Hostnames and identifiers
// are trimmed; secrets keep whatever was typed or pasted.
func applySetup(values settings.Values, payload setupPayload) settings.Values {
	if payload.Skip {
		return values
	}
	out := values
	if payload.Imported != nil {
		out = mergeImported(out, *payload.Imported)
	}
	set := func(target *string, value string) {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			*target = trimmed
		}
	}
	set(&out.TunnelHostname, payload.TunnelHostname)
	set(&out.Domain, payload.Domain)
	set(&out.WP.AdminURL, payload.AdminURL)
	set(&out.WP.AdminUser, payload.AdminUser)
	set(&out.WP.ConsumerKey, payload.ConsumerKey)
	set(&out.WP.ExportID, payload.ExportID)
	if payload.AdminPassword != "" {
		out.WP.AdminPassword = payload.AdminPassword
	}
	if payload.ConsumerSecret != "" {
		out.WP.ConsumerSecret = payload.ConsumerSecret
	}
	if payload.ExportCronKey != "" {
		out.WP.ExportCronKey = payload.ExportCronKey
	}
	if payload.WebhookSecret != "" {
		out.WebhookSecret = payload.WebhookSecret
	}
	return out
}

// mergeImported copies the non-empty fields of an imported configuration over
// the stored values, so an import never erases what the file does not carry.
func mergeImported(base, imported settings.Values) settings.Values {
	out := base
	set := func(target *string, value string) {
		if value != "" {
			*target = value
		}
	}
	set(&out.Domain, imported.Domain)
	set(&out.TunnelHostname, imported.TunnelHostname)
	set(&out.TunnelToken, imported.TunnelToken)
	set(&out.CatalogPath, imported.CatalogPath)
	set(&out.WebhookSecret, imported.WebhookSecret)
	set(&out.WP.AdminURL, imported.WP.AdminURL)
	set(&out.WP.AdminUser, imported.WP.AdminUser)
	set(&out.WP.AdminPassword, imported.WP.AdminPassword)
	set(&out.WP.ConsumerKey, imported.WP.ConsumerKey)
	set(&out.WP.ConsumerSecret, imported.WP.ConsumerSecret)
	set(&out.WP.ExportID, imported.WP.ExportID)
	set(&out.WP.ExportCronKey, imported.WP.ExportCronKey)
	if imported.PanelPort > 0 {
		out.PanelPort = imported.PanelPort
	}
	if imported.Debug {
		out.Debug = true
	}
	if imported.SyncMinutes != nil {
		out.SyncMinutes = imported.SyncMinutes
	}
	if imported.Access.Enabled {
		out.Access.Enabled = true
	}
	return out
}
