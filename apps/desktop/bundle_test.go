//go:build windows

package main

import (
	"bytes"
	"compress/gzip"
	"os"
	"path/filepath"
	"testing"
)

func gzipBytes(t *testing.T, raw []byte) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := gzip.NewWriter(&buffer)
	if _, err := writer.Write(raw); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func TestDeployPayloadGunzipsOnce(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bin", "cloudflared.exe")
	payload := gzipBytes(t, []byte("connector"))

	if err := deployPayload(payload, path); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(path); err != nil || string(raw) != "connector" {
		t.Fatalf("deployed = %q err=%v", raw, err)
	}
	if _, err := os.Stat(path + ".sha256"); err != nil {
		t.Fatalf("marker missing: %v", err)
	}

	// A matching marker skips the rewrite, so a running connector is not
	// replaced under itself.
	if err := os.WriteFile(path, []byte("tampered"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := deployPayload(payload, path); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(path); string(raw) != "tampered" {
		t.Fatalf("payload was rewritten with a matching marker: %q", raw)
	}

	// A new payload replaces the file and the marker.
	if err := deployPayload(gzipBytes(t, []byte("newer")), path); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(path); string(raw) != "newer" {
		t.Fatalf("deployed = %q", raw)
	}

	// Broken input fails instead of writing garbage.
	if err := deployPayload([]byte("not gzip"), path); err == nil {
		t.Fatal("expected an error for a non-gzip payload")
	}
}
