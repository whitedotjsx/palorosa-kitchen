package whatsapp

import (
	"context"
	"errors"
	"testing"

	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/engine"
)

func testClient(t testing.TB) *Client {
	t.Helper()
	kitchen := NewKitchen(t.TempDir(), "../../../../data/seed.json", nil)
	if kitchen.Index() == nil {
		t.Skip("seed catalog not available")
	}
	return &Client{kitchen: kitchen, log: waLog.Noop}
}

var header = []string{"ID orden", "Fecha de Entrega", "Productos", "Adicionales", "Observaciones"}

func TestSyncFromExportReplacesTheDay(t *testing.T) {
	c := testClient(t)
	c.kitchen.state.Orders["2026-10-04"] = map[string][]engine.ParsedOrderLine{"1": nil, "2": nil}
	fetch := func(_ context.Context, date string) ([][]string, error) {
		if date != "2026-10-04" {
			return [][]string{header}, nil
		}
		return [][]string{
			header,
			{"2", "4 octubre, 2026", "", "", ""},
			{"3", "4 octubre, 2026", "", "", "Sin azúcar"},
		}, nil
	}
	result, err := c.SyncFromExport(context.Background(), fetch, []string{"2026-10-04", "2026-10-05"})
	if err != nil {
		t.Fatal(err)
	}
	day := c.kitchen.state.Orders["2026-10-04"]
	if len(day) != 2 || day["1"] != nil || result.Added != 1 || result.Removed != 1 || result.PerDate["2026-10-04"] != 2 {
		t.Fatalf("day = %v, result = %+v", day, result)
	}
	if _, ok := day["3"]; !ok {
		t.Fatal("order 3 missing")
	}
	if c.kitchen.state.Observations["2026-10-04"]["3"] != "Sin azúcar" {
		t.Fatalf("observations = %v", c.kitchen.state.Observations["2026-10-04"])
	}
	if len(result.Dates) != 2 {
		t.Fatalf("dates = %v", result.Dates)
	}
}

func TestSyncFromExportKeepsTheDayOnBadExports(t *testing.T) {
	c := testClient(t)
	c.kitchen.state.Orders["2026-10-04"] = map[string][]engine.ParsedOrderLine{"1": nil}
	cases := map[string]func(context.Context, string) ([][]string, error){
		"error":      func(context.Context, string) ([][]string, error) { return nil, errors.New("boom") },
		"wrong date": func(context.Context, string) ([][]string, error) { return [][]string{header, {"9", "5 octubre, 2026", "", "", ""}}, nil },
		"no header":  func(context.Context, string) ([][]string, error) { return [][]string{{"a", "b"}, {"1", "2"}}, nil },
	}
	for name, fetch := range cases {
		if _, err := c.SyncFromExport(context.Background(), fetch, []string{"2026-10-04"}); err == nil {
			t.Fatalf("%s: expected an error", name)
		}
		if _, ok := c.kitchen.state.Orders["2026-10-04"]["1"]; !ok {
			t.Fatalf("%s: the day was wiped", name)
		}
	}
}

// TestSyncFromExportDropsInactiveStatuses is the status column rule: the saved
// export carries an Estado column now, processing and pending rows are cooked
// and the inactive ones never enter the day.
func TestSyncFromExportDropsInactiveStatuses(t *testing.T) {
	c := testClient(t)
	withStatus := []string{"ID orden", "Fecha de Entrega", "Estado", "Productos", "Adicionales", "Observaciones"}
	fetch := func(_ context.Context, _ string) ([][]string, error) {
		return [][]string{
			withStatus,
			{"1", "4 octubre, 2026", "processing", "", "", ""},
			{"2", "4 octubre, 2026", "pending", "", "", ""},
			{"3", "4 octubre, 2026", "cancelled", "", "", ""},
			{"4", "4 octubre, 2026", "wc-cancelled", "", "", ""},
		}, nil
	}
	result, err := c.SyncFromExport(context.Background(), fetch, []string{"2026-10-04"})
	if err != nil {
		t.Fatal(err)
	}
	day := c.kitchen.state.Orders["2026-10-04"]
	_, hasProcessing := day["1"]
	_, hasPending := day["2"]
	_, hasCancelled := day["3"]
	_, hasPrefixed := day["4"]
	if len(day) != 2 || !hasProcessing || !hasPending || hasCancelled || hasPrefixed {
		t.Fatalf("day = %v", day)
	}
	if result.Kept != 2 {
		t.Fatalf("kept = %d, want 2", result.Kept)
	}
}
