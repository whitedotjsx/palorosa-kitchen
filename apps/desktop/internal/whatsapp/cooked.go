package whatsapp

import (
	"strings"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/engine"
)

// orderIsCooked reports whether an order with this status belongs in the
// kitchen list. Only the inactive statuses are excluded: `pending` is cooked
// (the store's bank-transfer orders are prepared before the payment clears)
// and an unrecognized status is kept, so a status the parser does not know
// (a translated label, a new WooCommerce state) never wipes the day.
func orderIsCooked(status string) bool {
	normalized := engine.NormalizeName(status)
	normalized = strings.TrimPrefix(normalized, "wc ")
	switch normalized {
	case "cancelled", "canceled", "cancelado", "cancelada",
		"refunded", "reembolsado", "reembolsada",
		"failed", "fallido", "fallida",
		"trash", "trashed", "deleted", "eliminado", "eliminada", "papelera",
		"on hold", "onhold", "en espera", "espera",
		"draft", "auto draft":
		return false
	}
	return true
}

// exportStatusKeys are the accepted header names of the status column the
// operator adds to the saved WP All Export. The comparison goes through
// NormalizeName, so spelling, case and accents do not matter.
var exportStatusKeys = []string{
	"estado",
	"estado del pedido",
	"estatus",
	"status",
	"order status",
	"status del pedido",
}

// exportRowStatus reads the status cell of one export record, or "" when the
// export has no status column (an export configured before the column was
// added: every row is kept, like before).
func exportRowStatus(record map[string]any) string {
	for key, value := range record {
		normalized := engine.NormalizeName(key)
		for _, candidate := range exportStatusKeys {
			if normalized == candidate {
				return strings.TrimSpace(stringOfAny(value))
			}
		}
	}
	return ""
}
