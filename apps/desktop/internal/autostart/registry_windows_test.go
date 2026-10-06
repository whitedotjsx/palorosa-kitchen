//go:build windows

package autostart

import (
	"os"
	"testing"
)

// TestRegistryRoundTrip is a manual check: it writes the real Run entry, reads
// it back and removes it. Kept behind an env gate so the normal suite never
// touches the user's registry.
func TestRegistryRoundTrip(t *testing.T) {
	if os.Getenv("PALOROSA_AUTOSTART_CHECK") != "1" {
		t.Skip("set PALOROSA_AUTOSTART_CHECK=1 to run against the real registry")
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if err := Enable(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = Disable() }()
	if got := Current(); got != Command(exe) {
		t.Fatalf("registry value = %q, want %q", got, Command(exe))
	}
	if err := Ensure(exe); err != nil {
		t.Fatal(err)
	}
	if err := Disable(); err != nil {
		t.Fatal(err)
	}
	if Current() != "" || Enabled() {
		t.Fatal("entry was not removed")
	}
}
