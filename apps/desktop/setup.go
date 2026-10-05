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
}

// applySetup merges the first-run form over the stored values. Skip marks the
// setup as seen without changing anything. Hostnames and identifiers are
// trimmed; secrets keep whatever was typed or pasted.
func applySetup(values settings.Values, payload setupPayload) settings.Values {
	if payload.Skip {
		return values
	}
	set := func(target *string, value string) {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			*target = trimmed
		}
	}
	set(&values.TunnelHostname, payload.TunnelHostname)
	set(&values.Domain, payload.Domain)
	set(&values.WP.AdminURL, payload.AdminURL)
	set(&values.WP.AdminUser, payload.AdminUser)
	set(&values.WP.ConsumerKey, payload.ConsumerKey)
	set(&values.WP.ExportID, payload.ExportID)
	if payload.AdminPassword != "" {
		values.WP.AdminPassword = payload.AdminPassword
	}
	if payload.ConsumerSecret != "" {
		values.WP.ConsumerSecret = payload.ConsumerSecret
	}
	if payload.ExportCronKey != "" {
		values.WP.ExportCronKey = payload.ExportCronKey
	}
	if payload.WebhookSecret != "" {
		values.WebhookSecret = payload.WebhookSecret
	}
	return values
}
