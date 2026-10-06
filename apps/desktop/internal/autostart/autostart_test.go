package autostart

import "testing"

// TestCommandQuotesWithoutGoEscapes guards the Run value format: Windows
// expects a quoted path with single backslashes, not the Go-escaped string
// (%q / strconv.Quote) that doubles them.
func TestCommandQuotesWithoutGoEscapes(t *testing.T) {
	exe := `C:\Program Files\Palorosa\palorosa-kitchen.exe`
	want := `"C:\Program Files\Palorosa\palorosa-kitchen.exe"`
	if got := Command(exe); got != want {
		t.Fatalf("Command = %q, want %q", got, want)
	}
}
