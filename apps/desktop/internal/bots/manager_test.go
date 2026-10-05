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
