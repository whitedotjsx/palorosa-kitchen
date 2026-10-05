// Package notify dispatches order notifications to targets. A target is a
// phone with the events it wants and a schedule (immediate, or fixed times with
// quiet hours). Scheduled notices go to a persisted queue that a minute ticker
// flushes, so a restart does not lose a digest (D26).
package notify

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Event kinds a target can subscribe to.
const (
	KindNew      = "new"
	KindUpdate   = "update"
	KindToday    = "today"
	KindTomorrow = "tomorrow"
)

// Sender delivers one text to one phone through a bot account.
type Sender func(botID, phone, text string) error

// Schedule is when a target receives its notices.
type Schedule struct {
	Mode       string   `json:"mode"`                 // "immediate" | "times"
	Times      []string `json:"times,omitempty"`      // "HH:MM" in Timezone
	Timezone   string   `json:"timezone,omitempty"`   // default America/Bogota
	QuietHours []string `json:"quietHours,omitempty"` // ["21:00","06:00"]
}

// Target is a notification destination.
type Target struct {
	ID       string   `json:"id"`
	Label    string   `json:"label"`
	Phone    string   `json:"phone"`
	BotID    string   `json:"botId,omitempty"`
	Kinds    []string `json:"kinds"`
	Schedule Schedule `json:"schedule"`
	Enabled  bool     `json:"enabled"`
}

// Pending is a queued notice waiting for a target's send time.
type Pending struct {
	TargetID  string    `json:"targetId"`
	Kind      string    `json:"kind"`
	Date      string    `json:"date"`
	Text      string    `json:"text"`
	CreatedAt time.Time `json:"createdAt"`
}

type state struct {
	Targets []Target  `json:"targets"`
	Pending []Pending `json:"pending"`
}

// Manager owns the targets and the queue.
type Manager struct {
	path   string
	now    func() time.Time
	sender Sender
	log    *log.Logger

	mu    sync.Mutex
	state state
}

// New loads targets.json. A missing file starts empty.
func New(path string, sender Sender, logger *log.Logger) (*Manager, error) {
	manager := &Manager{path: path, now: time.Now, sender: sender, log: logger}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return manager, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(raw, &manager.state); err != nil {
		return nil, fmt.Errorf("notify: %w", err)
	}
	return manager, nil
}

// Targets returns the configured targets.
func (m *Manager) Targets() []Target {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Target{}, m.state.Targets...)
}

// Pending returns the queued notices.
func (m *Manager) Pending() []Pending {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]Pending{}, m.state.Pending...)
}

// AddTarget stores a new target and assigns an id.
func (m *Manager) AddTarget(target Target) (Target, error) {
	if target.ID == "" {
		target.ID = newID()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.state.Targets = append(m.state.Targets, target)
	if err := m.saveLocked(); err != nil {
		return Target{}, err
	}
	return target, nil
}

// UpdateTarget replaces a target by id.
func (m *Manager) UpdateTarget(target Target) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for index := range m.state.Targets {
		if m.state.Targets[index].ID == target.ID {
			m.state.Targets[index] = target
			return m.saveLocked()
		}
	}
	return fmt.Errorf("notify: target %q not found", target.ID)
}

// RemoveTarget drops a target and its queued notices.
func (m *Manager) RemoveTarget(id string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	found := false
	targets := m.state.Targets[:0]
	for _, target := range m.state.Targets {
		if target.ID == id {
			found = true
			continue
		}
		targets = append(targets, target)
	}
	m.state.Targets = targets
	m.state.Pending = filterPending(m.state.Pending, id)
	if !found {
		return fmt.Errorf("notify: target %q not found", id)
	}
	return m.saveLocked()
}

// NextSend is the earliest scheduled send time across enabled targets with a
// "times" schedule, and how many such targets there are. ok is false when no
// target is scheduled, so the panel can hide the indicator.
func (m *Manager) NextSend(now time.Time) (time.Time, int, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var next time.Time
	count := 0
	for _, target := range m.state.Targets {
		if !target.Enabled || target.Schedule.Mode != "times" || len(scheduleTimes(target.Schedule.Times)) == 0 {
			continue
		}
		count++
		candidate := nextSendAt(target, now)
		if next.IsZero() || candidate.Before(next) {
			next = candidate
		}
	}
	return next, count, count > 0
}

