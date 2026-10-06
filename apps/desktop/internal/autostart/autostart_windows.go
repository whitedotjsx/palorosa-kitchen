//go:build windows

// Package autostart registers the app to open when the user logs in.
package autostart

import (
	"os"

	"golang.org/x/sys/windows/registry"
)

const (
	runKey    = `Software\Microsoft\Windows\CurrentVersion\Run`
	valueName = "PalorosaKitchen"
)

// Command is the Run value written for an executable. Windows parses it as a
// command line, so the path must be quoted by hand: a Go-escaped string
// (%q / strconv.Quote) doubles the backslashes, which Windows does not expect
// in the value even though it usually tolerates them.
func Command(exe string) string {
	return `"` + exe + `"`
}

// Current returns the command stored in the Run entry, or "" when the entry
// does not exist.
func Current() string {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return ""
	}
	defer key.Close()
	value, _, err := key.GetStringValue(valueName)
	if err != nil {
		return ""
	}
	return value
}

func write(command string) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	return key.SetStringValue(valueName, command)
}

// Enable registers the current executable under HKCU Run.
func Enable() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	return write(Command(exe))
}

// Ensure registers exe, repairing an entry that points somewhere else. It is
// what makes autostart survive a moved or rebuilt executable: on every start
// the stored command is compared against the running exe.
func Ensure(exe string) error {
	if Current() == Command(exe) {
		return nil
	}
	return write(Command(exe))
}

// Disable removes the Run entry. Missing entry is not an error.
func Disable() error {
	key, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer key.Close()
	if err := key.DeleteValue(valueName); err != nil && err != registry.ErrNotExist {
		return err
	}
	return nil
}

// Enabled reports whether the Run entry is present.
func Enabled() bool {
	return Current() != ""
}
