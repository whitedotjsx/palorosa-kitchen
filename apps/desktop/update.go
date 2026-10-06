//go:build windows

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/settings"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/toast"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/update"
)

// The public release the app follows for self-updates.
const (
	updateRepo  = update.DefaultRepo
	updateAsset = update.DefaultAsset
)

// autoUpdateInterval is how often the running app looks for a new release.
const autoUpdateInterval = 6 * time.Hour

// updater is the process-wide self-update manager, set once config is loaded.
var updater *update.Manager

// restartAfterUpdate adapts restartApp to the updater's error-returning
// callback.
func restartAfterUpdate() error {
	restartApp()
	return nil
}

// newUpdater builds the self-update manager. A nil restart applies an update
// without relaunching, which is what the console modes use.
func newUpdater(restart func() error) *update.Manager {
	return update.New(update.Config{
		Repo:    updateRepo,
		Asset:   updateAsset,
		Current: version,
		Restart: restart,
	})
}

// runCLI handles the console-only modes: --version prints the build version,
// --update-check asks GitHub Releases for the latest release, and
// --update-now also downloads and installs it (without relaunching). Both
// print one JSON line. It returns the exit code and whether the process ran a
// console mode.
func runCLI(args []string) (int, bool) {
	switch {
	case hasFlag(args, "--version"):
		fmt.Println(version)
		return 0, true
	case hasFlag(args, "--update-check"):
		manager := newUpdater(nil)
		release, newer, err := manager.Check(context.Background())
		printUpdateResult(release, newer, err)
		return exitCode(err), true
	case hasFlag(args, "--update-now"):
		manager := newUpdater(nil)
		release, updated, err := manager.Update(context.Background())
		printUpdateResult(release, updated, err)
		return exitCode(err), true
	}
	return 0, false
}

// hasFlag reports whether args contains flag.
func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

// exitCode maps an error to the process exit code.
func exitCode(err error) int {
	if err != nil {
		return 1
	}
	return 0
}

// printUpdateResult emits one JSON line so scripts can read the outcome.
func printUpdateResult(release update.Release, changed bool, err error) {
	out := map[string]any{"current": version, "updated": changed}
	if release.Version != "" {
		out["latest"] = release.Version
	}
	if err != nil {
		out["error"] = err.Error()
	}
	raw, _ := json.Marshal(out)
	fmt.Println(string(raw))
}

// currentExecutable returns the running exe path, or "" when unavailable.
func currentExecutable() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	return exe
}

// autoUpdateState tracks the running automatic loop so the Ajustes switch
// applies without restarting the app.
var autoUpdate struct {
	mu      sync.Mutex
	applied *bool
	cancel  context.CancelFunc
}

// autoUpdateEnabled resolves the automatic update switch: settings.json wins
// over KITCHEN_AUTO_UPDATE=off, and on is the default.
func autoUpdateEnabled(store *settings.Store) bool {
	if store != nil {
		if value := store.Values().AutoUpdate; value != nil {
			return *value
		}
	}
	return !strings.EqualFold(strings.TrimSpace(os.Getenv("KITCHEN_AUTO_UPDATE")), "off")
}

// startAutoUpdate checks at launch and then every interval, restarting the app
// when a newer release is installed. When the setting is off it keeps only the
// manual check from the tray or the panel.
func startAutoUpdate(store *settings.Store) {
	syncAutoUpdate(autoUpdateEnabled(store))
}

// syncAutoUpdate starts or stops the automatic loop to match `enabled`. A
// call with the value already applied does nothing, so saving unrelated
// settings does not reset the six-hour timer.
func syncAutoUpdate(enabled bool) {
	autoUpdate.mu.Lock()
	defer autoUpdate.mu.Unlock()
	if autoUpdate.applied != nil && *autoUpdate.applied == enabled {
		return
	}
	value := enabled
	autoUpdate.applied = &value
	if autoUpdate.cancel != nil {
		autoUpdate.cancel()
		autoUpdate.cancel = nil
	}
	if !enabled || updater == nil {
		fmt.Fprintln(os.Stderr, "update: automático apagado")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	autoUpdate.cancel = cancel
	go updater.Run(ctx, autoUpdateInterval)
}

// checkUpdatesManually backs the tray item and reports the outcome as a toast.
func checkUpdatesManually() {
	if updater == nil {
		return
	}
	go func() {
		release, updated, err := updater.Update(context.Background())
		switch {
		case err != nil:
			toast.Show("Actualización", "No se pudo actualizar: "+err.Error())
		case updated:
			// Update already relaunched the app.
		case release.Version != "" && release.Version != version:
			toast.Show("Actualización", "Hay una versión "+release.Version+"; se instalará al reiniciar")
		default:
			toast.Show("Actualización", "Ya tienes la última versión ("+version+")")
		}
	}()
}
