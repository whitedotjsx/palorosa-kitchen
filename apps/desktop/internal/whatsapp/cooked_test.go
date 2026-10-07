package whatsapp

import "testing"

// TestOrderIsCookedStatuses pins the status rule: only inactive orders are
// left out, pending is cooked, and an unknown status is kept so a translated
// label never wipes the day.
func TestOrderIsCookedStatuses(t *testing.T) {
	cooked := []string{"processing", "pending", "Pendiente de pago", "Procesando", "completed", ""}
	for _, status := range cooked {
		if !orderIsCooked(status) {
			t.Errorf("%q should be cooked", status)
		}
	}
	inactive := []string{"cancelled", "canceled", "Cancelado", "wc-cancelled", "refunded", "reembolsado", "failed", "trash", "trashed", "deleted", "on-hold", "En espera", "draft"}
	for _, status := range inactive {
		if orderIsCooked(status) {
			t.Errorf("%q should not be cooked", status)
		}
	}
}

func TestExportRowStatusReadsTolerantColumns(t *testing.T) {
	for _, key := range []string{"Estado", "estado del pedido", "Order Status", "status", "Estatus"} {
		record := map[string]any{key: "cancelled"}
		if got := exportRowStatus(record); got != "cancelled" {
			t.Fatalf("key %q = %q, want cancelled", key, got)
		}
	}
	if got := exportRowStatus(map[string]any{"Productos": "Box Hombre X 1"}); got != "" {
		t.Fatalf("without a status column = %q, want empty", got)
	}
}
