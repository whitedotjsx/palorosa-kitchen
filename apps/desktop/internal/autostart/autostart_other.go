//go:build !windows

package autostart

import "errors"

var errUnsupported = errors.New("autostart is only implemented on Windows")

// Enable is a no-op outside Windows.
func Enable() error { return errUnsupported }

// Disable is a no-op outside Windows.
func Disable() error { return errUnsupported }

// Enabled always reports false outside Windows.
func Enabled() bool { return false }
