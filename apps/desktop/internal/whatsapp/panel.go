package whatsapp

import (
	"fmt"
	"strings"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/engine"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/labels"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/panelmodel"
)

// Name is the machine readable status for the panel API.
func (s Status) Name() string {
	switch s {
	case StatusConnected:
		return "connected"
	case StatusConnecting:
		return "connecting"
	case StatusDisconnected:
		return "disconnected"
	case StatusDisabled:
		return "disabled"
	default:
		return "unlinked"
	}
}

// PanelSnapshot builds the read model the panel server serves.
func (c *Client) PanelSnapshot() panelmodel.Snapshot {
	status := c.Status()
	linked := c.Linked()

	c.kitchen.mu.Lock()
	defer c.kitchen.mu.Unlock()

	snapshot := panelmodel.Snapshot{
		WhatsApp: panelmodel.WhatsApp{
			Status:    status.Name(),
			Label:     status.Label(),
			Linked:    linked,
			Phone:     c.Phone(),
			Allowlist: append([]string{}, c.cfg.Allowlist...),
		},
		Notifications: notificationsSnapshot(c.kitchen.state.Notifications),
	}

	dates := map[string]bool{}
	for date := range c.kitchen.state.Orders {
		dates[date] = true
	}
	for date := range c.kitchen.state.Lists {
		dates[date] = true
	}
	for _, date := range sortedKeys(dates) {
		snapshot.Days = append(snapshot.Days, c.dayLocked(date))
	}
	for index := len(c.kitchen.state.Events) - 1; index >= 0; index-- {
		event := c.kitchen.state.Events[index]
		snapshot.Activity = append(snapshot.Activity, panelmodel.Activity{
			Time: event.Time,
			Kind: event.Kind,
			Text: event.Text,
		})
		if len(snapshot.Activity) >= 10 {
			break
		}
	}
	return snapshot
}

func (c *Client) dayLocked(date string) panelmodel.Day {
	day := panelmodel.Day{Date: date}
	for _, number := range sortedKeys(c.kitchen.state.Orders[date]) {
		lines := c.kitchen.state.Orders[date][number]
		status := "pending"
		if c.kitchen.state.Notified[date][number] {
			status = "notified"
		}
		day.Orders = append(day.Orders, panelmodel.Order{
			Number: number,
			Text:   orderText(lines),
			Units:  len(c.orderUnits(lines)),
			Status: status,
			Note:   c.kitchen.state.Observations[date][number],
			Facets: orderFacets(lines),
		})
	}
	if list, ok := c.kitchen.state.Lists[date]; ok {
		day.List = listSnapshot(list)
	} else if computed := c.listFromStateLocked(date); computed != nil {
		day.List = listSnapshot(*computed)
	}
	return day
}

func listSnapshot(list engine.KitchenList) panelmodel.List {
	out := panelmodel.List{
		Unresolved:      len(list.Unresolved),
		Entries:         entriesSnapshot(list.Entries),
		UnresolvedItems: make([]panelmodel.Unresolved, 0, len(list.Unresolved)),
	}
	for _, entry := range list.Unresolved {
		out.UnresolvedItems = append(out.UnresolvedItems, panelmodel.Unresolved{
			ProductText: entry.ProductText,
			Quantity:    entry.Quantity,
			Count:       entry.Count,
			Reason:      entry.Reason,
			Detail:      entry.Detail,
			References:  entry.References,
		})
	}
	for _, entry := range list.Entries {
		out.Total += entry.Quantity
	}
	return out
}

func entriesSnapshot(entries []engine.KitchenListEntry) []panelmodel.Entry {
	out := make([]panelmodel.Entry, 0, len(entries))
	for _, entry := range entries {
		out = append(out, panelmodel.Entry{
			UnitID:   entry.UnitID,
			Name:     entry.Name,
			Measure:  entry.Measure,
			Category: entry.Category,
			Note:     entry.Note,
			Quantity: entry.Quantity,
		})
	}
	return out
}

func notificationsSnapshot(settings *notificationSettings) panelmodel.Notifications {
	on := func(value *bool) bool { return value == nil || *value }
	out := panelmodel.Notifications{Enabled: true, New: true, Updated: true, Today: true, Tomorrow: true}
	if settings != nil {
		out.Enabled = on(settings.Enabled)
		out.New = on(settings.New)
		out.Updated = on(settings.Updated)
		out.Today = on(settings.Today)
		out.Tomorrow = on(settings.Tomorrow)
	}
	return out
}

// SetNotifications applies the global notification control from the panel.
func (c *Client) SetNotifications(notifications panelmodel.Notifications) {
	c.setNotification("enabled", notifications.Enabled)
	c.setNotification("new", notifications.New)
	c.setNotification("updated", notifications.Updated)
	c.setNotification("today", notifications.Today)
	c.setNotification("tomorrow", notifications.Tomorrow)
}

// changed notifies the panel that the kitchen state moved.
func (c *Client) changed() {
	if c.cfg.OnOrder != nil {
		c.cfg.OnOrder()
	}
}

