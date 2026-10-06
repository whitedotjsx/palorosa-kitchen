package whatsapp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"go.mau.fi/whatsmeow/types"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/engine"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/labels"
)

// Handler builds the order hook router so the panel server can mount the same
// routes on a single origin (D27). ServeHooks uses it directly.
func (c *Client) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", c.handleHealth)
	mux.HandleFunc("/hook/order", c.handleHookOrder)
	mux.HandleFunc("/hook/orders", c.handleHookOrders)
	mux.HandleFunc("/hook/woocommerce", c.handleWooCommerce)
	mux.HandleFunc("/debug/reply", c.handleDebugReply)
	mux.HandleFunc("/debug/send", c.handleDebugSend)
	return mux
}

// ServeHooks runs the order hook HTTP server (POST /hook/order, POST
// /hook/orders, GET /health). It stops when ctx is cancelled.
func (c *Client) ServeHooks(ctx context.Context) error {
	if c.cfg.HookPort == 0 {
		return nil
	}
	server := &http.Server{
		Addr:              ":" + strconv.Itoa(c.cfg.HookPort),
		Handler:           c.Handler(),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()

	c.log.Infof("Hook server listening on http://localhost:%d/hook/order", c.cfg.HookPort)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

// handleDebugReply runs a command as if it arrived from a number and returns
// the reply text. Local machine only; it is a testing aid for the commands.
func (c *Client) handleDebugReply(w http.ResponseWriter, r *http.Request) {
	if !isLoopback(r) {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "Local only"})
		return
	}
	var payload struct {
		Text string `json:"text"`
		From string `json:"from"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Invalid JSON body"})
		return
	}
	if payload.From != "" && !c.allowed(payload.From) {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "Number not in allowlist"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "reply": c.reply(context.Background(), payload.Text)})
}

// handleDebugSend sends a text to an allowlisted number. Local machine only;
// a testing aid for verifying delivery.
func (c *Client) handleDebugSend(w http.ResponseWriter, r *http.Request) {
	if !isLoopback(r) {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "Local only"})
		return
	}
	var payload struct {
		To   string `json:"to"`
		Text string `json:"text"`
	}
	if err := decodeJSON(r, &payload); err != nil || payload.To == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Expected to and text"})
		return
	}
	if c.cli == nil {
		writeJSON(w, http.StatusServiceUnavailable, map[string]any{"ok": false, "error": "WhatsApp no conectado"})
		return
	}
	if !c.allowed(payload.To) {
		writeJSON(w, http.StatusForbidden, map[string]any{"ok": false, "error": "Number not in allowlist"})
		return
	}
	response, err := c.send(context.Background(), types.NewJID(payload.To, types.DefaultUserServer), payload.Text)
	if err != nil {
		writeJSON(w, http.StatusBadGateway, map[string]any{"ok": false, "error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "id": response.ID})
}

func isLoopback(r *http.Request) bool {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return false
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (c *Client) handleHealth(w http.ResponseWriter, _ *http.Request) {
	c.kitchen.mu.Lock()
	dates := make([]string, 0, len(c.kitchen.state.Lists))
	for date := range c.kitchen.state.Lists {
		dates = append(dates, date)
	}
	c.kitchen.mu.Unlock()
	sort.Strings(dates)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok": true,
		"whatsapp": map[string]any{
			"ready":     c.cli != nil && c.cli.IsConnected(),
			"lastError": "",
		},
		"dates": dates,
	})
}

func (c *Client) handleHookOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "Method not allowed"})
		return
	}
	if !c.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "Unauthorized"})
		return
	}
	var payload struct {
		DeliveryDate string                   `json:"deliveryDate"`
		OrderNumber  any                      `json:"orderNumber"`
		Lines        []engine.ParsedOrderLine `json:"lines"`
		Note         string                   `json:"note"`
		Observacion  string                   `json:"observacion"`
		Color        string                   `json:"color"`
		Motivo       string                   `json:"motivo"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Invalid JSON body"})
		return
	}
	number := orderNumberString(payload.OrderNumber)
	if !isDate(payload.DeliveryDate) || number == "" || payload.Lines == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Expected deliveryDate, orderNumber and lines"})
		return
	}
	note := payload.Note
	if note == "" {
		note = payload.Observacion
	}
	annotation := engine.OrderAnnotation{
		Color:  engine.AnnotationLabel(payload.Color),
		Reason: engine.AnnotationLabel(payload.Motivo),
	}
	result := c.applyOrder(payload.DeliveryDate, number, payload.Lines, fmt.Sprintf("%s #%s", labels.Bot.NewOrder, number), note, annotation, eventNew)
	writeJSON(w, http.StatusOK, result)
}

