//go:build windows

// Command palorosa-kitchen is the Windows desktop shell for the kitchen list:
// a WebView2 window plus a system tray icon, autostart and the WhatsApp bot
// (whatsmeow, no headless browser).
package main

import (
	"context"
	_ "embed"
	"encoding/base64"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/systray"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/auth"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/autostart"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/bots"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/config"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/engine"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/labels"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/notify"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/panelmodel"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/panelserver"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/push"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/selftest"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/settings"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/singleinstance"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/toast"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/tray"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/tunnel"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/update"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/whatsapp"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/window"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/wpexport"
)

//go:embed assets/tray.ico
var trayIcon []byte

//go:embed assets/panel-icon.png
var panelIconPNG []byte

//go:embed assets/app.ico
var appIconICO []byte

const (
	listRelative  = "apps/standalone/dist/index.html"
	panelRelative = "apps/panel/dist/index.html"
	mutexName     = `Local\PalorosaKitchen`
	showEventName = `Local\PalorosaKitchenShow`
)

// version is the running release. The build injects it with
// -ldflags "-X main.version=x.y.z" and the updater compares against it.
var version = "0.1.0"

// startedAt is the process start, for the diagnostics uptime.
var startedAt = time.Now()

// shutdownApp stops the running services and quits. startHost installs it; a
// no-op keeps the tray and the restart route safe before that.
var shutdownApp = func() {}

func main() {
	// Console-only modes (release checks and the update end-to-end proof) run
	// before the window, the tray and the single-instance mutex.
	if code, handled := runCLI(os.Args[1:]); handled {
		os.Exit(code)
	}

	// The webview and the tray icon must share the main thread's message loop.
	runtime.LockOSThread()
	// Capture stdout/stderr first: the GUI build has no console, so every
	// diagnostic line would otherwise be lost.
	captureOutput()

	// A copy started by "Guardar y reiniciar" waits here until the previous
	// process releases the single-instance mutex.
	if pid := parseRestartWait(os.Args[1:]); pid != 0 {
		waitForProcessExit(pid)
	}

	// A finished update leaves the replaced exe behind; it is unlocked now.
	update.Cleanup(currentExecutable())

	// One instance only: a second launch shows the running window and exits.
	if already, err := singleinstance.Acquire(mutexName); err != nil {
		fmt.Fprintln(os.Stderr, "singleinstance:", err)
	} else if already {
		_ = singleinstance.SignalShow(showEventName)
		return
	}

	cfg := config.Load()
	logs.AttachFile(filepath.Join(cfg.DataDir, "app.log"))
	updater = newUpdater(restartAfterUpdate)
	// The desktop window loads the panel, which the panel server serves on the
	// loopback port (the standalone list stays a separate build).
	homeURL := fmt.Sprintf("http://127.0.0.1:%d/", cfg.PanelPort)

	store, storeErr := settings.Open(cfg.SettingsPath)
	if storeErr != nil {
		fmt.Fprintln(os.Stderr, "settings:", storeErr)
	}
	// First run without any configuration: ask for the tunnel (and the store
	// credentials) before deciding host or spectator, so no manual restart is
	// needed to reach the invite screen.
	setupMode := !store.Exists()

	options := window.Options{
		Title:  labels.Tray.Title,
		Width:  1100,
		Height: 820,
		URL:    homeURL,
		Debug:  cfg.Debug,
		Hidden: true,
		Icon:   appIconICO,
	}
	if setupMode {
		options.URL = ""
		options.HTML = string(setupHTML)
		options.Hidden = false
	}
	window.Prepare(options)
	singleinstance.ListenShow(showEventName, window.Show)
	// Closing the window hides it; the app keeps running in the tray.
	window.InterceptClose(window.Hide)

	if setupMode {
		bindSetup(store, func() {
			next := config.Load()
			if remoteHost(next) {
				fmt.Fprintln(os.Stderr, "host: otro equipo ya es el host de", next.TunnelHostname, "- modo espectador")
				startSpectator(next)
				return
			}
			startHost(next, store)
		})
		// A minimal tray so closing the setup window does not strand the app.
		tray.Register(trayIcon, labels.Tray, tray.Handlers{
			OnOpen:            window.Show,
			OnList:            window.Show,
			OnLink:            window.Show,
			AutostartEnabled:  autostart.Enabled(),
			OnToggleAutostart: toggleAutostart,
			OnQuit: func() {
				window.Quit()
				systray.Quit()
			},
		})
		window.Run()
		window.Destroy()
		return
	}

	// One host at a time: when another computer already serves the tunnel,
	// this app runs no bots, tunnel or webhooks and shows the host's panel.
	if remoteHost(cfg) {
		fmt.Fprintln(os.Stderr, "host: otro equipo ya es el host de", cfg.TunnelHostname, "- modo espectador")
		startSpectator(cfg)
	} else {
		startHost(cfg, store)
	}
	startAutoUpdate()

	window.Run()
	window.Destroy()
}

