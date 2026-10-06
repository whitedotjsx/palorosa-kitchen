// Package bots runs several WhatsApp accounts over one shared kitchen. The
// registry lives in bots.json, and each account owns its own session under
// bots/<id>/ while the orders, lists and catalog stay shared (D25).
package bots

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/engine"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/panelmodel"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/whatsapp"
)

// Bot is one WhatsApp account.
type Bot struct {
	ID        string   `json:"id"`
	Name      string   `json:"name"`
	Phone     string   `json:"phone,omitempty"`
	Enabled   bool     `json:"enabled"`
	Loopback  bool     `json:"loopback,omitempty"`
	Allowlist []string `json:"allowlist,omitempty"`
}

// Common is the shared configuration for every account.
type Common struct {
	BaseDir       string
	CatalogPath   string
	PairingPhone  string
	HookToken     string
	WebhookSecret string
	Debug         bool
	// Allowlist seeds the first default account when bots.json does not exist
	// yet (the legacy WHATSAPP_ALLOWLIST environment variable).
	Allowlist  []string
	OnOrder    func()
	OnQR       func(id, page string)
	OnLinked   func(id string)
	OnStatus   func(id string, status whatsapp.Status)
	Dispatcher func(text, kind, date string) int
}

// Manager owns the registry and the running accounts.
type Manager struct {
	common  Common
	log     *log.Logger
	kitchen *whatsapp.Kitchen
	// panel serves the panel from the shared kitchen when no account is
	// running, so orders and lists stay visible while WhatsApp is off.
	panel whatsapp.PanelOps

	mu      sync.Mutex
	bots    []Bot
	clients map[string]*whatsapp.Client
}

// New loads bots.json, migrating a single existing session into a default bot
// on first run.
func New(common Common, logger *log.Logger) (*Manager, error) {
	if logger == nil {
		logger = log.Default()
	}
	kitchen := whatsapp.NewKitchen(common.BaseDir, common.CatalogPath, nil)
	manager := &Manager{
		common:  common,
		log:     logger,
		kitchen: kitchen,
		panel:   whatsapp.NewPanelClient(kitchen, common.Dispatcher, common.OnOrder),
		clients: map[string]*whatsapp.Client{},
	}
	path := filepath.Join(common.BaseDir, "bots.json")
	raw, err := os.ReadFile(path)
	if err == nil {
		return manager, json.Unmarshal(raw, &manager.bots)
	}
	if !os.IsNotExist(err) {
		return nil, err
	}
	migrateLegacy(common.BaseDir, logger)
	manager.bots = []Bot{{ID: "default", Name: "Cocina", Enabled: true, Allowlist: append([]string{}, common.Allowlist...)}}
	return manager, manager.save()
}

// Start connects every enabled account.
func (m *Manager) Start(ctx context.Context) {
	m.mu.Lock()
	bots := append([]Bot{}, m.bots...)
	m.mu.Unlock()
	for _, bot := range bots {
		if bot.Enabled {
			m.start(bot)
		}
	}
}

// Bots returns a copy of the registry.
func (m *Manager) Bots() []Bot {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Bot{}, m.bots...)
}

// Primary is the first enabled account, used for the panel and the hooks.
func (m *Manager) Primary() *whatsapp.Client {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, bot := range m.bots {
		if client, ok := m.clients[bot.ID]; ok {
			return client
		}
	}
	for _, bot := range m.bots {
		if bot.Enabled {
			return m.startLocked(bot)
		}
	}
	return nil
}

// Client returns the running client for a bot id.
func (m *Manager) Client(id string) *whatsapp.Client {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.clients[id]
}

// QRPNG returns the current pairing QR image for a bot.
func (m *Manager) QRPNG(id string) ([]byte, error) {
	client := m.Client(id)
	if client == nil {
		return nil, fmt.Errorf("bots: %q is not running", id)
	}
	return client.QRPNG()
}

// Status is the account status name, or "off" when it is not running.
func (m *Manager) Status(id string) string {
	m.mu.Lock()
	client := m.clients[id]
	m.mu.Unlock()
	if client == nil {
		return "off"
	}
	return client.Status().Name()
}

