package whatsapp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/engine"
)

// maxEvents caps the persisted activity log so the state file stays small.
const maxEvents = 40

// botState mirrors the Node bot state: parsed orders per delivery date and the
// last computed list per date (for the notification delta).
type botState struct {
	Orders map[string]map[string][]engine.ParsedOrderLine `json:"orders"`
	Lists  map[string]engine.KitchenList                  `json:"lists"`
	// Notifications is nil until the operator toggles something; nil means on.
	Notifications *notificationSettings `json:"notifications,omitempty"`
	// Observations holds the order observation per delivery date and order.
	Observations map[string]map[string]string `json:"observations,omitempty"`
	// Checkpoint is the snapshot saved by the LISTO command; NUEVO diffs it.
	Checkpoint *checkpointState `json:"checkpoint,omitempty"`
	// Notified marks the orders an immediate notice already went out for.
	Notified map[string]map[string]bool `json:"notified,omitempty"`
	// Events is the recent activity feed, oldest first.
	Events []activityEvent `json:"events,omitempty"`
}

// activityEvent is one entry in the panel's recent-activity feed.
type activityEvent struct {
	Time time.Time `json:"time"`
	Kind string    `json:"kind"` // new | update | removed
	Text string    `json:"text"`
}

// addEventLocked appends an activity event, trimming the oldest ones. The
// caller holds kitchen.mu.
func addEventLocked(state *botState, kind, text string) {
	state.Events = append(state.Events, activityEvent{Time: time.Now(), Kind: kind, Text: text})
	if len(state.Events) > maxEvents {
		state.Events = state.Events[len(state.Events)-maxEvents:]
	}
}

// checkpointState is a frozen copy of the orders and lists at LISTO time.
type checkpointState struct {
	Orders map[string]map[string][]engine.ParsedOrderLine `json:"orders"`
	Lists  map[string]engine.KitchenList                  `json:"lists"`
}

func cloneKitchenList(list engine.KitchenList) engine.KitchenList {
	return engine.KitchenList{
		Entries:         append([]engine.KitchenListEntry{}, list.Entries...),
		Unresolved:      append([]engine.UnresolvedEntry{}, list.Unresolved...),
		IgnoredProducts: append([]engine.IgnoredEntry{}, list.IgnoredProducts...),
		IgnoredUnits:    append([]engine.IgnoredEntry{}, list.IgnoredUnits...),
		Warnings:        append([]engine.WarningEntry{}, list.Warnings...),
	}
}

func cloneOrders(orders map[string]map[string][]engine.ParsedOrderLine) map[string]map[string][]engine.ParsedOrderLine {
	out := make(map[string]map[string][]engine.ParsedOrderLine, len(orders))
	for date, byNumber := range orders {
		numbers := make(map[string][]engine.ParsedOrderLine, len(byNumber))
		for number, lines := range byNumber {
			numbers[number] = append([]engine.ParsedOrderLine{}, lines...)
		}
		out[date] = numbers
	}
	return out
}

func cloneLists(lists map[string]engine.KitchenList) map[string]engine.KitchenList {
	out := make(map[string]engine.KitchenList, len(lists))
	for date, list := range lists {
		out[date] = cloneKitchenList(list)
	}
	return out
}

// notificationSettings is the granular notification control. A nil pointer
// means the default, which is on.
type notificationSettings struct {
	Enabled  *bool `json:"enabled,omitempty"`
	New      *bool `json:"new,omitempty"`
	Updated  *bool `json:"updated,omitempty"`
	Today    *bool `json:"today,omitempty"`
	Tomorrow *bool `json:"tomorrow,omitempty"`
}

// UnmarshalJSON accepts both the old boolean form and the object form.
func (n *notificationSettings) UnmarshalJSON(data []byte) error {
	var enabled bool
	if err := json.Unmarshal(data, &enabled); err == nil {
		n.Enabled = &enabled
		return nil
	}
	type alias notificationSettings
	return json.Unmarshal(data, (*alias)(n))
}

func newBotState() *botState {
	return &botState{
		Orders:       map[string]map[string][]engine.ParsedOrderLine{},
		Lists:        map[string]engine.KitchenList{},
		Observations: map[string]map[string]string{},
		Notified:     map[string]map[string]bool{},
	}
}

func loadBotState(path string) *botState {
	raw, err := os.ReadFile(path)
	if err != nil {
		return newBotState()
	}
	state := newBotState()
	if err := json.Unmarshal(raw, state); err != nil {
		return newBotState()
	}
	if state.Orders == nil {
		state.Orders = map[string]map[string][]engine.ParsedOrderLine{}
	}
	if state.Lists == nil {
		state.Lists = map[string]engine.KitchenList{}
	}
	if state.Observations == nil {
		state.Observations = map[string]map[string]string{}
	}
	if state.Notified == nil {
		state.Notified = map[string]map[string]bool{}
	}
	return state
}

func saveBotState(path string, state *botState) error {
	raw, err := json.MarshalIndent(state, "", "\t")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, raw, 0o644); err != nil {
		return err
	}
	return os.Rename(temporary, path)
}
