//go:build windows

package cftunnel

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMaterializeWritesOnce(t *testing.T) {
	dir := t.TempDir()
	payload := []byte("library-bytes")

	path, err := Materialize(dir, payload)
	if err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(path); err != nil || string(raw) != "library-bytes" {
		t.Fatalf("deployed = %q err=%v", raw, err)
	}
	if _, err := os.Stat(path + ".sha256"); err != nil {
		t.Fatalf("marker missing: %v", err)
	}

	// A matching marker skips the rewrite, so a loaded library is not replaced
	// under itself.
	if err := os.WriteFile(path, []byte("tampered"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Materialize(dir, payload); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(path); string(raw) != "tampered" {
		t.Fatalf("payload was rewritten with a matching marker: %q", raw)
	}

	// A new payload replaces the file and the marker.
	if _, err := Materialize(dir, []byte("newer")); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(path); string(raw) != "newer" {
		t.Fatalf("deployed = %q", raw)
	}
}

// TestLoadAndReportError exercises the whole C ABI bridge with the built
// library: load, create, start (with a token the library rejects before any
// network call), read the error, stop and free.
func TestLoadAndReportError(t *testing.T) {
	library := filepath.Join("..", "..", "third_party", "cf-quick-tunnel-rs", "target", "release", "cloudflare_quick_tunnel.dll")
	payload, err := os.ReadFile(library)
	if err != nil {
		t.Skipf("build the vendored library first (cargo build --release --lib): %v", err)
	}
	path, err := Materialize(t.TempDir(), payload)
	if err != nil {
		t.Fatal(err)
	}
	tunnel, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	defer tunnel.Free()

	if err := tunnel.Start("not base64 !!!", "ingress:\n  - hostname: cocina.example.com\n    service: http://127.0.0.1:5211\n"); err != nil {
		t.Fatalf("start should accept the request and fail asynchronously: %v", err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if tunnel.State() == Errored {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if state := tunnel.State(); state != Errored {
		t.Fatalf("state = %d, want errored", state)
	}
	if message := tunnel.LastError(); message == "" {
		t.Fatal("expected an error message from the library")
	}
	tunnel.Stop()
}

func TestMaterializeRejectsEmptyPayload(t *testing.T) {
	if _, err := Materialize(t.TempDir(), nil); err == nil {
		t.Fatal("expected an error for an empty payload")
	}
}

func TestMaterializePath(t *testing.T) {
	dir := t.TempDir()
	path, err := Materialize(dir, []byte("x"))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "bin", "cf-tunnel.dll"); path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
}