// Phone returns the account's live linked number, falling back to the value
// stored in the registry. The registry only keeps a phone when a previous run
// saved one, so the live session is the source of truth.
func (m *Manager) Phone(id string) string {
	m.mu.Lock()
	client := m.clients[id]
	fallback := ""
	for _, bot := range m.bots {
		if bot.ID == id {
			fallback = bot.Phone
			break
		}
	}
	m.mu.Unlock()
	if client != nil {
		if phone := client.Phone(); phone != "" {
			return phone
		}
	}
	return fallback
}

// Add registers an account and starts it when enabled.
func (m *Manager) Add(bot Bot) (Bot, error) {
	if bot.ID == "" {
		bot.ID = newID()
	}
	m.mu.Lock()
	for _, existing := range m.bots {
		if existing.ID == bot.ID {
			m.mu.Unlock()
			return Bot{}, fmt.Errorf("bots: id %q already exists", bot.ID)
		}
	}
	m.bots = append(m.bots, bot)
	err := m.saveLocked()
	m.mu.Unlock()
	if err != nil {
		return Bot{}, err
	}
	if bot.Enabled {
		m.start(bot)
	}
	return bot, nil
}

// Update applies registry changes (rename, enable/disable, allowlist) and
// restarts the account so its session picks them up.
func (m *Manager) Update(updated Bot) (Bot, error) {
	m.mu.Lock()
	index := -1
	for i := range m.bots {
		if m.bots[i].ID == updated.ID {
			index = i
			break
		}
	}
	if index < 0 {
		m.mu.Unlock()
		return Bot{}, fmt.Errorf("bots: %q not found", updated.ID)
	}
	previous := m.bots[index]
	if updated.Name == "" {
		updated.Name = previous.Name
	}
	updated.Phone = previous.Phone
	m.bots[index] = updated
	client := m.clients[updated.ID]
	delete(m.clients, updated.ID)
	err := m.saveLocked()
	m.mu.Unlock()
	if err != nil {
		return Bot{}, err
	}
	if client != nil {
		client.Close()
	}
	if updated.Enabled {
		m.start(updated)
	}
	return updated, nil
}

// RetryPairing starts a new QR cycle for a not-yet-linked account.
func (m *Manager) RetryPairing(id string) error {
	client := m.Client(id)
	if client == nil {
		return fmt.Errorf("bots: %q is not running", id)
	}
	client.RetryPairing()
	return nil
}

// Reconnect forces a fresh connection for a linked account whose socket died
// permanently (for example a session replaced by another copy of the app).
func (m *Manager) Reconnect(id string) error {
	client := m.Client(id)
	if client == nil {
		return fmt.Errorf("bots: %q is not running", id)
	}
	return client.Reconnect()
}

// Logout unlinks an account's WhatsApp session.
func (m *Manager) Logout(id string) error {
	client := m.Client(id)
	if client == nil {
		return fmt.Errorf("bots: %q is not running", id)
	}
	return client.Logout()
}

// SetCatalog swaps the shared kitchen catalog for every account.
func (m *Manager) SetCatalog(catalog *engine.Catalog) {
	m.kitchen.SetCatalog(catalog)
}

// PublishList recomputes and stores the kitchen list for a date. It only
// touches the shared kitchen, so publishing works without a live account.
func (m *Manager) PublishList(date string) error {
	return m.panel.PublishList(date)
}

// PanelSnapshot is the read model the panel serves. It reads the shared
// kitchen directly (orders and lists stay visible while the bot is off,
// unlinked or disconnected) and reports the first running account's WhatsApp
// state when there is one.
func (m *Manager) PanelSnapshot() panelmodel.Snapshot {
	snapshot := m.panel.PanelSnapshot()
	if client := m.running(); client != nil {
		snapshot.WhatsApp = client.WhatsAppState()
	}
	return snapshot
}

// OrderDetail resolves one order for the panel's lazy ticket view.
func (m *Manager) OrderDetail(date, number string) (panelmodel.OrderDetail, error) {
	return m.panel.OrderDetail(date, number)
}

// RemoveOrder drops an order from a day, as a cancelled webhook would.
func (m *Manager) RemoveOrder(date, number string) error {
	return m.panel.RemoveOrder(date, number)
}

// SetNotifications applies the global notification control from the panel.
func (m *Manager) SetNotifications(notifications panelmodel.Notifications) {
	m.panel.SetNotifications(notifications)
}

// Handler mounts the order hooks over the shared kitchen, so webhooks keep
// ingesting orders without a live WhatsApp account.
func (m *Manager) Handler() http.Handler {
	return m.panel.Handler()
}

