package settings

import "strings"

// FromEnv builds values from the environment keys the Node tools use, so the
// first run can migrate .env into settings.json (D28).
func FromEnv(getenv func(string) string) Values {
	return Values{
		Domain:         strings.TrimSpace(getenv("PALOROSA_DOMAIN")),
		TunnelHostname: strings.TrimSpace(getenv("TUNNEL_HOSTNAME")),
		WP: WP{
			AdminURL:       strings.TrimSpace(getenv("WP_ADMIN_URL")),
			AdminUser:      strings.TrimSpace(getenv("WP_ADMIN_USER")),
			AdminPassword:  getenv("WP_ADMIN_PASSWORD"),
			ConsumerKey:    strings.TrimSpace(firstEnv(getenv, "WOOCOMERCE_CONSUMER_KEY", "WOOCOMMERCE_CONSUMER_KEY")),
			ConsumerSecret: firstEnv(getenv, "WOOCOMERCE_CONSUMER_SECRET", "WOOCOMMERCE_CONSUMER_SECRET"),
			ExportID:       strings.TrimSpace(getenv("WP_EXPORT_ID")),
			ExportCronKey:  getenv("WP_EXPORT_CRON_KEY"),
		},
		WebhookSecret: getenv("WC_WEBHOOK_SECRET"),
	}
}

// firstEnv returns the first non-empty value among the keys, so the historical
// misspelled keys in .env.example (WOOCOMERCE_) and the corrected spelling both
// work.
func firstEnv(getenv func(string) string, keys ...string) string {
	for _, key := range keys {
		if value := getenv(key); value != "" {
			return value
		}
	}
	return ""
}
