package whatsapp

import (
	"path/filepath"
	"sync"
	"sync/atomic"

	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/engine"
)

// Kitchen is the shared kitchen state: orders, lists, observations, the
// checkpoint and the catalog index. The bot manager owns one and every account
// shares it, so orders received by one bot are seen by all (D25).
type Kitchen struct {
	mu    sync.Mutex
	state *botState
	path  string
	index atomic.Pointer[engine.Index]
}

// NewKitchen loads the shared state from dataDir and indexes the catalog.
func NewKitchen(dataDir, catalogPath string, log waLog.Logger) *Kitchen {
	statePath := filepath.Join(dataDir, "bot-state.json")
	kitchen := &Kitchen{state: loadBotState(statePath), path: statePath}
	if catalogPath != "" {
		if catalog, err := engine.LoadCatalog(catalogPath); err != nil {
			if log != nil {
				log.Warnf("Could not load catalog %s: %v", catalogPath, err)
			}
		} else {
			kitchen.index.Store(engine.IndexCatalog(catalog))
		}
	}
	return kitchen
}

// Index returns the current catalog index, or nil when no catalog is loaded.
func (k *Kitchen) Index() *engine.Index {
	return k.index.Load()
}

// SetCatalog swaps the catalog index in place, so a catalog saved from the
// panel applies to the next order without restarting the bots.
func (k *Kitchen) SetCatalog(catalog *engine.Catalog) {
	if catalog == nil {
		k.index.Store(nil)
		return
	}
	k.index.Store(engine.IndexCatalog(catalog))
}
