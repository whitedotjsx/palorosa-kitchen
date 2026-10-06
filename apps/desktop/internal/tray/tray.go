//go:build windows

// Package tray builds the system tray icon and menu.
package tray

import (
	"strings"
	"sync"

	"github.com/getlantern/systray"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/labels"
)

// Handlers are the callbacks invoked from tray menu clicks.
type Handlers struct {
	OnOpen            func()
	OnList            func()
	OnLink            func()
	OnToggleAutostart func(enabled bool)
	AutostartEnabled  bool
	OnUpdate          func()
	OnHandoff         func()
	OnQuit            func()
}

var (
	statusMu      sync.Mutex
	statusItem    *systray.MenuItem
	tunnelItem    *systray.MenuItem
	pendingStatus string
	pendingTunnel string
)

// Register creates the tray icon and menu. It must run on the main thread and
// returns immediately: the shared window message loop pumps its events.
func Register(icon []byte, text labels.TrayLabels, handlers Handlers) {
	systray.Register(func() {
		systray.SetIcon(icon)
		systray.SetTooltip(text.Tooltip)

		open := systray.AddMenuItem(text.Open, text.OpenHint)
		list := systray.AddMenuItem(text.List, text.ListHint)
		status := systray.AddMenuItem(statusText(text, pendingStatus), "")
		status.Disable()
		tunnel := systray.AddMenuItem(tunnelText(text, pendingTunnel), "")
		tunnel.Disable()
		link := systray.AddMenuItem(text.Link, text.LinkHint)
		systray.AddSeparator()
		autostart := systray.AddMenuItemCheckbox(text.Autostart, text.AutostartHint, handlers.AutostartEnabled)
		update := systray.AddMenuItem(text.Update, text.UpdateHint)
		if handlers.OnUpdate == nil {
			update.Hide()
		}
		handoff := systray.AddMenuItem(text.Handoff, text.HandoffHint)
		if handlers.OnHandoff == nil {
			handoff.Hide()
		}
		systray.AddSeparator()
		quit := systray.AddMenuItem(text.Quit, text.QuitHint)

		statusMu.Lock()
		statusItem = status
		tunnelItem = tunnel
		if pendingStatus != "" {
			status.SetTitle(statusText(text, pendingStatus))
		}
		if pendingTunnel != "" {
			tunnel.SetTitle(tunnelText(text, pendingTunnel))
		}
		statusMu.Unlock()

		go func() {
			for {
				select {
				case <-open.ClickedCh:
					call(handlers.OnOpen)
				case <-list.ClickedCh:
					call(handlers.OnList)
				case <-link.ClickedCh:
					call(handlers.OnLink)
				case <-autostart.ClickedCh:
					enabled := !autostart.Checked()
					if enabled {
						autostart.Check()
					} else {
						autostart.Uncheck()
					}
					if handlers.OnToggleAutostart != nil {
						handlers.OnToggleAutostart(enabled)
					}
				case <-update.ClickedCh:
					call(handlers.OnUpdate)
				case <-handoff.ClickedCh:
					call(handlers.OnHandoff)
				case <-quit.ClickedCh:
					call(handlers.OnQuit)
					return
				}
			}
		}()
	}, nil)
}

// SetStatus updates the disabled WhatsApp status line. Safe before the tray is
// ready (the value is applied when the menu is built).
func SetStatus(value string) {
	statusMu.Lock()
	defer statusMu.Unlock()
	pendingStatus = value
	if statusItem != nil {
		statusItem.SetTitle(statusText(labels.Tray, value))
	}
}

// SetTunnel updates the disabled Cloudflare tunnel status line.
func SetTunnel(value string) {
	statusMu.Lock()
	defer statusMu.Unlock()
	pendingTunnel = value
	if tunnelItem != nil {
		tunnelItem.SetTitle(tunnelText(labels.Tray, value))
	}
}

func statusText(text labels.TrayLabels, value string) string {
	return strings.ReplaceAll(text.Status, "{status}", value)
}

func tunnelText(text labels.TrayLabels, value string) string {
	if value == "" {
		value = labels.Tunnel.Disabled
	}
	return strings.ReplaceAll(text.Tunnel, "{status}", value)
}

func call(handler func()) {
	if handler != nil {
		handler()
	}
}
