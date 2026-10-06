package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/engine"
)

// ExportRows returns the WP All Export sheet (header first) for one delivery
// date (YYYY-MM-DD).
type ExportRows func(ctx context.Context, date string) ([][]string, error)

// SyncRequest is one store refresh: the days to reconcile, both order sources
// and whether to force the full export.
type SyncRequest struct {
	Dates []string
	// ExportRows runs the WP All Export for one day. Nil when the cron key is
	// missing: the sync then falls back to the creation-range lookup.
	ExportRows ExportRows
	// APIBase, APIKey and APISecret are the WooCommerce REST credentials of
	// the fast creation-range lookup. Empty disables it.
	APIBase   string
	APIKey    string
	APISecret string
	// CreatedDays is the lookup window in days back from now. Zero means
	// DefaultCreatedDays.
	CreatedDays int
	// Full forces the full export even for days that already have one.
	Full bool
}

// DefaultCreatedDays is how far back the fast lookup looks for created orders.
const DefaultCreatedDays = 45

// maxCreatedDays caps the lookup window so one sync cannot page through years.
const maxCreatedDays = 365

// wooOrdersPerPage and wooMaxPages bound one WooCommerce lookup.
const (
	wooOrdersPerPage = 100
	wooMaxPages      = 100
)

// SyncResult summarises a store sync for the panel.
type SyncResult struct {
	// Kept is the number of orders across the synced dates.
	Kept    int            `json:"kept"`
	Added   int            `json:"added"`
	Removed int            `json:"removed"`
	Dates   []string       `json:"dates"`
	PerDate map[string]int `json:"perDate"`
	// Exported are the days reconciled with the full export this run;
	// Refreshed the days updated with the creation-range lookup.
	Exported  []string `json:"exported,omitempty"`
	Refreshed []string `json:"refreshed,omitempty"`
	At        string   `json:"at"`
}