func (c *Client) handleHookOrders(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "Method not allowed"})
		return
	}
	if !c.authorized(r) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "Unauthorized"})
		return
	}
	var payload struct {
		DeliveryDate string `json:"deliveryDate"`
		Orders       []struct {
			OrderNumber any                      `json:"orderNumber"`
			Lines       []engine.ParsedOrderLine `json:"lines"`
			Color       string                   `json:"color"`
			Motivo      string                   `json:"motivo"`
		} `json:"orders"`
	}
	if err := decodeJSON(r, &payload); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Invalid JSON body"})
		return
	}
	if !isDate(payload.DeliveryDate) || payload.Orders == nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Expected deliveryDate and orders"})
		return
	}
	orders := map[string][]engine.ParsedOrderLine{}
	annotations := map[string]engine.OrderAnnotation{}
	for index, order := range payload.Orders {
		number := orderNumberString(order.OrderNumber)
		if number == "" {
			number = strconv.Itoa(index + 1)
		}
		lines := order.Lines
		if lines == nil {
			lines = []engine.ParsedOrderLine{}
		}
		orders[number] = lines
		annotation := engine.OrderAnnotation{
			Color:  engine.AnnotationLabel(order.Color),
			Reason: engine.AnnotationLabel(order.Motivo),
		}
		if annotation.Color != "" || annotation.Reason != "" {
			annotations[number] = annotation
		}
	}
	result := c.applyOrders(payload.DeliveryDate, orders, annotations, labels.Bot.UpdatedOrder, "", eventUpdate)
	writeJSON(w, http.StatusOK, result)
}

func (c *Client) applyOrder(date, number string, lines []engine.ParsedOrderLine, title, note string, annotation engine.OrderAnnotation, kind string) map[string]any {
	if c.kitchen.Index() == nil {
		return map[string]any{"ok": false, "error": "Catalog not loaded"}
	}
	c.kitchen.mu.Lock()
	previousDay := c.previousListLocked(date)
	previousLines := c.kitchen.state.Orders[date][number]
	previousNote := ""
	if observations, ok := c.kitchen.state.Observations[date]; ok {
		previousNote = observations[number]
	}
	if c.kitchen.state.Orders[date] == nil {
		c.kitchen.state.Orders[date] = map[string][]engine.ParsedOrderLine{}
	}
	c.kitchen.state.Orders[date][number] = lines
	if note != "" {
		if c.kitchen.state.Observations[date] == nil {
			c.kitchen.state.Observations[date] = map[string]string{}
		}
		c.kitchen.state.Observations[date][number] = note
	}
	if annotation.Color != "" || annotation.Reason != "" {
		if c.kitchen.state.Annotations[date] == nil {
			c.kitchen.state.Annotations[date] = map[string]engine.OrderAnnotation{}
		}
		c.kitchen.state.Annotations[date][number] = annotation
	} else {
		delete(c.kitchen.state.Annotations[date], number)
	}
	next := c.listFromStateLocked(date)
	if next != nil {
		c.kitchen.state.Lists[date] = *next
	} else {
		delete(c.kitchen.state.Lists, date)
	}
	diff := engine.DiffLists(previousDay, emptyIfNil(next))
	if len(previousLines) == 0 {
		addEventLocked(c.kitchen.state, "new", strings.ReplaceAll(labels.Activity.NewOrder, "{n}", number))
	} else if !sameUnits(c.orderUnits(previousLines), c.orderUnits(lines)) || orderText(previousLines) != orderText(lines) || previousNote != note {
		addEventLocked(c.kitchen.state, "update", strings.ReplaceAll(labels.Activity.UpdateOrder, "{n}", number))
	}
	delete(c.kitchen.state.Notified[date], number)
	_ = saveBotState(c.kitchen.path, c.kitchen.state)
	c.kitchen.mu.Unlock()

	// Build the order-level notice: the order text, its food units before and
	// after, and the resulting totals in the day list.
	before := c.orderUnits(previousLines)
	after := c.orderUnits(lines)
	isNew := len(previousLines) == 0
	foodChanged := !sameUnits(before, after)
	changed := isNew || foodChanged || orderText(previousLines) != orderText(lines) || previousNote != note

	notified := 0
	if changed {
		noticeKind := eventUpdate
		if isNew {
			noticeKind = eventNew
		}
		notice := engine.OrderNotice{
			Title:             title,
			When:              whenLabel(date),
			Date:              date,
			OrderText:         orderText(lines),
			Note:              note,
			Kind:              noticeKind,
			Before:            before,
			After:             after,
			Totals:            c.totalsFor(next, noticeNames(before, after)),
			TotalsWerePresent: presentUnits(previousDay),
			FoodChanged:       foodChanged || isNew,
		}
		notified = c.sendNotification(engine.FormatOrderNotice(notice), noticeKind, date)
	}
	if notified > 0 {
		c.markNotified(date, number)
	}
	c.changed()
	return hookResult(notified, diff)
}