// TestAll sends a test message to every enabled target now. It returns how many
// were sent.
func (m *Manager) TestAll() int {
	m.mu.Lock()
	targets := append([]Target{}, m.state.Targets...)
	sender := m.sender
	m.mu.Unlock()
	if sender == nil {
		return 0
	}
	sent := 0
	for _, target := range targets {
		if !target.Enabled {
			continue
		}
		if err := sender(target.BotID, target.Phone, testText); err != nil {
			m.logf("test to %s failed: %v", target.Phone, err)
			continue
		}
		sent++
	}
	return sent
}

// testText is the body of a test notification.
const testText = "Prueba de avisos de la cocina. Si ves este mensaje, el destino funciona."

// Test sends a test message to one target now.
func (m *Manager) Test(id string) error {
	m.mu.Lock()
	var target *Target
	for index := range m.state.Targets {
		if m.state.Targets[index].ID == id {
			target = &m.state.Targets[index]
		}
	}
	sender := m.sender
	m.mu.Unlock()
	if target == nil {
		return fmt.Errorf("notify: target %q not found", id)
	}
	if sender == nil {
		return fmt.Errorf("notify: no sender")
	}
	return sender(target.BotID, target.Phone, testText)
}

// Dispatch routes one notice: immediate targets get it now, scheduled targets
// are queued for their next send time. It returns how many were sent now.
func (m *Manager) Dispatch(text, kind, date string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := m.now()
	sent := 0
	for index := range m.state.Targets {
		target := m.state.Targets[index]
		if !target.Enabled || !wants(target, kind) {
			continue
		}
		if target.Schedule.Mode == "times" {
			m.state.Pending = append(m.state.Pending, Pending{
				TargetID: target.ID, Kind: kind, Date: date, Text: text, CreatedAt: now,
			})
			continue
		}
		if m.sender != nil {
			if err := m.sender(target.BotID, target.Phone, text); err != nil {
				m.logf("send to %s failed: %v", target.Phone, err)
				continue
			}
		}
		sent++
	}
	_ = m.saveLocked()
	return sent
}

// Tick flushes every queued notice whose send time has arrived, one aggregated
// message per target. It returns how many messages it sent.
func (m *Manager) Tick(now time.Time) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.state.Pending) == 0 {
		return 0
	}

	dueByTarget := map[string][]int{}
	for index := range m.state.Pending {
		target, ok := m.targetLocked(m.state.Pending[index].TargetID)
		if !ok {
			continue
		}
		if !now.Before(nextSendAt(target, m.state.Pending[index].CreatedAt)) {
			dueByTarget[target.ID] = append(dueByTarget[target.ID], index)
		}
	}
	if len(dueByTarget) == 0 {
		return 0
	}

	sent := 0
	dropped := map[int]bool{}
	for targetID, indices := range dueByTarget {
		target, _ := m.targetLocked(targetID)
		texts := make([]string, 0, len(indices))
		for _, index := range indices {
			texts = append(texts, m.state.Pending[index].Text)
		}
		if m.sender != nil {
			if err := m.sender(target.BotID, target.Phone, strings.Join(texts, "\n\n")); err != nil {
				m.logf("scheduled send to %s failed: %v", target.Phone, err)
				continue
			}
		}
		for _, index := range indices {
			dropped[index] = true
		}
		sent++
	}
	if sent > 0 {
		pending := m.state.Pending[:0]
		for index, item := range m.state.Pending {
			if !dropped[index] {
				pending = append(pending, item)
			}
		}
		m.state.Pending = pending
		_ = m.saveLocked()
	}
	return sent
}

// Run flushes the queue every minute until ctx is cancelled.
func (m *Manager) Run(ctx context.Context) {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case now := <-ticker.C:
			m.Tick(now)
		}
	}
}

func (m *Manager) targetLocked(id string) (Target, bool) {
	for _, target := range m.state.Targets {
		if target.ID == id {
			return target, true
		}
	}
	return Target{}, false
}

