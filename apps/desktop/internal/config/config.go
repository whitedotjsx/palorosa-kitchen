// Package config resolves the desktop app runtime configuration. It reads the
// repository .env (the same file the Node apps use) and then the persisted
// settings.json, which wins; environment variables fill whatever settings left
// empty (D28).
package config

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/settings"
)

// defaultDomain is the store domain when neither settings nor the environment
// set one (D28).
const defaultDomain = "palorosabreakfast.com"

// Config is the desktop app runtime configuration.
type Config struct {
	DataDir          string
	SettingsPath     string
	Domain           string
	CatalogPath      string
	Allowlist        []string
	PairingPhone     string
	HookPort         int
	PanelPort        int
	HookToken        string
	WebhookSecret    string
	WPAdminURL       string
	WPAdminUser      string
	WPAdminPassword  string
	WPConsumerKey    string
	WPConsumerSecret string
	WPExportID       string
	WPExportCronKey  string
	TunnelName       string
	TunnelHostname   string
	TunnelToken      string
	TunnelService    string
	CloudflaredPath  string
	AccessEnabled    bool
	WhatsAppEnabled  bool
	ListHTML         string
	PanelHTML        string
	Debug            bool
}

// Load walks up from the working directory and the executable looking for a
// .env file, then reads the host settings. Precedence is settings.json over
// .env over the built-in default (D28); on the first run the .env values are
// migrated into settings.json.
func Load() Config {
	loadDotEnv()

	configDir, err := os.UserConfigDir()
	if err != nil || configDir == "" {
		configDir = "."
	}
	dataDir := filepath.Join(configDir, "palorosa-kitchen")

	store, storeErr := settings.Open(filepath.Join(dataDir, "settings.json"))
	if storeErr != nil {
		fmt.Fprintln(os.Stderr, "settings:", storeErr)
	}
	if !store.Exists() {
		if migrated := settings.FromEnv(os.Getenv); !migrated.Empty() {
			if err := store.Update(migrated); err != nil {
				fmt.Fprintln(os.Stderr, "settings:", err)
			} else {
				fmt.Fprintln(os.Stderr, "settings: imported .env into", store.Path())
			}
		}
	}
	values := store.Values()

	hookPort := port(env("BOT_PORT", "5210"), 5210)
	panelPort := port(env("PANEL_PORT", "5211"), 5211)
	if values.PanelPort > 0 {
		panelPort = values.PanelPort
	}
	domain := pick(values.Domain, env("PALOROSA_DOMAIN", defaultDomain))

	return Config{
		DataDir:          dataDir,
		SettingsPath:     store.Path(),
		Domain:           domain,
		CatalogPath:      pick(values.CatalogPath, catalogPath()),
		Allowlist:        allowlist(env("WHATSAPP_ALLOWLIST", "")),
		PairingPhone:     digits(env("WHATSAPP_PAIRING_PHONE", "")),
		HookPort:         hookPort,
		PanelPort:        panelPort,
		HookToken:        env("BOT_HOOK_TOKEN", ""),
		WebhookSecret:    pick(values.WebhookSecret, env("WC_WEBHOOK_SECRET", "")),
		WPAdminURL:       SiteURL(pick(values.WP.AdminURL, env("WP_ADMIN_URL", "")), domain),
		WPAdminUser:      pick(values.WP.AdminUser, env("WP_ADMIN_USER", "")),
		WPAdminPassword:  pick(values.WP.AdminPassword, env("WP_ADMIN_PASSWORD", "")),
		WPConsumerKey:    pick(values.WP.ConsumerKey, env("WOOCOMERCE_CONSUMER_KEY", "")),
		WPConsumerSecret: pick(values.WP.ConsumerSecret, env("WOOCOMERCE_CONSUMER_SECRET", "")),
		WPExportID:       pick(values.WP.ExportID, env("WP_EXPORT_ID", "")),
		WPExportCronKey:  pick(values.WP.ExportCronKey, env("WP_EXPORT_CRON_KEY", "")),
		TunnelName:       env("TUNNEL_NAME", "palorosa-kitchen"),
		TunnelHostname:   pick(values.TunnelHostname, env("TUNNEL_HOSTNAME", "")),
		TunnelToken:      pick(values.TunnelToken, env("TUNNEL_TOKEN", "")),
		TunnelService:    env("TUNNEL_SERVICE", "http://127.0.0.1:"+strconv.Itoa(panelPort)),
		CloudflaredPath:  env("CLOUDFLARED_PATH", ""),
		AccessEnabled:    values.Access.Enabled,
		WhatsAppEnabled:  !strings.EqualFold(env("KITCHEN_WHATSAPP", ""), "off"),
		ListHTML:         env("KITCHEN_LIST_HTML", ""),
		PanelHTML:        env("KITCHEN_PANEL_HTML", ""),
		Debug:            values.Debug || env("KITCHEN_DESKTOP_DEBUG", "") == "1",
	}
}