// bindSetup exposes the first-run form to the setup page. Saving continues
// with the right role in place, without restarting the app.
func bindSetup(store *settings.Store, onSaved func()) {
	err := window.Bind("palorosaSetup", func(payload setupPayload) error {
		if store == nil {
			return errors.New("configuración no disponible")
		}
		if err := store.Update(applySetup(store.Values(), payload)); err != nil {
			return err
		}
		onSaved()
		return nil
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "setup bind:", err)
	}
}

// startHost runs the full kitchen host: bots, notifications, tunnel, panel and
// the tray. It does not start the window message loop.
func startHost(cfg config.Config, store *settings.Store) {
	homeURL := fmt.Sprintf("http://127.0.0.1:%d/", cfg.PanelPort)
	bot := startBot(cfg, homeURL)
	notifyManager := startNotify(cfg, bot)

	if bot.client != nil {
		if err := window.Bind("palorosaRetryPairing", bot.retryPairing); err != nil {
			fmt.Fprintln(os.Stderr, "bind:", err)
		}
	}

	shutdownApp = func() {
		bot.close()
		stopNotify()
		stopPanel()
		stopTunnel()
		window.Quit()
		systray.Quit()
	}

	startTunnel(cfg)
	startPanel(cfg, bot, notifyManager, store)
	navigateWhenReady(homeURL)

	tray.Register(trayIcon, labels.Tray, tray.Handlers{
		OnOpen:            window.Show,
		OnList:            bot.showList,
		OnLink:            bot.openPairing,
		AutostartEnabled:  autostart.Enabled(),
		OnToggleAutostart: toggleAutostart,
		OnUpdate:          checkUpdatesManually,
		OnQuit:            shutdownApp,
	})
}

// restartApp relaunches the app and quits this process. Used by the panel's
// "Guardar y reiniciar" and safe to call at any time.
func restartApp() {
	if err := relaunch(); err != nil {
		fmt.Fprintln(os.Stderr, "restart:", err)
		return
	}
	shutdownApp()
}

// startSpectator shows the host's panel in the window, with a reduced tray.
func startSpectator(cfg config.Config) {
	window.Navigate(spectatorURL(cfg))
	tray.SetTunnel(labels.Tunnel.Spectator)
	// The updater restarts the app through this, so a spectator installs a
	// new exe too.
	shutdownApp = func() {
		window.Quit()
		systray.Quit()
	}
	tray.Register(trayIcon, labels.Tray, tray.Handlers{
		OnOpen:            window.Show,
		OnList:            window.Show,
		OnLink:            window.Show,
		AutostartEnabled:  autostart.Enabled(),
		OnToggleAutostart: toggleAutostart,
		OnUpdate:          checkUpdatesManually,
		OnQuit:            shutdownApp,
	})
	window.Show()
}

// botShell owns the WhatsApp accounts and tracks whether the pairing page is
// on screen, so a QR refresh does not yank the user away from the list.
type botShell struct {
	client        *whatsapp.Client
	manager       *bots.Manager
	homeURL       string
	mu            sync.Mutex
	pairingOnView bool
}

