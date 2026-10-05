package whatsapp

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/engine"
)

// ExportRows returns the WP All Export sheet (header first) for one delivery
// date (YYYY-MM-DD).
type ExportRows func(ctx context.Context, date string) ([][]string, error)

// SyncResult summarises a store sync for the panel.
type SyncResult struct {
	// Kept is the number of orders across the synced dates.
	Kept    int            `json:"kept"`
	Added   int            `json:"added"`
	Removed int            `json:"removed"`
	Dates   []string       `json:"dates"`
	PerDate map[string]int `json:"perDate"`
	At      string         `json:"at"`
}

// SyncFromExport runs the store's WP All Export for each delivery date and
// makes that day's orders match the export exactly: orders in the sheet are
// stored or replaced, orders missing from it are dropped. The export filters
// by delivery date, so it never misses an order however early it was booked.
// It never sends WhatsApp notices; it repairs missed or failed webhooks.
// Dates are synced one after another (the export is a single saved config).
func (c *Client) SyncFromExport(ctx context.Context, fetch ExportRows, dates []string) (SyncResult, error) {
	result := SyncResult{Dates: []string{}, PerDate: map[string]int{}}
	if c.kitchen.Index() == nil {
		return result, errors.New("el catálogo no está cargado")
	}
	var failures []string
	for _, date := range dates {
		orders, notes, annotations, err := exportOrders(ctx, fetch, date)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", date, err))
			continue
		}
		added, removed := c.replaceDay(date, orders, notes, annotations)
		result.Added += added
		result.Removed += removed
		result.Kept += len(orders)
		result.PerDate[date] = len(orders)
		result.Dates = append(result.Dates, date)
	}
	sort.Strings(result.Dates)
	result.At = time.Now().Format(time.RFC3339)
	if len(result.Dates) > 0 {
		c.changed()
	}
	if len(failures) > 0 {
		return result, errors.New(strings.Join(failures, "; "))
	}
	return result, nil
}

// exportOrders runs the export for one date and parses it into order lines.
// A sheet whose rows belong to another date means the filter did not apply,
// and is rejected rather than wiping the day.
func exportOrders(ctx context.Context, fetch ExportRows, date string) (map[string][]engine.ParsedOrderLine, map[string]string, map[string]engine.OrderAnnotation, error) {
	rows, err := fetch(ctx, date)
	if err != nil {
		return nil, nil, nil, err
	}
	orders := map[string][]engine.ParsedOrderLine{}
	notes := map[string]string{}
	annotations := map[string]engine.OrderAnnotation{}
	if len(rows) == 0 || (len(rows) == 1 && isBlankRow(rows[0])) {
		return orders, notes, annotations, nil
	}
	if !engine.IsWideOrderHeader(rows[0]) {
		return nil, nil, nil, errors.New("el export no tiene las columnas esperadas («ID orden», «Productos»)")
	}
	for _, record := range engine.TableToRecords(rows) {
		number := strings.TrimSpace(fmt.Sprint(record["ID orden"]))
		if number == "" {
			continue
		}
		parsed := engine.ParseWideOrderRows([]map[string]any{record}, engine.WideExportOptions{})
		if parsed.DeliveryDate != "" && parsed.DeliveryDate != date {
			return nil, nil, nil, fmt.Errorf("el export devolvió pedidos del %s; el filtro de fecha no se aplicó", parsed.DeliveryDate)
		}
		orders[number] = append(orders[number], parsed.Lines...)
		if annotation, ok := parsed.Annotations[number]; ok {
			annotations[number] = annotation
		}
		if note := strings.TrimSpace(fmt.Sprint(record["Observaciones"])); note != "" && note != "<nil>" {
			notes[number] = note
		}
	}
	return orders, notes, annotations, nil
}

func isBlankRow(row []string) bool {
	for _, cell := range row {
		if strings.TrimSpace(cell) != "" {
			return false
		}
	}
	return true
}

// replaceDay swaps a day's orders for the exported ones and rebuilds its list.
func (c *Client) replaceDay(date string, orders map[string][]engine.ParsedOrderLine, notes map[string]string, annotations map[string]engine.OrderAnnotation) (added, removed int) {
	c.kitchen.mu.Lock()
	defer c.kitchen.mu.Unlock()
	state := c.kitchen.state
	previous := state.Orders[date]
	for number := range orders {
		if _, ok := previous[number]; !ok {
			added++
		}
	}
	for number := range previous {
		if _, ok := orders[number]; !ok {
			removed++
		}
	}
	if len(orders) == 0 {
		delete(state.Orders, date)
		delete(state.Observations, date)
		delete(state.Annotations, date)
	} else {
		state.Orders[date] = orders
		if state.Observations == nil {
			state.Observations = map[string]map[string]string{}
		}
		state.Observations[date] = notes
		if state.Annotations == nil {
			state.Annotations = map[string]map[string]engine.OrderAnnotation{}
		}
		state.Annotations[date] = annotations
	}
	if next := c.listFromStateLocked(date); next != nil {
		state.Lists[date] = *next
	} else {
		delete(state.Lists, date)
	}
	if added > 0 || removed > 0 {
		addEventLocked(state, "update", fmt.Sprintf("Sincronizado con la tienda (%s): %d pedidos, %d nuevos, %d quitados", date, len(orders), added, removed))
	}
	_ = saveBotState(c.kitchen.path, state)
	return added, removed
}