// SyncStore brings the requested days up to date. A day that was never
// reconciled pays the full export once (exact replace, marker saved); every
// later refresh uses the WooCommerce lookup by creation range, which is fast
// but cannot prove an order is gone, so it only adds, updates and removes the
// orders it actually saw.
func (c *Client) SyncStore(ctx context.Context, req SyncRequest) (SyncResult, error) {
	result := SyncResult{Dates: []string{}, PerDate: map[string]int{}, Exported: []string{}, Refreshed: []string{}}
	if c.kitchen.Index() == nil {
		return result, errors.New("el catálogo no está cargado")
	}
	if len(req.Dates) == 0 {
		return result, errors.New("sin fechas para sincronizar")
	}
	canExport := req.ExportRows != nil
	canLookup := strings.TrimSpace(req.APIBase) != "" && req.APIKey != "" && req.APISecret != ""

	var exports, lookups []string
	var failures []string
	c.kitchen.mu.Lock()
	for _, date := range req.Dates {
		_, exported := c.kitchen.state.Exported[date]
		switch {
		case (req.Full || !exported) && canExport:
			exports = append(exports, date)
		case canLookup:
			lookups = append(lookups, date)
		case canExport:
			// No lookup and no marker proof: the export is the only safe path.
			exports = append(exports, date)
		default:
			failures = append(failures, fmt.Sprintf("%s: falta la Cron key de WP All Export", date))
		}
	}
	c.kitchen.mu.Unlock()

	for _, date := range exports {
		orders, notes, annotations, err := exportOrders(ctx, req.ExportRows, date)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", date, err))
			continue
		}
		added, removed := c.replaceDay(date, orders, notes, annotations)
		c.markExported(date)
		result.Added += added
		result.Removed += removed
		result.Kept += len(orders)
		result.PerDate[date] = len(orders)
		result.Dates = append(result.Dates, date)
		result.Exported = append(result.Exported, date)
	}

	if len(lookups) > 0 && canLookup {
		window := req.CreatedDays
		if window <= 0 {
			window = DefaultCreatedDays
		}
		if window > maxCreatedDays {
			window = maxCreatedDays
		}
		orders, err := c.fetchStoreOrders(ctx, req.APIBase, req.APIKey, req.APISecret, time.Now().AddDate(0, 0, -window))
		if err != nil {
			failures = append(failures, err.Error())
		} else {
			added, removed, kept, perDate, dates := c.applyLookup(lookups, orders)
			result.Added += added
			result.Removed += removed
			result.Kept += kept
			for date, count := range perDate {
				result.PerDate[date] = count
			}
			result.Dates = append(result.Dates, dates...)
			result.Refreshed = append(result.Refreshed, dates...)
		}
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

// applyLookup groups the fetched orders by delivery date, keeps the requested
// days and merges them silently. Orders outside the requested days are ignored
// (the lookup window covers every delivery, not only these days).
func (c *Client) applyLookup(dates []string, orders []woocommerceOrder) (added, removed, kept int, perDate map[string]int, touched []string) {
	wanted := make(map[string]bool, len(dates))
	for _, date := range dates {
		wanted[date] = true
	}
	seen := map[string]map[string][]engine.ParsedOrderLine{}
	notes := map[string]map[string]string{}
	annotations := map[string]map[string]engine.OrderAnnotation{}
	removals := map[string]map[string]bool{}
	for index := range orders {
		order := &orders[index]
		row := wooRow(order)
		export := engine.ParseWideOrderRows([]map[string]any{row}, engine.WideExportOptions{})
		date := export.DeliveryDate
		if date == "" || !wanted[date] {
			continue
		}
		number := strings.TrimSpace(order.Number)
		if number == "" {
			number = strings.TrimSpace(stringOfAny(row["ID orden"]))
		}
		if number == "" {
			continue
		}
		if strings.EqualFold(strings.TrimSpace(order.Status), "processing") {
			if seen[date] == nil {
				seen[date] = map[string][]engine.ParsedOrderLine{}
				notes[date] = map[string]string{}
				annotations[date] = map[string]engine.OrderAnnotation{}
			}
			seen[date][number] = export.Lines
			if note := wooObservation(order); note != "" {
				notes[date][number] = note
			}
			if annotation, ok := export.Annotations[number]; ok {
				annotations[date][number] = annotation
			}
		} else {
			if removals[date] == nil {
				removals[date] = map[string]bool{}
			}
			removals[date][number] = true
		}
	}

	perDate = make(map[string]int, len(dates))
	for _, date := range dates {
		add, drop, total := c.mergeDay(date, seen[date], notes[date], annotations[date], removals[date])
		added += add
		removed += drop
		kept += total
		perDate[date] = total
		touched = append(touched, date)
	}
	sort.Strings(touched)
	return added, removed, kept, perDate, touched
}

// markExported records that the day paid for the full export once, so the next
// refresh can use the fast lookup.
func (c *Client) markExported(date string) {
	c.kitchen.mu.Lock()
	defer c.kitchen.mu.Unlock()
	if c.kitchen.state.Exported == nil {
		c.kitchen.state.Exported = map[string]time.Time{}
	}
	c.kitchen.state.Exported[date] = time.Now()
	_ = saveBotState(c.kitchen.path, c.kitchen.state)
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

// mergeDay applies a lookup to one day without judging the orders it did not
// see: processing orders are stored or replaced, non-processing ones are
// dropped and everything else stays. The creation window cannot prove an
// order is gone, so the day is never wiped here.
func (c *Client) mergeDay(date string, orders map[string][]engine.ParsedOrderLine, notes map[string]string, annotations map[string]engine.OrderAnnotation, removals map[string]bool) (added, removed, total int) {
	c.kitchen.mu.Lock()
	defer c.kitchen.mu.Unlock()
	state := c.kitchen.state
	previous := state.Orders[date]
	if len(orders) == 0 && len(removals) == 0 {
		return 0, 0, len(previous)
	}
	for number := range orders {
		if _, ok := previous[number]; !ok {
			added++
		}
	}
	for number := range removals {
		if _, ok := previous[number]; ok {
			removed++
		}
	}
	if state.Orders == nil {
		state.Orders = map[string]map[string][]engine.ParsedOrderLine{}
	}
	if state.Orders[date] == nil {
		state.Orders[date] = map[string][]engine.ParsedOrderLine{}
	}
	if state.Observations[date] == nil {
		state.Observations[date] = map[string]string{}
	}
	if state.Annotations[date] == nil {
		state.Annotations[date] = map[string]engine.OrderAnnotation{}
	}
	for number, lines := range orders {
		state.Orders[date][number] = lines
		if note := notes[number]; note != "" {
			state.Observations[date][number] = note
		} else {
			delete(state.Observations[date], number)
		}
		if annotation, ok := annotations[number]; ok {
			state.Annotations[date][number] = annotation
		} else {
			delete(state.Annotations[date], number)
		}
	}
	for number := range removals {
		delete(state.Orders[date], number)
		delete(state.Observations[date], number)
		delete(state.Annotations[date], number)
	}
	total = len(state.Orders[date])
	if total == 0 {
		delete(state.Orders, date)
		delete(state.Observations, date)
		delete(state.Annotations, date)
	}
	if next := c.listFromStateLocked(date); next != nil {
		state.Lists[date] = *next
	} else {
		delete(state.Lists, date)
	}
	if added > 0 || removed > 0 {
		addEventLocked(state, "update", fmt.Sprintf("Actualizado desde la tienda (%s): %d nuevos, %d quitados", date, added, removed))
	}
	_ = saveBotState(c.kitchen.path, state)
	return added, removed, total
}

// fetchStoreOrders pages the WooCommerce REST orders created after `after`.
// The delivery date lives in meta_data, which the REST API cannot filter, so
// the lookup takes a creation window and the caller groups by delivery date.
func (c *Client) fetchStoreOrders(ctx context.Context, base, key, secret string, after time.Time) ([]woocommerceOrder, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	if base == "" {
		return nil, errors.New("falta la URL de la tienda (Ajustes)")
	}
	client := &http.Client{Timeout: 60 * time.Second}
	var out []woocommerceOrder
	for page := 1; page <= wooMaxPages; page++ {
		endpoint := base + "/wp-json/wc/v3/orders?per_page=" + strconv.Itoa(wooOrdersPerPage) +
			"&page=" + strconv.Itoa(page) +
			"&orderby=date&order=desc" +
			"&after=" + url.QueryEscape(after.Format(time.RFC3339))
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, err
		}
		request.SetBasicAuth(key, secret)
		request.Header.Set("Accept", "application/json")
		response, err := client.Do(request)
		if err != nil {
			return nil, fmt.Errorf("no se pudo consultar la tienda: %w", err)
		}
		raw, readErr := io.ReadAll(io.LimitReader(response.Body, 32<<20))
		response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("la tienda respondió %s al listar pedidos", response.Status)
		}
		if readErr != nil {
			return nil, readErr
		}
		var batch []woocommerceOrder
		if err := json.Unmarshal(raw, &batch); err != nil {
			return nil, fmt.Errorf("respuesta inesperada de la tienda: %w", err)
		}
		out = append(out, batch...)
		if len(batch) < wooOrdersPerPage {
			break
		}
	}
	return out, nil
}