func startBot(cfg config.Config, homeURL string) *botShell {
	shell := &botShell{homeURL: homeURL}
	if !cfg.WhatsAppEnabled {
		tray.SetStatus(labels.WhatsApp.Disabled)
		return shell
	}

	manager, err := bots.New(bots.Common{
		BaseDir:       cfg.DataDir,
		CatalogPath:   cfg.CatalogPath,
		PairingPhone:  cfg.PairingPhone,
		HookToken:     cfg.HookToken,
		WebhookSecret: cfg.WebhookSecret,
		Debug:         cfg.Debug,
		OnOrder:       func() { publishPanel("orders") },
		OnQR:          func(_, page string) { shell.refreshPairing(page) },
		OnLinked:      func(string) { shell.onLinked() },
		OnStatus: func(_ string, status whatsapp.Status) {
			tray.SetStatus(status.Label())
			publishPanel("whatsapp")
		},
		Dispatcher: dispatchNotify,
	}, log.Default())
	if err != nil {
		fmt.Fprintln(os.Stderr, "bots:", err)
		tray.SetStatus(labels.WhatsApp.Disabled)
		return shell
	}
	shell.manager = manager
	manager.Start(context.Background())
	shell.client = manager.Primary()
	if shell.client == nil {
		fmt.Fprintln(os.Stderr, "bots: no enabled account")
		tray.SetStatus(labels.WhatsApp.Disabled)
		return shell
	}

	if len(cfg.Allowlist) == 0 {
		fmt.Fprintln(os.Stderr, "whatsapp: WHATSAPP_ALLOWLIST is empty, commands are ignored")
	}

	// No linked session: the app still opens on the list. Pairing is started on
	// demand from the tray ("Vincular WhatsApp") or the panel's Bots screen.
	return shell
}

// retryPairing is bound to the Retry button on the pairing page.
func (b *botShell) retryPairing() {
	if b.client != nil {
		b.client.RetryPairing()
	}
}

func (b *botShell) openPairing() {
	b.mu.Lock()
	b.pairingOnView = true
	b.mu.Unlock()
	if b.client != nil {
		window.Navigate(b.client.PairingPage())
	}
	window.Show()
}

// refreshPairing updates the QR in place, but only while the pairing page is
// the one on screen, so a rotation never yanks the user away from the list.
func (b *botShell) refreshPairing(page string) {
	b.mu.Lock()
	visible := b.pairingOnView
	b.mu.Unlock()
	if visible {
		window.Navigate(page)
	}
}

func (b *botShell) showList() {
	b.mu.Lock()
	b.pairingOnView = false
	b.mu.Unlock()
	window.Navigate(b.homeURL)
	window.Show()
}

// onLinked runs when the connection is established. It only brings the window
// up when the operator was pairing; a normal startup stays in the tray.
func (b *botShell) onLinked() {
	b.mu.Lock()
	wasPairing := b.pairingOnView
	b.pairingOnView = false
	b.mu.Unlock()
	window.Navigate(b.homeURL)
	if wasPairing {
		window.Show()
	}
}

func (b *botShell) close() {
	if b.manager != nil {
		b.manager.Close()
		return
	}
	if b.client != nil {
		b.client.Close()
	}
}

var (
	tunnelMu     sync.Mutex
	activeTunnel *tunnel.Manager
)

// startTunnel creates (once) and runs the Cloudflare named tunnel that exposes
// the hook server. It is a no-op when TUNNEL_HOSTNAME is empty.
func startTunnel(cfg config.Config) {
	if cfg.TunnelHostname == "" {
		tray.SetTunnel(labels.Tunnel.Disabled)
		return
	}
	manager, err := tunnel.New(tunnel.Config{
		Name:            cfg.TunnelName,
		Hostname:        cfg.TunnelHostname,
		Service:         cfg.TunnelService,
		DataDir:         cfg.DataDir,
		CloudflaredPath: cloudflaredPath(cfg),
		OnStatus: func(status tunnel.Status, _ string) {
			tray.SetTunnel(labels.TunnelLabel(int(status)))
			publishPanel("tunnel")
		},
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "tunnel:", err)
		tray.SetTunnel(labels.Tunnel.Errored)
		return
	}
	tunnelMu.Lock()
	activeTunnel = manager
	tunnelMu.Unlock()

	go func() {
		ctx := context.Background()
		if err := manager.Ensure(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "tunnel:", err)
			if errors.Is(err, tunnel.ErrNotLoggedIn) {
				fmt.Fprintln(os.Stderr, "tunnel: ejecuta 'cloudflared tunnel login' una vez y vuelve a abrir la aplicación")
			}
			tray.SetTunnel(labels.Tunnel.Errored)
			return
		}
		fmt.Fprintf(os.Stderr, "tunnel: %s -> %s\n", manager.PublicURL(), cfg.TunnelService)
		if err := manager.Run(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "tunnel:", err)
		}
	}()
}