// RemoveOrder drops an order from a day, as a cancelled webhook would.
func (c *Client) RemoveOrder(date, number string) error {
	if c.kitchen.Index() == nil {
		return fmt.Errorf("catalog not loaded")
	}
	c.applyRemoval(date, number, labels.Bot.Cancelled)
	return nil
}

// OrderDetail resolves one order for the panel's lazy ticket view: its lines
// with variants and the kitchen units they produce, plus the aggregated units
// of the whole order.
func (c *Client) OrderDetail(date, number string) (panelmodel.OrderDetail, error) {
	c.kitchen.mu.Lock()
	lines, ok := c.kitchen.state.Orders[date][number]
	if !ok {
		c.kitchen.mu.Unlock()
		return panelmodel.OrderDetail{}, fmt.Errorf("pedido no encontrado")
	}
	note := c.kitchen.state.Observations[date][number]
	status := "pending"
	if c.kitchen.state.Notified[date][number] {
		status = "notified"
	}
	index := c.kitchen.Index()
	c.kitchen.mu.Unlock()

	detail := panelmodel.OrderDetail{
		Number:  number,
		Status:  status,
		Note:    note,
		Lines:   make([]panelmodel.OrderLine, 0, len(lines)),
		Entries: []panelmodel.Entry{},
	}
	if index == nil {
		detail.Lines = []panelmodel.OrderLine{}
		return detail, nil
	}
	resolved := engine.ResolveLines(lines, index)
	for _, line := range resolved {
		out := panelmodel.OrderLine{
			ProductText: line.Line.ProductText,
			Quantity:    line.Line.Quantity,
			Options:     append([]string{}, line.Line.Options...),
			Source:      line.Line.Source,
			Status:      line.Status,
			Detail:      line.UnresolvedDetail,
			Units:       []panelmodel.LineUnit{},
		}
		for _, contribution := range line.Contributions {
			unit := index.UnitsByID[contribution.UnitID]
			if unit == nil || !unit.IsKitchen || !unit.Active {
				continue
			}
			out.Units = append(out.Units, panelmodel.LineUnit{
				UnitID:   unit.ID,
				Name:     unit.Name,
				Quantity: contribution.Quantity,
			})
		}
		detail.Lines = append(detail.Lines, out)
	}
	list := engine.AggregateUnits(resolved, index)
	detail.Entries = entriesSnapshot(list.Entries)
	detail.Units = len(detail.Entries)
	return detail, nil
}

// colorWords are the color names the panel groups under its Color filter,
// plus the modifiers and connectors that appear next to them ("rosado con
// dorado", "azul oscuro").
var colorWords = []string{
	"rojo", "roja", "rosado", "rosada", "rosa", "azul", "verde",
	"amarillo", "amarilla", "negro", "negra", "blanco", "blanca",
	"dorado", "dorada", "plateado", "plateada", "gris", "morado",
	"morada", "violeta", "lila", "naranja", "vinotinto", "vino",
	"beige", "cafe", "marron", "turquesa", "celeste", "crema",
	"champan", "champana", "corinto", "fucsia", "salmon", "mostaza",
	"oliva", "terracota", "purpura",
	"claro", "clara", "oscuro", "oscura", "metalico", "metalica",
	"perlado", "perlada", "mate", "brillante", "con", "y", "o",
}

// orderFacets collects the attachable values of an order: its breakfasts, its
// add-ons, its color options and every other option. Duplicates collapse.
func orderFacets(lines []engine.ParsedOrderLine) []panelmodel.Facet {
	facets := []panelmodel.Facet{}
	seen := map[string]bool{}
	add := func(kind, value string) {
		value = strings.TrimSpace(value)
		if value == "" {
			return
		}
		key := kind + "\x00" + strings.ToLower(value)
		if seen[key] {
			return
		}
		seen[key] = true
		facets = append(facets, panelmodel.Facet{Kind: kind, Value: value})
	}
	for _, line := range lines {
		if line.Source == "add_on" {
			add("add_on", line.ProductText)
		} else {
			add("breakfast", line.ProductText)
		}
		for _, option := range line.Options {
			if isColorValue(option) {
				add("color", option)
			} else {
				add("other", option)
			}
		}
	}
	return facets
}

// isColorValue reports whether an option value names a color, so the panel can
// group it under the Color filter. Every word must be a color (or a color
// modifier), so "Jugo de naranja" stays an option and "Dorado/negro" is a
// color.
func isColorValue(text string) bool {
	normalized := engine.NormalizeName(text)
	normalized = strings.TrimPrefix(normalized, "el color ")
	normalized = strings.TrimPrefix(normalized, "color ")
	words := strings.Fields(normalized)
	if len(words) == 0 {
		return false
	}
	for _, word := range words {
		known := false
		for _, color := range colorWords {
			if word == color {
				known = true
				break
			}
		}
		if !known {
			return false
		}
	}
	return true
}