func (m *Manager) logf(format string, args ...any) {
	if m.log != nil {
		m.log.Printf(format, args...)
	}
}

func (m *Manager) saveLocked() error {
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(m.state, "", "\t")
	if err != nil {
		return err
	}
	temporary := m.path + ".tmp"
	if err := os.WriteFile(temporary, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, m.path)
}

func filterPending(pending []Pending, targetID string) []Pending {
	out := pending[:0]
	for _, item := range pending {
		if item.TargetID != targetID {
			out = append(out, item)
		}
	}
	return out
}

func wants(target Target, kind string) bool {
	if kind == KindNew || kind == KindUpdate {
		for _, k := range target.Kinds {
			if k == kind {
				return true
			}
		}
		return false
	}
	// "today"/"tomorrow" are date windows handled by the caller; a target that
	// wants new or updated notices also wants those dates.
	return len(target.Kinds) > 0
}

// nextSendAt is the first configured time at or after "after" that is outside
// the quiet hours.
func nextSendAt(target Target, after time.Time) time.Time {
	times := scheduleTimes(target.Schedule.Times)
	if target.Schedule.Mode != "times" || len(times) == 0 {
		return after
	}
	location := location(target.Schedule.Timezone)
	local := after.In(location)
	for _, slot := range times {
		candidate := atTime(local, slot, location, 0)
		if !candidate.Before(local) {
			return applyQuiet(candidate, target.Schedule, location)
		}
	}
	return applyQuiet(atTime(local, times[0], location, 1), target.Schedule, location)
}

func applyQuiet(candidate time.Time, schedule Schedule, location *time.Location) time.Time {
	if len(schedule.QuietHours) != 2 {
		return candidate
	}
	start, okStart := parseClock(schedule.QuietHours[0])
	end, okEnd := parseClock(schedule.QuietHours[1])
	if !okStart || !okEnd {
		return candidate
	}
	minutes := candidate.In(location).Hour()*60 + candidate.In(location).Minute()
	switch {
	case start < end:
		if minutes >= start && minutes < end {
			return atClock(candidate, end, location, 0)
		}
	case start > end:
		if minutes >= start {
			return atClock(candidate, end, location, 1)
		}
		if minutes < end {
			return atClock(candidate, end, location, 0)
		}
	}
	return candidate
}

func atTime(local time.Time, slot string, location *time.Location, dayOffset int) time.Time {
	minutes, ok := parseClock(slot)
	if !ok {
		return local
	}
	return atClock(local, minutes, location, dayOffset)
}

func atClock(reference time.Time, minutes int, location *time.Location, dayOffset int) time.Time {
	return time.Date(
		reference.In(location).Year(), reference.In(location).Month(), reference.In(location).Day()+dayOffset,
		minutes/60, minutes%60, 0, 0, location,
	)
}

func parseClock(value string) (int, bool) {
	parts := strings.Split(strings.TrimSpace(value), ":")
	if len(parts) != 2 {
		return 0, false
	}
	var hour, minute int
	if _, err := fmt.Sscanf(parts[0], "%d", &hour); err != nil {
		return 0, false
	}
	if _, err := fmt.Sscanf(parts[1], "%d", &minute); err != nil {
		return 0, false
	}
	if hour < 0 || hour > 23 || minute < 0 || minute > 59 {
		return 0, false
	}
	return hour*60 + minute, true
}

func scheduleTimes(times []string) []string {
	valid := make([]string, 0, len(times))
	for _, slot := range times {
		if _, ok := parseClock(slot); ok {
			valid = append(valid, slot)
		}
	}
	sort.Slice(valid, func(i, j int) bool {
		left, _ := parseClock(valid[i])
		right, _ := parseClock(valid[j])
		return left < right
	})
	return valid
}

func location(name string) *time.Location {
	if name == "" {
		name = "America/Bogota"
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return time.FixedZone("America/Bogota", -5*60*60)
	}
	return loc
}

func newID() string {
	raw := make([]byte, 9)
	if _, err := rand.Read(raw); err != nil {
		return hex.EncodeToString([]byte(time.Now().String()))
	}
	return hex.EncodeToString(raw)
}