// SyncFromExport runs the store's WP All Export and makes the synced days
// match it. It never needs a WhatsApp connection.
func (m *Manager) SyncFromExport(ctx context.Context, fetch whatsapp.ExportRows, dates []string) (whatsapp.SyncResult, error) {
	return m.panel.SyncFromExport(ctx, fetch, dates)
}

// running returns the first account with a live client, if any, without
// starting one.
func (m *Manager) running() *whatsapp.Client {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, bot := range m.bots {
		if client, ok := m.clients[bot.ID]; ok {
			return client
		}
	}
	return nil
}

// Remove stops and drops an account.
func (m *Manager) Remove(id string) error {
	m.mu.Lock()
	client := m.clients[id]
	delete(m.clients, id)
	kept := m.bots[:0]
	found := false
	for _, bot := range m.bots {
		if bot.ID == id {
			found = true
			continue
		}
		kept = append(kept, bot)
	}
	m.bots = kept
	err := m.saveLocked()
	m.mu.Unlock()
	if client != nil {
		client.Close()
	}
	if !found {
		return fmt.Errorf("bots: %q not found", id)
	}
	return err
}

// Close stops every account.
func (m *Manager) Close() {
	m.mu.Lock()
	clients := make([]*whatsapp.Client, 0, len(m.clients))
	for _, client := range m.clients {
		clients = append(clients, client)
	}
	m.mu.Unlock()
	for _, client := range clients {
		client.Close()
	}
}

func (m *Manager) start(bot Bot) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startLocked(bot)
}

func (m *Manager) startLocked(bot Bot) *whatsapp.Client {
	if client, ok := m.clients[bot.ID]; ok {
		return client
	}
	client, err := whatsapp.New(whatsapp.Config{
		DataDir:       filepath.Join(m.common.BaseDir, "bots", bot.ID),
		Allowlist:     bot.Allowlist,
		Loopback:      bot.Loopback,
		PairingPhone:  m.common.PairingPhone,
		HookToken:     m.common.HookToken,
		WebhookSecret: m.common.WebhookSecret,
		Debug:         m.common.Debug,
		Logger:        m.log,
		Kitchen:       m.kitchen,
		OnOrder:       m.common.OnOrder,
		Dispatcher:    m.common.Dispatcher,
		OnQR: func(page string) {
			if m.common.OnQR != nil {
				m.common.OnQR(bot.ID, page)
			}
		},
		OnLinked: func() {
			if m.common.OnLinked != nil {
				m.common.OnLinked(bot.ID)
			}
		},
		OnStatus: func(status whatsapp.Status) {
			if m.common.OnStatus != nil {
				m.common.OnStatus(bot.ID, status)
			}
		},
	})
	if err != nil {
		m.log.Printf("bots: %s: %v", bot.ID, err)
		return nil
	}
	m.clients[bot.ID] = client
	go func() {
		if err := client.Start(context.Background()); err != nil {
			m.log.Printf("bots: %s: %v", bot.ID, err)
		}
	}()
	return client
}

func (m *Manager) save() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.saveLocked()
}

func (m *Manager) saveLocked() error {
	if err := os.MkdirAll(m.common.BaseDir, 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(m.bots, "", "\t")
	if err != nil {
		return err
	}
	path := filepath.Join(m.common.BaseDir, "bots.json")
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}

// migrateLegacy moves a single session into bots/default/ so the migration is
// invisible to an existing install.
func migrateLegacy(baseDir string, logger *log.Logger) {
	source := filepath.Join(baseDir, "whatsapp.db")
	target := filepath.Join(baseDir, "bots", "default", "whatsapp.db")
	if info, err := os.Stat(source); err != nil || info.IsDir() {
		return
	}
	if _, err := os.Stat(target); err == nil {
		return
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		logger.Printf("bots: migration: %v", err)
		return
	}
	for _, suffix := range []string{"", "-wal", "-shm"} {
		from := source + suffix
		if _, err := os.Stat(from); err != nil {
			continue
		}
		_ = os.Rename(from, target+suffix)
	}
	logger.Printf("bots: migrated the existing session into bots/default/")
}

func newID() string {
	raw := make([]byte, 6)
	if _, err := rand.Read(raw); err != nil {
		return "bot"
	}
	return hex.EncodeToString(raw)
}
