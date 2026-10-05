package whatsapp

import (
	"strconv"
	"testing"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/engine"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/panelmodel"
)

func TestOrderFacetsGroupsVariants(t *testing.T) {
	lines := []engine.ParsedOrderLine{
		{ProductText: "Box Mujer", Quantity: 1, Source: "breakfast", Options: []string{"Rosado", "Jugo de naranja", "color: Azul"}},
		{ProductText: "Box Mujer", Quantity: 1, Source: "breakfast", Options: []string{"Rosado"}},
		{ProductText: "Mini tabla de quesos adicional (Picnic)", Quantity: 1, Source: "add_on"},
	}
	facets := orderFacets(lines)
	if len(facets) != 5 {
		t.Fatalf("facets = %+v", facets)
	}
	assertFacet(t, facets, "breakfast", "Box Mujer")
	assertFacet(t, facets, "add_on", "Mini tabla de quesos adicional (Picnic)")
	assertFacet(t, facets, "color", "Rosado")
	assertFacet(t, facets, "color", "color: Azul")
	assertFacet(t, facets, "other", "Jugo de naranja")
}

func assertFacet(t *testing.T, facets []panelmodel.Facet, kind, value string) {
	t.Helper()
	for _, facet := range facets {
		if facet.Kind == kind && facet.Value == value {
			return
		}
	}
	t.Fatalf("facet %s/%s missing in %+v", kind, value, facets)
}

func TestIsColorValue(t *testing.T) {
	for _, value := range []string{"Rosado", "Dorado/negro", "color: azul", "vinotinto", "Amarilla", "Rosado con dorado"} {
		if !isColorValue(value) {
			t.Errorf("%q should be a color", value)
		}
	}
	for _, value := range []string{"Jugo de naranja", "Cumpleaños", "Café Mocca", "Box Mujer", ""} {
		if isColorValue(value) {
			t.Errorf("%q should not be a color", value)
		}
	}
}

// seedManyOrders fills a day with the given number of realistic orders.
func seedManyOrders(c *Client, date string, total int) {
	orders := map[string][]engine.ParsedOrderLine{}
	for i := 0; i < total; i++ {
		number := strconv.Itoa(7000 + i)
		orders[number] = []engine.ParsedOrderLine{
			{ProductText: "Box Mujer", Quantity: 1, Source: "breakfast", Options: []string{"Jugo de naranja", "Sándwich sencillo", "Rosado"}},
			{ProductText: "Box Hombre", Quantity: 1, Source: "breakfast", Options: []string{"Café Mocca", "Waffles"}},
			{ProductText: "Mini tabla de quesos adicional (Picnic)", Quantity: 1, Source: "add_on"},
		}
	}
	c.kitchen.state.Orders[date] = orders
}

// BenchmarkPanelDay600Orders measures the expensive snapshot path: resolving
// 600 orders into their units, facets and the aggregated list.
func BenchmarkPanelDay600Orders(b *testing.B) {
	c := testClient(b)
	seedManyOrders(c, "2026-10-04", 600)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.kitchen.mu.Lock()
		day := c.dayLocked("2026-10-04")
		c.kitchen.mu.Unlock()
		if len(day.Orders) != 600 || len(day.List.Entries) == 0 {
			b.Fatalf("day = %d orders, %d entries", len(day.Orders), len(day.List.Entries))
		}
	}
}

func BenchmarkOrderDetail(b *testing.B) {
	c := testClient(b)
	seedManyOrders(c, "2026-10-04", 600)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := c.OrderDetail("2026-10-04", "7000"); err != nil {
			b.Fatal(err)
		}
	}
}

func TestOrderDetailResolvesLines(t *testing.T) {
	c := testClient(t)
	c.kitchen.state.Orders["2026-10-04"] = map[string][]engine.ParsedOrderLine{
		"7": {
			{ProductText: "Box Mujer", Quantity: 2, Source: "breakfast", Options: []string{"Jugo de naranja", "Sándwich sencillo", "Rosado"}},
			{ProductText: "Producto inventado", Quantity: 1, Source: "add_on"},
		},
	}
	c.kitchen.state.Observations["2026-10-04"] = map[string]string{"7": "Sin azúcar"}

	detail, err := c.OrderDetail("2026-10-04", "7")
	if err != nil {
		t.Fatal(err)
	}
	if detail.Note != "Sin azúcar" || len(detail.Lines) != 2 {
		t.Fatalf("detail = %+v", detail)
	}
	if detail.Lines[0].Status != "resolved" || len(detail.Lines[0].Units) == 0 {
		t.Fatalf("line 0 = %+v", detail.Lines[0])
	}
	if detail.Lines[1].Status != "unresolved" {
		t.Fatalf("line 1 = %+v", detail.Lines[1])
	}
	if detail.Units == 0 || len(detail.Entries) == 0 {
		t.Fatalf("entries = %+v", detail.Entries)
	}

	if _, err := c.OrderDetail("2026-10-04", "404"); err == nil {
		t.Fatal("expected an error for a missing order")
	}
}
