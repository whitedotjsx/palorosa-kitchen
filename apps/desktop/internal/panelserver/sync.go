package panelserver

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// syncCooldown is the minimum gap between two syncs of the same dates. A sync
// asked for sooner returns the last result, so several open panels cannot
// hammer the store with exports.
const syncCooldown = 20 * time.Second

// DefaultSyncMinutes is the automatic sync interval when none is configured.
const DefaultSyncMinutes = 10

// orderSyncer serialises store syncs from the button and the background loop.
type orderSyncer struct {
	run func(context.Context, []string) (any, error)

	mu        sync.Mutex
	running   bool
	last      time.Time
	lastDates string
	result    any
	err       string
}

// syncDates are the delivery days a sync covers: today and tomorrow in
// Bogotá, plus the day the operator is looking at.
func syncDates(extra string) []string {
	loc := time.FixedZone("America/Bogota", -5*60*60)
	now := time.Now().In(loc)
	dates := []string{now.Format("2006-01-02"), now.AddDate(0, 0, 1).Format("2006-01-02")}
	if extra != "" && extra != dates[0] && extra != dates[1] {
		if _, err := time.Parse("2006-01-02", extra); err == nil {
			dates = append(dates, extra)
		}
	}
	return dates
}

type syncStatus struct {
	OK       bool   `json:"ok"`
	Enabled  bool   `json:"enabled"`
	Running  bool   `json:"running"`
	Interval int    `json:"intervalMinutes"`
	At       string `json:"at,omitempty"`
	Result   any    `json:"result,omitempty"`
	Error    string `json:"error,omitempty"`
}

// sync runs one store sync unless one ran within the cooldown or is running.
// It reports whether a new sync actually ran.
func (o *orderSyncer) sync(ctx context.Context, dates []string) bool {
	key := strings.Join(dates, ",")
	o.mu.Lock()
	if o.running || (!o.last.IsZero() && key == o.lastDates && time.Since(o.last) < syncCooldown) {
		o.mu.Unlock()
		return false
	}
	o.running = true
	o.mu.Unlock()

	result, err := o.run(ctx, dates)

	o.mu.Lock()
	defer o.mu.Unlock()
	o.running = false
	o.last = time.Now()
	o.lastDates = key
	if result != nil {
		o.result = result
	}
	if err != nil {
		o.err = err.Error()
		return true
	}
	o.err = ""
	o.result = result
	return true
}

func (o *orderSyncer) status(interval int) syncStatus {
	o.mu.Lock()
	defer o.mu.Unlock()
	out := syncStatus{OK: true, Enabled: true, Running: o.running, Interval: interval, Result: o.result, Error: o.err}
	if !o.last.IsZero() {
		out.At = o.last.Format(time.RFC3339)
	}
	return out
}

// syncInterval is the configured automatic interval in minutes (0 = off).
func (s *Server) syncInterval() int {
	if s.cfg.SyncInterval == nil {
		return DefaultSyncMinutes
	}
	return s.cfg.SyncInterval()
}

// syncLoop syncs right after start and then every configured interval. The
// interval is re-read every minute so a change in Ajustes applies live.
func (s *Server) syncLoop(ctx context.Context) {
	if s.syncer == nil {
		return
	}
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
		}
		interval := s.syncInterval()
		s.syncer.mu.Lock()
		last := s.syncer.last
		s.syncer.mu.Unlock()
		if interval > 0 && (last.IsZero() || time.Since(last) >= time.Duration(interval)*time.Minute) {
			if s.syncer.sync(ctx, syncDates("")) {
				s.Publish("orders")
				s.Publish("sync")
			}
		}
		timer.Reset(time.Minute)
	}
}

// handleSyncGet reports the last store sync.
func (s *Server) handleSyncGet(w http.ResponseWriter, _ *http.Request) {
	if s.syncer == nil {
		writeJSON(w, http.StatusOK, syncStatus{OK: true, Enabled: false})
		return
	}
	writeJSON(w, http.StatusOK, s.syncer.status(s.syncInterval()))
}

// handleSyncRun pulls the orders from the store now (any signed-in viewer can
// refresh; the cooldown protects the store). An optional {"date"} adds the day
// the panel is showing to today and tomorrow.
func (s *Server) handleSyncRun(w http.ResponseWriter, r *http.Request) {
	var payload struct {
		Date string `json:"date"`
	}
	_ = json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&payload)
	if s.syncer == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "La sincronización con la tienda no está disponible (bot desactivado)"})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	if s.syncer.sync(ctx, syncDates(strings.TrimSpace(payload.Date))) {
		s.Publish("orders")
		s.Publish("sync")
	}
	status := s.syncer.status(s.syncInterval())
	code := http.StatusOK
	if status.Error != "" {
		status.OK = false
		code = http.StatusBadGateway
	}
	writeJSON(w, code, status)
}