// pick returns the settings value when set, otherwise the environment (or
// default) value. It is the precedence rule for every settings backed field.
func pick(fromSettings, fromEnv string) string {
	if fromSettings != "" {
		return fromSettings
	}
	return fromEnv
}

// catalogPath resolves the catalog seed: the override, then data/seed.json
// walking up from the working directory and the executable.
func catalogPath() string {
	if override := env("KITCHEN_CATALOG_PATH", ""); override != "" {
		return override
	}
	for _, dir := range searchDirs() {
		candidate := filepath.Join(dir, "data", "seed.json")
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func port(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 || parsed > 65535 {
		return fallback
	}
	return parsed
}

func env(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok && value != "" {
		return value
	}
	return fallback
}

func digits(value string) string {
	var out strings.Builder
	for _, r := range value {
		if r >= '0' && r <= '9' {
			out.WriteRune(r)
		}
	}
	return out.String()
}

func allowlist(value string) []string {
	parts := strings.Split(value, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if number := digits(part); number != "" {
			out = append(out, number)
		}
	}
	return out
}

// loadDotEnv reads the first .env found walking up from the working directory
// and the executable. Existing environment variables are never overwritten.
func loadDotEnv() {
	for _, dir := range searchDirs() {
		path := filepath.Join(dir, ".env")
		file, err := os.Open(path)
		if err != nil {
			continue
		}
		parseDotEnv(file)
		_ = file.Close()
		return
	}
}

func searchDirs() []string {
	var dirs []string
	if cwd, err := os.Getwd(); err == nil {
		dirs = append(dirs, walkUp(cwd)...)
	}
	if exe, err := os.Executable(); err == nil {
		dirs = append(dirs, walkUp(filepath.Dir(exe))...)
	}
	return dirs
}

func walkUp(dir string) []string {
	var dirs []string
	for {
		dirs = append(dirs, dir)
		parent := filepath.Dir(dir)
		if parent == dir {
			return dirs
		}
		dir = parent
	}
}

func parseDotEnv(file *os.File) {
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.TrimSpace(key)
		value = strings.Trim(strings.TrimSpace(value), `"'`)
		if key == "" {
			continue
		}
		if _, exists := os.LookupEnv(key); !exists {
			_ = os.Setenv(key, value)
		}
	}
}

// WP are the WordPress and WooCommerce credentials the store checks use.
type WP struct {
	AdminURL       string
	AdminUser      string
	AdminPassword  string
	ConsumerKey    string
	ConsumerSecret string
	// ExportID, ExportCronKey and ExportToken select the WP All Export saved
	// export the order sync runs. An empty token is derived from the key.
	ExportID      string
	ExportCronKey string
	ExportToken   string
}

// ResolveWP resolves the store credentials from the given settings values,
// falling back to the environment like Load does. Callers pass the live
// settings so values saved in Ajustes apply without restarting the app.
func ResolveWP(values settings.Values) WP {
	domain := pick(values.Domain, env("PALOROSA_DOMAIN", defaultDomain))
	return WP{
		AdminURL:       SiteURL(pick(values.WP.AdminURL, env("WP_ADMIN_URL", "")), domain),
		AdminUser:      pick(values.WP.AdminUser, env("WP_ADMIN_USER", "")),
		AdminPassword:  pick(values.WP.AdminPassword, env("WP_ADMIN_PASSWORD", "")),
		ConsumerKey:    pick(values.WP.ConsumerKey, env("WOOCOMERCE_CONSUMER_KEY", "")),
		ConsumerSecret: pick(values.WP.ConsumerSecret, env("WOOCOMERCE_CONSUMER_SECRET", "")),
		ExportID:       pick(values.WP.ExportID, env("WP_EXPORT_ID", "1")),
		ExportCronKey:  pick(values.WP.ExportCronKey, env("WP_EXPORT_CRON_KEY", "")),
		ExportToken:    env("WP_EXPORT_TOKEN", ""),
	}
}

// SiteURL normalizes the WordPress site URL. An empty value is inferred from
// the store domain; a missing scheme becomes https; a pasted /wp-admin or
// /wp-login.php path is dropped so the result is always the site root.
func SiteURL(raw, domain string) string {
	value := strings.TrimSpace(raw)
	if value == "" {
		value = strings.TrimSpace(domain)
	}
	if value == "" {
		value = defaultDomain
	}
	if !strings.Contains(value, "://") {
		value = "https://" + value
	}
	value = strings.TrimRight(value, "/")
	for _, suffix := range []string{"/wp-login.php", "/wp-admin"} {
		if strings.HasSuffix(strings.ToLower(value), suffix) {
			value = strings.TrimRight(value[:len(value)-len(suffix)], "/")
		}
	}
	return value
}