func stopTunnel() {
	tunnelMu.Lock()
	manager := activeTunnel
	tunnelMu.Unlock()
	if manager != nil {
		manager.Stop()
	}
}

var (
	panelMu     sync.Mutex
	panelCancel context.CancelFunc
	activePanel *panelserver.Server
)

// publishPanel pushes an SSE event to the open panel sessions.
func publishPanel(event string) {
	panelMu.Lock()
	server := activePanel
	panelMu.Unlock()
	if server != nil {
		server.Publish(event)
	}
}

var (
	notifyMu     sync.Mutex
	notifyCancel context.CancelFunc
	activeNotify *notify.Manager
)

// startNotify loads the notification targets and starts the scheduler that
// flushes their queued notices (D26).
func startNotify(cfg config.Config, bot *botShell) *notify.Manager {
	var sender notify.Sender = func(string, string, string) error { return errors.New("whatsapp apagado") }
	if bot != nil {
		sender = func(botID, phone, text string) error {
			if bot.manager != nil {
				if client := bot.manager.Client(botID); client != nil {
					return client.SendTo(phone, text)
				}
			}
			if bot.client != nil {
				return bot.client.SendTo(phone, text)
			}
			return errors.New("whatsapp apagado")
		}
	}
	manager, err := notify.New(filepath.Join(cfg.DataDir, "targets.json"), sender, log.Default())
	if err != nil {
		fmt.Fprintln(os.Stderr, "notify:", err)
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	notifyMu.Lock()
	activeNotify = manager
	notifyCancel = cancel
	notifyMu.Unlock()
	go manager.Run(ctx)
	return manager
}

func stopNotify() {
	notifyMu.Lock()
	cancel := notifyCancel
	notifyMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

// dispatchNotify routes a notice through the target manager when it is up. It
// also raises a native Windows toast, because WebView2 cannot deliver Web Push
// and the panel cannot request the web Notification permission in the desktop.
func dispatchNotify(text, kind, date string) int {
	title, body, _ := strings.Cut(strings.TrimSpace(text), "\n")
	toast.Show(title, body)
	notifyMu.Lock()
	manager := activeNotify
	notifyMu.Unlock()
	if manager == nil {
		return 0
	}
	return manager.Dispatch(text, kind, date)
}

// startPanel runs the local panel server. It exposes the settings API and,
// when the bot is up, re-mounts the hook routes on the same origin so one
// tunnel can front both (D24, D27). The panel is loopback-only for now. The
// store is the one main already opened, so the setup form and the panel share
// the same in-memory configuration.
func startPanel(cfg config.Config, bot *botShell, targets *notify.Manager, store *settings.Store) {
	if store == nil {
		opened, err := settings.Open(cfg.SettingsPath)
		if err != nil {
			fmt.Fprintln(os.Stderr, "settings:", err)
		} else {
			store = opened
		}
	}
	var hooks http.Handler
	if bot != nil && bot.client != nil {
		hooks = bot.client.Handler()
	}
	var sessions *auth.Manager
	openedAuth, authErr := auth.New(filepath.Join(cfg.DataDir, "sessions.json"))
	if authErr != nil {
		fmt.Fprintln(os.Stderr, "auth:", authErr)
	} else {
		sessions = openedAuth
	}
	var pusher *push.Manager
	openedPush, pushErr := push.New(filepath.Join(cfg.DataDir, "push.json"), log.Default())
	if pushErr != nil {
		fmt.Fprintln(os.Stderr, "push:", pushErr)
	} else {
		pusher = openedPush
	}
	baseURL := ""
	if cfg.TunnelHostname != "" {
		baseURL = "https://" + cfg.TunnelHostname
	}
	selfDeps := selftest.Deps{
		CatalogPath:   func() string { return cfg.CatalogPath },
		PanelPort:     func() int { return cfg.PanelPort },
		WebhookSecret: func() string { return cfg.WebhookSecret },
		// Read the live settings on every run so credentials saved in Ajustes
		// are tested right away, without restarting the app.
		WP: func() selftest.WPCredentials {
			if store == nil {
				return selftest.WPCredentials{
					AdminURL:       cfg.WPAdminURL,
					AdminUser:      cfg.WPAdminUser,
					AdminPassword:  cfg.WPAdminPassword,
					ConsumerKey:    cfg.WPConsumerKey,
					ConsumerSecret: cfg.WPConsumerSecret,
				}
			}
			wp := config.ResolveWP(store.Values())
			return selftest.WPCredentials{
				AdminURL:       wp.AdminURL,
				AdminUser:      wp.AdminUser,
				AdminPassword:  wp.AdminPassword,
				ConsumerKey:    wp.ConsumerKey,
				ConsumerSecret: wp.ConsumerSecret,
			}
		},
		TunnelURL: func() (string, string) {
			tunnelMu.Lock()
			manager := activeTunnel
			tunnelMu.Unlock()
			if manager == nil {
				if cfg.TunnelHostname == "" {
					return "", "disabled"
				}
				return "", "stopped"
			}
			status, _ := manager.Status()
			return manager.PublicURL(), status.Name()
		},
	}
	if bot != nil && bot.manager != nil {
		manager := bot.manager
		selfDeps.Bots = func() []selftest.BotStatus {
			out := []selftest.BotStatus{}
			for _, b := range manager.Bots() {
				out = append(out, selftest.BotStatus{Name: b.Name, Enabled: b.Enabled, Status: manager.Status(b.ID)})
			}
			return out
		}
	}
	if pusher != nil {
		selfDeps.PushCount = pusher.Count
	}
	if targets != nil {
		selfDeps.TargetsCount = func() (int, int) {
			list := targets.Targets()
			enabled := 0
			for _, target := range list {
				if target.Enabled {
					enabled++
				}
			}
			return enabled, len(list)
		}
	}
	server := panelserver.New(panelserver.Config{
		Port:         cfg.PanelPort,
		Settings:     store,
		Auth:         sessions,
		PanelBaseURL: baseURL,
		Panel:        readPanel(cfg),
		PanelIcon:    readPanelIcon(),
		CatalogPath:  func() string { return cfg.CatalogPath },
		OnCatalogSaved: func(catalog *engine.Catalog) {
			if bot != nil && bot.manager != nil {
				bot.manager.SetCatalog(catalog)
			}
		},
		PrintTemplatePath: filepath.Join(cfg.DataDir, "print-template.json"),
		Targets:           targets,
		Push:              pusher,
		Bots:              bot.manager,
		Autostart:         autostart.Enabled,
		SetAutostart:      setAutostart,
		SelfTest:          selftest.New(selfDeps),
		Update:            updater,
		Diagnostics: func() panelserver.Diagnostics {
			return panelserver.Diagnostics{
				Version:   version,
				Uptime:    time.Since(startedAt).Round(time.Second).String(),
				DataDir:   cfg.DataDir,
				Catalog:   cfg.CatalogPath,
				PanelPort: cfg.PanelPort,
				Debug:     cfg.Debug,
				Logs:      logs.Tail(150),
				LogPath:   filepath.Join(cfg.DataDir, "app.log"),
			}
		},
		LogPath: filepath.Join(cfg.DataDir, "app.log"),
		OpenLogFolder: func() error {
			// explorer.exe returns exit code 1 even on success, so only a
			// failure to launch it is an error.
			return exec.Command("explorer.exe", cfg.DataDir).Start()
		},
		Hooks:      hooks,
		Debug:      cfg.Debug,
		InstanceID: instanceID,
		Machine:    machineID(),
		// The station key spectators present derives from this secret.
		StationSecret: cfg.WebhookSecret,
		Snapshot: func() panelmodel.Snapshot {
			if bot == nil || bot.client == nil {
				return panelmodel.Snapshot{}
			}
			return bot.client.PanelSnapshot()
		},
		SyncOrders: func(ctx context.Context, dates []string) (any, error) {
			if bot == nil || bot.client == nil {
				return nil, errors.New("bot deshabilitado")
			}
			// Read the live settings so credentials saved in Ajustes apply now.
			var values settings.Values
			if store != nil {
				values = store.Values()
			}
			wp := config.ResolveWP(values)
			exporter, err := wpexport.New(wpexport.Config{
				Base:     wp.AdminURL,
				User:     wp.AdminUser,
				Password: wp.AdminPassword,
				ExportID: wp.ExportID,
				CronKey:  wp.ExportCronKey,
				Token:    wp.ExportToken,
			})
			if err != nil {
				return nil, err
			}
			return bot.client.SyncFromExport(ctx, exporter.Rows, dates)
		},
		SyncInterval: func() int {
			if store != nil {
				if minutes := store.Values().SyncMinutes; minutes != nil {
					return *minutes
				}
			}
			return panelserver.DefaultSyncMinutes
		},
		RemoveOrder: func(date, number string) error {
			if bot == nil || bot.client == nil {
				return errors.New("bot deshabilitado")
			}
			return bot.client.RemoveOrder(date, number)
		},
		OrderDetail: func(date, number string) (panelmodel.OrderDetail, error) {
			if bot == nil || bot.client == nil {
				return panelmodel.OrderDetail{}, errors.New("bot deshabilitado")
			}
			return bot.client.OrderDetail(date, number)
		},
		Restart: restartApp,
		SetNotifications: func(notifications panelmodel.Notifications) error {
			if bot == nil || bot.client == nil {
				return errors.New("bot deshabilitado")
			}
			bot.client.SetNotifications(notifications)
			return nil
		},
		TunnelLogin: func() error {
			tunnelMu.Lock()
			manager := activeTunnel
			tunnelMu.Unlock()
			if manager == nil {
				return errors.New("el túnel no está configurado; define el hostname y reinicia")
			}
			go func() {
				if err := manager.Login(context.Background()); err != nil {
					fmt.Fprintln(os.Stderr, "tunnel login:", err)
				}
			}()
			return nil
		},
		Tunnel: func() panelserver.TunnelInfo {
			tunnelMu.Lock()
			manager := activeTunnel
			tunnelMu.Unlock()
			if manager == nil {
				status := "stopped"
				if cfg.TunnelHostname == "" {
					status = "disabled"
				}
				return panelserver.TunnelInfo{
					Hostname: cfg.TunnelHostname,
					Service:  cfg.TunnelService,
					Status:   status,
					Detail:   labels.Tunnel.Disabled,
				}
			}
			status, detail := manager.Status()
			return panelserver.TunnelInfo{
				Hostname:    manager.Hostname(),
				URL:         manager.PublicURL(),
				Service:     manager.Service(),
				Status:      status.Name(),
				Detail:      detail,
				CertPresent: manager.LoggedIn(),
			}
		},
		OnChange: func() {
			// D27: reconnect the bot session or the tunnel when a secret a live
			// session depends on changes.
			fmt.Fprintln(os.Stderr, "settings: updated, reconnect pending")
		},
	})
	panelMu.Lock()
	activePanel = server
	panelMu.Unlock()
	ctx, cancel := context.WithCancel(context.Background())
	panelMu.Lock()
	panelCancel = cancel
	panelMu.Unlock()
	go func() {
		if err := server.Run(ctx); err != nil {
			fmt.Fprintln(os.Stderr, "panel:", err)
		}
	}()
}

func stopPanel() {
	panelMu.Lock()
	cancel := panelCancel
	panelMu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func toggleAutostart(enabled bool) {
	if err := setAutostart(enabled); err != nil {
		fmt.Fprintln(os.Stderr, "autostart:", err)
	}
}

// setAutostart registers or removes the login item, for the tray and Ajustes.
func setAutostart(enabled bool) error {
	if enabled {
		return autostart.Enable()
	}
	return autostart.Disable()
}

// navigateWhenReady waits for the panel server to answer, then points the
// window at it. The window is hidden on start, so the wait is invisible.
func navigateWhenReady(url string) {
	go func() {
		for range 50 {
			response, err := http.Get(url + "health")
			if err == nil {
				_ = response.Body.Close()
				break
			}
			time.Sleep(100 * time.Millisecond)
		}
		window.Navigate(url)
	}()
}

// findListURL resolves the built single-file kitchen list: an explicit override
// first, then the dev layout walking up from the working directory and the exe.
// It is kept for the tray fallback; the window loads the panel.
func findListURL(override string) string {
	if override != "" {
		return fileURL(override)
	}
	for _, candidate := range listCandidates() {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return fileURL(candidate)
		}
	}
	return missingPageURL()
}

func listCandidates() []string {
	var candidates []string
	if cwd, err := os.Getwd(); err == nil {
		candidates = append(candidates, walkUp(cwd)...)
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(dir, "index.html"))
		candidates = append(candidates, walkUp(dir)...)
	}
	return candidates
}

// readPanel loads the built Svelte panel (one HTML file). Without a build the
// panelserver serves its placeholder page.
func readPanel(cfg config.Config) []byte {
	path := cfg.PanelHTML
	if path == "" {
		for _, candidate := range panelCandidates() {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				path = candidate
				break
			}
		}
	}
	if path == "" {
		// Not in the dev layout (e.g. the exe was copied to another computer):
		// use the panel embedded at build time.
		return bundled("panel.html")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "panel:", err)
		return nil
	}
	return raw
}

