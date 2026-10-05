//go:build !windows

package toast

// Show is a no-op outside Windows, where the desktop shell does not run.
func Show(string, string) {}