// markNotified records that an immediate notice went out for an order.
func (c *Client) markNotified(date, number string) {
	c.kitchen.mu.Lock()
	if c.kitchen.state.Notified[date] == nil {
		c.kitchen.state.Notified[date] = map[string]bool{}
	}
	c.kitchen.state.Notified[date][number] = true
	_ = saveBotState(c.kitchen.path, c.kitchen.state)
	c.kitchen.mu.Unlock()
}

// applyRemoval drops an order from a day (cancelled, trashed or no longer
// processing) and notifies its before/after units.
func (c *Client) applyRemoval(date, number, title string) map[string]any {
	if c.kitchen.Index() == nil {
		return map[string]any{"ok": false, "error": "Catalog not loaded"}
	}
	c.kitchen.mu.Lock()
	previousDay := c.previousListLocked(date)
	previousLines := c.kitchen.state.Orders[date][number]
	if orders, ok := c.kitchen.state.Orders[date]; ok {
		delete(orders, number)
		if len(orders) == 0 {
			delete(c.kitchen.state.Orders, date)
		}
	}
	if observations, ok := c.kitchen.state.Observations[date]; ok {
		delete(observations, number)
	}
	delete(c.kitchen.state.Annotations[date], number)
	delete(c.kitchen.state.Notified[date], number)
	if len(previousLines) > 0 {
		addEventLocked(c.kitchen.state, "removed", strings.ReplaceAll(labels.Activity.Removed, "{n}", number))
	}
	next := c.listFromStateLocked(date)
	if next != nil {
		c.kitchen.state.Lists[date] = *next
	} else {
		delete(c.kitchen.state.Lists, date)
	}
	diff := engine.DiffLists(previousDay, emptyIfNil(next))
	_ = saveBotState(c.kitchen.path, c.kitchen.state)
	c.kitchen.mu.Unlock()

	before := c.orderUnits(previousLines)
	notified := 0
	if len(before) > 0 {
		notice := engine.OrderNotice{
			Title:             title,
			When:              whenLabel(date),
			Date:              date,
			OrderText:         orderText(previousLines),
			Kind:              eventUpdate,
			Before:            before,
			After:             nil,
			Totals:            c.totalsFor(next, noticeNames(before, nil)),
			TotalsWerePresent: presentUnits(previousDay),
			FoodChanged:       true,
		}
		notified = c.sendNotification(engine.FormatOrderNotice(notice), eventUpdate, date)
	}
	c.changed()
	return hookResult(notified, diff)
}

// orderUnits resolves an order's lines to its aggregated kitchen units.
func (c *Client) orderUnits(lines []engine.ParsedOrderLine) []engine.KitchenListEntry {
	index := c.kitchen.Index()
	if len(lines) == 0 || index == nil {
		return nil
	}
	return engine.AggregateUnits(engine.ResolveLines(lines, index), index).Entries
}

func orderText(lines []engine.ParsedOrderLine) string {
	parts := make([]string, 0, len(lines))
	for _, line := range lines {
		parts = append(parts, fmt.Sprintf("x%d %s", line.Quantity, line.ProductText))
	}
	return strings.Join(parts, " | ")
}