// cloudflaredPath prefers an explicit CLOUDFLARED_PATH, then the connector
// embedded in the exe, and finally lets the tunnel package search the system.
func cloudflaredPath(cfg config.Config) string {
	if cfg.CloudflaredPath != "" {
		return cfg.CloudflaredPath
	}
	return bundledCloudflared(cfg.DataDir)
}

// readPanelIcon returns the embedded 512x512 PWA icon.
func readPanelIcon() []byte {
	return panelIconPNG
}

func panelCandidates() []string {
	var candidates []string
	appendFor := func(dir string) {
		for {
			candidates = append(candidates, filepath.Join(dir, filepath.FromSlash(panelRelative)))
			parent := filepath.Dir(dir)
			if parent == dir {
				return
			}
			dir = parent
		}
	}
	if cwd, err := os.Getwd(); err == nil {
		appendFor(cwd)
	}
	if exe, err := os.Executable(); err == nil {
		dir := filepath.Dir(exe)
		candidates = append(candidates, filepath.Join(dir, "index.html"))
		appendFor(dir)
	}
	return candidates
}

func walkUp(dir string) []string {
	var paths []string
	for {
		paths = append(paths, filepath.Join(dir, filepath.FromSlash(listRelative)))
		parent := filepath.Dir(dir)
		if parent == dir {
			return paths
		}
		dir = parent
	}
}

func fileURL(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	return "file:///" + filepath.ToSlash(abs)
}

// missingPageURL is shown when the list has not been built yet.
func missingPageURL() string {
	const page = `<!doctype html>
<html lang="es"><head><meta charset="utf-8"><title>Cocina en Palorosa</title>
<style>body{font-family:system-ui,sans-serif;margin:3rem;color:#6a4b20;background:#fdf8f5}
code{background:#f6d5cd;padding:.15rem .4rem;border-radius:.3rem}</style></head>
<body><h1>Falta compilar la lista</h1>
<p>No se encontró <code>apps/standalone/dist/index.html</code>.</p>
<p>Ejecuta <code>pnpm build:list</code> y vuelve a abrir la aplicación.</p></body></html>`
	return "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(page))
}
