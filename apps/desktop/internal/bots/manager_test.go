package bots

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultRegistryAndPersistence(t *testing.T) {
	dir := t.TempDir()
	manager, err := New(Common{BaseDir: dir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if got := manager.Bots(); len(got) != 1 || got[0].ID != "default" {
		t.Fatalf("default registry = %+v", got)
	}
	if _, err := manager.Add(Bot{ID: "caja", Name: "Caja"}); err != nil {
		t.Fatal(err)
	}
	reopened, err := New(Common{BaseDir: dir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(reopened.Bots()) != 2 {
		t.Fatalf("bots = %d, want 2", len(reopened.Bots()))
	}
	if err := reopened.Remove("caja"); err != nil {
		t.Fatal(err)
	}
	if len(reopened.Bots()) != 1 {
		t.Fatal("caja not removed")
	}
}

func TestPhoneFallsBackToRegistry(t *testing.T) {
	dir := t.TempDir()
	raw := `[{"id":"default","name":"Cocina","phone":"+573001112233","enabled":false}]`
	if err := os.WriteFile(filepath.Join(dir, "bots.json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	manager, err := New(Common{BaseDir: dir}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if phone := manager.Phone("default"); phone != "+573001112233" {
		t.Fatalf("phone = %q", phone)
	}
	if phone := manager.Phone("missing"); phone != "" {
		t.Fatalf("missing phone = %q", phone)
	}
}

func TestMigrationMovesLegacySession(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "whatsapp.db"), []byte("db"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := New(Common{BaseDir: dir}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "bots", "default", "whatsapp.db")); err != nil {
		t.Fatalf("legacy session not migrated: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "whatsapp.db")); !os.IsNotExist(err) {
		t.Fatal("legacy session should be gone after migration")
	}
}

func TestNewSeedsLegacyAllowlist(t *testing.T) {
	dir := t.TempDir()
	manager, err := New(Common{BaseDir: dir, Allowlist: []string{"573238022428"}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	list := manager.Bots()
	if len(list) != 1 || len(list[0].Allowlist) != 1 || list[0].Allowlist[0] != "573238022428" {
		t.Fatalf("default bot allowlist = %+v", list)
	}
}

func TestPanelWorksWithoutAccounts(t *testing.T) {
	dir := t.TempDir()
	raw := `[{"id":"default","name":"Cocina","enabled":false,"allowlist":["57300"]}]`
	if err := os.WriteFile(filepath.Join(dir, "bots.json"), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	state := `{"orders":{"2026-10-04":{"7":[{"productText":"Box Mujer","quantity":1,"source":"breakfast"}]}}}`
	if err := os.WriteFile(filepath.Join(dir, "bot-state.json"), []byte(state), 0o600); err != nil {
		t.Fatal(err)
	}
	manager, err := New(Common{BaseDir: dir, CatalogPath: "../../../../data/seed.json"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	// With no enabled account the panel still reads the shared kitchen.
	snapshot := manager.PanelSnapshot()
	if snapshot.WhatsApp.Status != "disabled" {
		t.Fatalf("whatsapp status = %q", snapshot.WhatsApp.Status)
	}
	if len(snapshot.Days) != 1 || len(snapshot.Days[0].Orders) != 1 || snapshot.Days[0].Orders[0].Number != "7" {
		t.Fatalf("days = %+v", snapshot.Days)
	}
	if _, err := manager.OrderDetail("2026-10-04", "7"); err != nil {
		t.Fatalf("order detail: %v", err)
	}
	if err := manager.PublishList("2026-10-04"); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if err := manager.RemoveOrder("2026-10-04", "7"); err != nil {
		t.Fatalf("remove: %v", err)
	}
	if days := manager.PanelSnapshot().Days; len(days) != 0 {
		t.Fatalf("order not removed: %+v", days)
	}
}