func sameUnits(a, b []engine.KitchenListEntry) bool {
	if len(a) != len(b) {
		return false
	}
	quantities := make(map[string]int, len(a))
	for _, entry := range a {
		quantities[entry.UnitID] = entry.Quantity
	}
	for _, entry := range b {
		if quantities[entry.UnitID] != entry.Quantity {
			return false
		}
	}
	return true
}
func noticeNames(before, after []engine.KitchenListEntry) map[string]string {
	names := map[string]string{}
	for _, entry := range before {
		names[entry.UnitID] = entry.Name
	}
	for _, entry := range after {
		names[entry.UnitID] = entry.Name
	}
	return names
}

// presentUnits reports which units were already in the day list.
func presentUnits(list *engine.KitchenList) map[string]bool {
	present := map[string]bool{}
	if list != nil {
		for _, entry := range list.Entries {
			present[entry.UnitID] = true
		}
	}
	return present
}

// totalsFor returns the day totals after the change for the affected units.
func (c *Client) totalsFor(list *engine.KitchenList, names map[string]string) []engine.KitchenListEntry {
	if len(names) == 0 {
		return nil
	}
	present := map[string]engine.KitchenListEntry{}
	if list != nil {
		for _, entry := range list.Entries {
			present[entry.UnitID] = entry
		}
	}
	out := make([]engine.KitchenListEntry, 0, len(names))
	for id, name := range names {
		if entry, ok := present[id]; ok {
			out = append(out, entry)
		} else {
			out = append(out, engine.KitchenListEntry{UnitID: id, Name: name})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

func (c *Client) applyOrders(date string, orders map[string][]engine.ParsedOrderLine, annotations map[string]engine.OrderAnnotation, title, note, kind string) map[string]any {
	if c.kitchen.Index() == nil {
		return map[string]any{"ok": false, "error": "Catalog not loaded"}
	}
	c.kitchen.mu.Lock()
	previous := c.previousListLocked(date)
	c.kitchen.state.Orders[date] = orders
	if len(annotations) > 0 {
		c.kitchen.state.Annotations[date] = annotations
	} else {
		delete(c.kitchen.state.Annotations, date)
	}
	next := c.listFromStateLocked(date)
	if next != nil {
		c.kitchen.state.Lists[date] = *next
	}
	diff := engine.DiffLists(previous, emptyIfNil(next))
	_ = saveBotState(c.kitchen.path, c.kitchen.state)
	c.kitchen.mu.Unlock()

	notified := c.notifyDiff(diff, title, note, date, kind)
	c.changed()
	return hookResult(notified, diff)
}

// previousListLocked returns a copy of the last list so the diff has a stable
// snapshot. The caller holds stateMu.
func (c *Client) previousListLocked(date string) *engine.KitchenList {
	if list, ok := c.kitchen.state.Lists[date]; ok {
		return &list
	}
	return nil
}

func (c *Client) authorized(r *http.Request) bool {
	if c.cfg.HookToken == "" {
		return true
	}
	return r.Header.Get("x-bot-token") == c.cfg.HookToken
}

func hookResult(notified int, diff engine.ListDiff) map[string]any {
	return map[string]any{
		"ok":       true,
		"notified": notified,
		"added":    len(diff.Added),
		"changed":  len(diff.Changed),
		"removed":  len(diff.Removed),
	}
}

func emptyIfNil(list *engine.KitchenList) engine.KitchenList {
	if list == nil {
		return engine.KitchenList{
			Entries:         []engine.KitchenListEntry{},
			Unresolved:      []engine.UnresolvedEntry{},
			IgnoredProducts: []engine.IgnoredEntry{},
			IgnoredUnits:    []engine.IgnoredEntry{},
			Warnings:        []engine.WarningEntry{},
		}
	}
	return *list
}

func orderNumberString(value any) string {
	switch typed := value.(type) {
	case nil:
		return ""
	case string:
		return typed
	case float64:
		if typed == float64(int64(typed)) {
			return strconv.FormatInt(int64(typed), 10)
		}
		return strconv.FormatFloat(typed, 'f', -1, 64)
	default:
		return fmt.Sprint(typed)
	}
}

func isDate(value string) bool {
	if len(value) != 10 {
		return false
	}
	_, err := time.Parse("2006-01-02", value)
	return err == nil
}

func decodeJSON(r *http.Request, target any) error {
	return json.NewDecoder(r.Body).Decode(target)
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("content-type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(data)
}
