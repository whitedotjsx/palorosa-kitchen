package engine

import (
	"strings"
	"testing"
)

func TestFormatOrderNoticeUpdate(t *testing.T) {
	notice := OrderNotice{
		Title:     "Pedido actualizado #75627",
		When:      "MAÑANA",
		Date:      "2026-10-02",
		OrderText: "x1 Gold Pro",
		Note:      "Entregar entre las 9:00 am y 10:00 am.",
		Kind:      "update",
		Before: []KitchenListEntry{
			{UnitID: "jugo", Name: "Jugo de naranja", Quantity: 1},
			{UnitID: "sandwich", Name: "Sándwich sencillo", Quantity: 1},
		},
		After: []KitchenListEntry{
			{UnitID: "jugo", Name: "Jugo de naranja", Quantity: 2},
			{UnitID: "parfait", Name: "Parfait", Quantity: 3},
		},
		Totals: []KitchenListEntry{
			{UnitID: "jugo", Name: "Jugo de naranja", Quantity: 3},
			{UnitID: "sandwich", Name: "Sándwich sencillo", Quantity: 0},
			{UnitID: "parfait", Name: "Parfait", Quantity: 3},
		},
		TotalsWerePresent: map[string]bool{"jugo": true, "sandwich": true, "parfait": false},
		FoodChanged:       true,
	}
	want := `Pedido actualizado #75627
MAÑANA · viernes 2 de octubre de 2026

🧾 Pedido: x1 Gold Pro
📝 Observación: Entregar entre las 9:00 am y 10:00 am.

📄 Pedido original:
- 1 × Jugo de naranja
- 1 × Sándwich sencillo

✏️ Pedido actualizado:
- 2 × Jugo de naranja
- 0 × Sándwich sencillo (❌ se eliminó 1)
- 3 × Parfait (✅ se agregaron 3)

📋 Así queda la lista:
- 3 × Jugo de naranja (🔄 nuevo total)
- 0 × Sándwich sencillo (🚫 ya no hay que hacer)
- 3 × Parfait (🆕 nuevo en la lista)

ℹ️ Escribe LISTA para ver la lista completa.`
	if got := FormatOrderNotice(notice); got != want {
		t.Errorf("FormatOrderNotice mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestFormatOrderNoticeNoFoodChange(t *testing.T) {
	notice := OrderNotice{
		Title: "Pedido actualizado #1", When: "HOY", Date: "2026-10-02",
		OrderText: "x1 Gold Pro", Note: "Cambió la dirección.", Kind: "update",
		FoodChanged: false,
	}
	got := FormatOrderNotice(notice)
	if !strings.Contains(got, "no cambió nada de comida") {
		t.Errorf("expected the no-food-change line, got:\n%s", got)
	}
}

func TestFormatOrderNoticeNew(t *testing.T) {
	notice := OrderNotice{
		Title: "Pedido nuevo #2", When: "MAÑANA", Date: "2026-10-02",
		OrderText: "x1 Gold Basic", Kind: "new",
		After:  []KitchenListEntry{{UnitID: "s", Name: "Sándwich sencillo", Quantity: 1}},
		Totals: []KitchenListEntry{{UnitID: "s", Name: "Sándwich sencillo", Quantity: 4}},
	}
	got := FormatOrderNotice(notice)
	if !strings.Contains(got, "Hay que agregar:") || !strings.Contains(got, "Total en la lista (después de agregar):") {
		t.Errorf("new notice missing sections:\n%s", got)
	}
}
