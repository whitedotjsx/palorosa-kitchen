package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPickPrefersSettings(t *testing.T) {
	cases := []struct {
		name     string
		settings string
		env      string
		want     string
	}{
		{"settings wins", "from-settings", "from-env", "from-settings"},
		{"env when settings empty", "", "from-env", "from-env"},
		{"empty when both empty", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := pick(tc.settings, tc.env); got != tc.want {
				t.Fatalf("pick(%q, %q) = %q, want %q", tc.settings, tc.env, got, tc.want)
			}
		})
	}
}

func TestEnsureCatalogPrefersExistingPath(t *testing.T) {
	dir := t.TempDir()
	existing := filepath.Join(dir, "seed.json")
	if err := os.WriteFile(existing, []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := EnsureCatalog(existing, filepath.Join(dir, "data"), []byte(`{"embedded":true}`)); got != existing {
		t.Fatalf("EnsureCatalog = %q, want the existing path", got)
	}
}

func TestEnsureCatalogDeploysEmbeddedSeedOnce(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "otro-pc", "seed.json")
	target := filepath.Join(dir, "data", "catalog.json")

	got := EnsureCatalog(missing, filepath.Join(dir, "data"), []byte(`{"seed":1}`))
	if got != target {
		t.Fatalf("EnsureCatalog = %q, want %q", got, target)
	}
	raw, err := os.ReadFile(target)
	if err != nil || string(raw) != `{"seed":1}` {
		t.Fatalf("deployed = %q err=%v", raw, err)
	}

	// An existing catalog.json is never overwritten, so panel edits survive.
	if err := os.WriteFile(target, []byte(`{"edited":true}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := EnsureCatalog(missing, filepath.Join(dir, "data"), []byte(`{"seed":2}`)); got != target {
		t.Fatalf("EnsureCatalog = %q, want %q", got, target)
	}
	if raw, _ := os.ReadFile(target); string(raw) != `{"edited":true}` {
		t.Fatalf("catalog.json was overwritten: %q", raw)
	}
}

func TestEnsureCatalogWithoutEmbeddedKeepsConfigured(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "nope.json")
	if got := EnsureCatalog(missing, dir, nil); got != missing {
		t.Fatalf("EnsureCatalog = %q, want the configured path", got)
	}
}
