//go:build !windows

package autostart

import "errors"

var errUnsupported = errors.New("autostart is only implemented on Windows")

// Command is the Run value written for an executable (Windows only).
func Command(exe string) string { return `"` + exe + `"` }

// Current always reports no entry outside Windows.
func Current() string { return "" }

// Enable is a no-op outside Windows.
func Enable() error { return errUnsupported }

// Ensure is a no-op outside Windows.
func Ensure(string) error { return errUnsupported }

// Disable is a no-op outside Windows.
func Disable() error { return errUnsupported }

// Enabled always reports false outside Windows.
func Enabled() bool { return false }
