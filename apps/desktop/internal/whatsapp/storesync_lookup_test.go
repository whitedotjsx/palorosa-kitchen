package whatsapp

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

// orderJSON builds one WooCommerce order payload for the lookup tests.
func orderJSON(number, status, spanishDate string) map[string]any {
	return map[string]any{
		"number": number,
		"status": status,
		"meta_data": []map[string]any{
			{"key": "Seleccionar una Fecha", "value": spanishDate},
		},
		"line_items": []any{},
	}
}

func lookupServer(t *testing.T, orders []map[string]any) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, pass, ok := r.BasicAuth(); !ok || user != "ck" || pass != "cs" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(orders)
	}))
	t.Cleanup(server.Close)
	return server
}

// TestSyncStoreExportsOnceThenUsesLookup is the hybrid rule: the first sync of
// a day pays the full export (exact replace), every later sync uses the fast
// creation-range lookup, and a forced sync exports again.
func TestSyncStoreExportsOnceThenUsesLookup(t *testing.T) {
	c := testClient(t)
	exportCalls := 0
	fetch := func(_ context.Context, date string) ([][]string, error) {
		exportCalls++
		if date != "2026-10-04" {
			return [][]string{header}, nil
		}
		return [][]string{
			header,
			{"1", "4 octubre, 2026", "", "", ""},
			{"2", "4 octubre, 2026", "", "", ""},
		}, nil
	}
	server := lookupServer(t, []map[string]any{
		orderJSON("2", "processing", "4 octubre, 2026"),
		orderJSON("3", "processing", "4 octubre, 2026"),
		orderJSON("1", "cancelled", "4 octubre, 2026"),
		orderJSON("4", "processing", "5 octubre, 2026"),
	})
	request := func(full bool) SyncRequest {
		return SyncRequest{
			Dates:       []string{"2026-10-04"},
			ExportRows:  fetch,
			APIBase:     server.URL,
			APIKey:      "ck",
			APISecret:   "cs",
			CreatedDays: 45,
			Full:        full,
		}
	}

	first, err := c.SyncStore(context.Background(), request(false))
	if err != nil {
		t.Fatal(err)
	}
	if exportCalls != 1 || len(first.Exported) != 1 || len(first.Refreshed) != 0 {
		t.Fatalf("first sync = %+v, exportCalls = %d", first, exportCalls)
	}
	if day := c.kitchen.state.Orders["2026-10-04"]; len(day) != 2 {
		t.Fatalf("day after export = %v", day)
	}
	if _, ok := c.kitchen.state.Exported["2026-10-04"]; !ok {
		t.Fatal("day was not marked as exported")
	}

	second, err := c.SyncStore(context.Background(), request(false))
	if err != nil {
		t.Fatal(err)
	}
	if exportCalls != 1 || len(second.Refreshed) != 1 || len(second.Exported) != 0 {
		t.Fatalf("second sync = %+v, exportCalls = %d", second, exportCalls)
	}
	day := c.kitchen.state.Orders["2026-10-04"]
	_, hasTwo := day["2"]
	_, hasThree := day["3"]
	_, hasOne := day["1"]
	if len(day) != 2 || !hasTwo || !hasThree || hasOne {
		t.Fatalf("day after lookup = %v", day)
	}
	if second.Added != 1 || second.Removed != 1 {
		t.Fatalf("lookup counts = added %d, removed %d", second.Added, second.Removed)
	}

	forced, err := c.SyncStore(context.Background(), request(true))
	if err != nil {
		t.Fatal(err)
	}
	if exportCalls != 2 || len(forced.Exported) != 1 {
		t.Fatalf("forced sync = %+v, exportCalls = %d", forced, exportCalls)
	}
	day = c.kitchen.state.Orders["2026-10-04"]
	if len(day) != 2 {
		t.Fatalf("day after forced export = %v", day)
	}
	if _, ok := day["3"]; ok {
		t.Fatal("the lookup-added order survived the forced export")
	}
}

func TestFetchStoreOrdersPaginates(t *testing.T) {
	c := testClient(t)
	page := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		page++
		count := 100
		if page == 2 {
			count = 1
		}
		if page > 2 {
			count = 0
		}
		batch := make([]map[string]any, 0, count)
		for i := 0; i < count; i++ {
			batch = append(batch, orderJSON(fmt.Sprintf("%d", page*1000+i), "processing", "4 octubre, 2026"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(batch)
	}))
	t.Cleanup(server.Close)

	orders, err := c.fetchStoreOrders(context.Background(), server.URL, "ck", "cs", time.Now().AddDate(0, 0, -45))
	if err != nil {
		t.Fatal(err)
	}
	if len(orders) != 101 {
		t.Fatalf("orders = %d, want 101", len(orders))
	}
	if page != 2 {
		t.Fatalf("pages = %d, want 2", page)
	}
}
