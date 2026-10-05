// Package settings persists the host configuration (store domain, WordPress
// credentials, tunnel hostname) in settings.json, with secrets encrypted at
// rest through the platform codec. It is the source of truth over .env (D28).
package settings

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Values is the persisted host configuration. Secret fields hold plaintext in
// memory; the platform codec encrypts them on disk.
type Values struct {
	Domain         string `json:"domain,omitempty"`
	TunnelHostname string `json:"tunnelHostname,omitempty"`
	// TunnelToken runs the named tunnel without the account certificate, so a
	// new computer only needs the token in the imported configuration.
	TunnelToken string `json:"tunnelToken,omitempty"`
	CatalogPath string `json:"catalogPath,omitempty"`
	PanelPort      int    `json:"panelPort,omitempty"`
	Debug          bool   `json:"debug,omitempty"`
	WP             WP     `json:"wp,omitempty"`
	WebhookSecret  string `json:"webhookSecret,omitempty"`
	Access         Access `json:"access,omitempty"`
	// SyncMinutes is the automatic WooCommerce order sync interval. Nil means
	// the default; 0 turns the automatic sync off.
	SyncMinutes *int `json:"syncMinutes,omitempty"`
}

// WP holds the WordPress and WooCommerce credentials. AdminUser/AdminPassword
// are the wp-login ones (order pull and snapshot); ConsumerKey/ConsumerSecret
// are the WooCommerce REST ones (webhook management). They are separate.
type WP struct {
	AdminURL       string `json:"adminUrl,omitempty"`
	AdminUser      string `json:"adminUser,omitempty"`
	AdminPassword  string `json:"adminPassword,omitempty"`
	ConsumerKey    string `json:"consumerKey,omitempty"`
	ConsumerSecret string `json:"consumerSecret,omitempty"`
	ExportID       string `json:"exportId,omitempty"`
	ExportCronKey  string `json:"exportCronKey,omitempty"`
}

// Access configures the optional Cloudflare Access layer in front of the panel.
type Access struct {
	Enabled bool `json:"enabled,omitempty"`
}

// Store reads and writes settings.json.
type Store struct {
	path   string
	values Values
	exists bool
}

// Open loads settings.json. A missing file returns an empty store with Exists
// false so the caller can migrate from the environment. A corrupt file returns
// the empty store and an error; the caller should keep the file untouched.
func Open(path string) (*Store, error) {
	store := &Store{path: path}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return store, nil
		}
		return store, err
	}
	store.exists = true
	values, err := decode(raw)
	if err != nil {
		return store, fmt.Errorf("settings: %w", err)
	}
	store.values = values
	return store, nil
}

// Exists reports whether settings.json was present at Open.
func (s *Store) Exists() bool { return s.exists }

// Path is the settings.json location.
func (s *Store) Path() string { return s.path }

// Values returns the loaded configuration (secrets in clear, in memory only).
func (s *Store) Values() Values { return s.values }

// Update replaces the configuration and writes it atomically. Secrets are
// encrypted by the platform codec before they touch the disk.
func (s *Store) Update(values Values) error {
	raw, err := encode(values)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return err
	}
	temporary := s.path + ".tmp"
	if err := os.WriteFile(temporary, raw, 0o600); err != nil {
		return err
	}
	if err := os.Rename(temporary, s.path); err != nil {
		return err
	}
	s.values = values
	s.exists = true
	return nil
}

// Empty reports whether no value is set, so the caller can skip migrating an
// empty environment into a new file.
func (v Values) Empty() bool {
	return v.Domain == "" &&
		v.TunnelHostname == "" &&
		v.TunnelToken == "" &&
		v.WebhookSecret == "" &&
		v.WP == WP{}
}

// Redacted returns a copy with every secret masked, for API responses.
func (v Values) Redacted() Values {
	out := v
	out.WP.AdminPassword = Mask(out.WP.AdminPassword)
	out.WP.ConsumerSecret = Mask(out.WP.ConsumerSecret)
	out.WP.ExportCronKey = Mask(out.WP.ExportCronKey)
	out.WebhookSecret = Mask(out.WebhookSecret)
	out.TunnelToken = Mask(out.TunnelToken)
	return out
}

// Mask hides a secret, keeping only its last four characters.
func Mask(secret string) string {
	if secret == "" {
		return ""
	}
	if len(secret) <= 4 {
		return strings.Repeat("\u2022", len(secret))
	}
	return "\u2022\u2022\u2022\u2022" + secret[len(secret)-4:]
}

func encode(values Values) ([]byte, error) {
	sealed, err := sealValues(values)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(sealed, "", "\t")
}

func decode(raw []byte) (Values, error) {
	var sealed Values
	if err := json.Unmarshal(raw, &sealed); err != nil {
		return Values{}, err
	}
	return openValues(sealed)
}

func sealValues(v Values) (Values, error) {
	var err error
	if v.WP.AdminPassword, err = seal(v.WP.AdminPassword); err != nil {
		return Values{}, err
	}
	if v.WP.ConsumerSecret, err = seal(v.WP.ConsumerSecret); err != nil {
		return Values{}, err
	}
	if v.WP.ExportCronKey, err = seal(v.WP.ExportCronKey); err != nil {
		return Values{}, err
	}
	if v.WebhookSecret, err = seal(v.WebhookSecret); err != nil {
		return Values{}, err
	}
	if v.TunnelToken, err = seal(v.TunnelToken); err != nil {
		return Values{}, err
	}
	return v, nil
}

func openValues(v Values) (Values, error) {
	var err error
	if v.WP.AdminPassword, err = open(v.WP.AdminPassword); err != nil {
		return Values{}, err
	}
	if v.WP.ConsumerSecret, err = open(v.WP.ConsumerSecret); err != nil {
		return Values{}, err
	}
	if v.WP.ExportCronKey, err = open(v.WP.ExportCronKey); err != nil {
		return Values{}, err
	}
	if v.WebhookSecret, err = open(v.WebhookSecret); err != nil {
		return Values{}, err
	}
	if v.TunnelToken, err = open(v.TunnelToken); err != nil {
		return Values{}, err
	}
	return v, nil
}
