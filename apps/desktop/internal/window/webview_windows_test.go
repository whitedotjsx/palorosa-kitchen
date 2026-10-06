//go:build windows

package window

import "testing"

// TestAvailableOverride covers the forced missing-runtime path. The real
// registry probe depends on the machine, so the override is what lets the
// warning flow be exercised anywhere.
func TestAvailableOverride(t *testing.T) {
	t.Setenv("KITCHEN_NO_WEBVIEW", "1")
	if Available() {
		t.Fatal("KITCHEN_NO_WEBVIEW=1 should force Available() to false")
	}
}
