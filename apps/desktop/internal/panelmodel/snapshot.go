// Package panelmodel is the read model the panel server exposes. The WhatsApp
// bot builds it and the HTTP API serves it, so neither package imports the
// other.
package panelmodel

import "time"

// Snapshot is the current kitchen state the panel renders.
type Snapshot struct {
	WhatsApp      WhatsApp      `json:"whatsapp"`
	Notifications Notifications `json:"notifications"`
	Days          []Day         `json:"days"`
	Activity      []Activity    `json:"activity"`
}

// Activity is one recent kitchen event shown on the summary screen.
type Activity struct {
	Time time.Time `json:"time"`
	Kind string    `json:"kind"`
	Text string    `json:"text"`
}

// WhatsApp is the bot connection state.
type WhatsApp struct {
	Status    string   `json:"status"`
	Label     string   `json:"label"`
	Linked    bool     `json:"linked"`
	Phone     string   `json:"phone"`
	Allowlist []string `json:"allowlist"`
}

// Notifications is the global notification control.
type Notifications struct {
	Enabled  bool `json:"enabled"`
	New      bool `json:"new"`
	Updated  bool `json:"updated"`
	Today    bool `json:"today"`
	Tomorrow bool `json:"tomorrow"`
}

// Day is one delivery date: its orders and the aggregated kitchen list.
type Day struct {
	Date   string  `json:"date"`
	Orders []Order `json:"orders"`
	List   List    `json:"list"`
}

// Order is one order in a day.
type Order struct {
	Number string `json:"number"`
	Text   string `json:"text"`
	Units  int    `json:"units"`
	// Status is "notified" once an immediate notice was sent for the order, or
	// "pending" while it has not.
	Status string `json:"status"`
	// Note is the customer's observation on the order, empty when none.
	Note string `json:"note,omitempty"`
	// Facets are the order's variant values (breakfast, add-ons, colors and
	// other options) the panel uses for its attach filters.
	Facets []Facet `json:"facets,omitempty"`
}

// Facet is one attachable value of an order. Kind is "breakfast", "add_on",
// "color" or "other".
type Facet struct {
	Kind  string `json:"kind"`
	Value string `json:"value"`
}

// OrderDetail is the lazy payload for one order: its lines with variants and
// the kitchen units each line produces, plus the aggregated units of the whole
// order. The panel loads it only when a ticket is expanded, so the list stays
// cheap with hundreds of orders.
type OrderDetail struct {
	Number  string      `json:"number"`
	Status  string      `json:"status"`
	Note    string      `json:"note,omitempty"`
	Units   int         `json:"units"`
	Lines   []OrderLine `json:"lines"`
	Entries []Entry     `json:"entries"`
}

// OrderLine is one product line of an order.
type OrderLine struct {
	ProductText string     `json:"productText"`
	Quantity    int        `json:"quantity"`
	Options     []string   `json:"options"`
	// Source is "breakfast" or "add_on".
	Source string `json:"source"`
	// Status is "resolved", "unresolved" or "ignored".
	Status string `json:"status"`
	// Detail carries the unresolved reason detail (missing choice, candidates).
	Detail string     `json:"detail,omitempty"`
	Units  []LineUnit `json:"units"`
}

// LineUnit is one kitchen unit a single order line contributes.
type LineUnit struct {
	UnitID   string `json:"unitId"`
	Name     string `json:"name"`
	Quantity int    `json:"quantity"`
}

// List is the aggregated kitchen list for a day.
type List struct {
	Entries    []Entry `json:"entries"`
	Unresolved int     `json:"unresolved"`
	Total      int     `json:"total"`
	// UnresolvedItems lists the order lines the catalog could not resolve,
	// so the printed sheet can show them for manual review.
	UnresolvedItems []Unresolved `json:"unresolvedItems"`
}

// Unresolved is an order line the catalog could not map to a unit.
type Unresolved struct {
	ProductText string   `json:"productText"`
	Quantity    int      `json:"quantity"`
	Count       int      `json:"count"`
	Reason      string   `json:"reason"`
	Detail      string   `json:"detail,omitempty"`
	References  []string `json:"references,omitempty"`
}

// Entry is one prepared unit in the list.
type Entry struct {
	UnitID   string `json:"unitId"`
	Name     string `json:"name"`
	Measure  string `json:"measure"`
	Category string `json:"category"`
	Note     string `json:"note"`
	Quantity int    `json:"quantity"`
}

// Day finds a day by date, or returns nil.
func (s Snapshot) Day(date string) *Day {
	for index := range s.Days {
		if s.Days[index].Date == date {
			return &s.Days[index]
		}
	}
	return nil
}
