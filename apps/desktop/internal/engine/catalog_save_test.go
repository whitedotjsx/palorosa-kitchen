package engine

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSeedCatalogIsValid(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "..", "data", "seed.json"))
	if err != nil {
		t.Skip("seed not available:", err)
	}
	_, problems, err := ParseCatalog(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(problems) > 0 {
		t.Fatalf("seed has problems: %v", problems)
	}
}

func TestSaveCatalogRejectsBrokenReferences(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seed.json")
	raw := []byte(`{"schemaVersion":1,"units":[{"id":"a","name":"A"}],"products":[{"id":"p","name":"P"}],
		"recipes":[{"productId":"p","components":[{"kind":"fixed","unitId":"missing","quantity":1}]}],"containers":[]}`)
	if _, err := SaveCatalog(path, raw); err == nil {
		t.Fatal("expected a validation error")
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatal("invalid catalog must not be written")
	}
}

func TestSaveCatalogWritesAndKeepsBackup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seed.json")
	if err := os.WriteFile(path, []byte(`{"old":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	raw := []byte(`{"schemaVersion":1,"units":[{"id":"a","name":"A"}],"products":[{"id":"p","name":"P"}],
		"recipes":[{"productId":"p","components":[{"kind":"fixed","unitId":"a","quantity":2}]}],"containers":[],"extra":{"kept":1}}`)
	catalog, err := SaveCatalog(path, raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(catalog.Recipes) != 1 {
		t.Fatal("catalog not decoded")
	}
	written, _ := os.ReadFile(path)
	if !strings.Contains(string(written), `"kept": 1`) {
		t.Fatalf("unknown fields must survive: %s", written)
	}
	backup, _ := os.ReadFile(path + ".bak")
	if string(backup) != `{"old":true}` {
		t.Fatalf("backup missing: %s", backup)
	}
}
