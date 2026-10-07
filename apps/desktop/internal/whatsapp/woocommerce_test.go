package whatsapp

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// webhookRequest builds the native WooCommerce payload for one order.
func webhookRequest(t testing.TB, number, status, date string) *http.Request {
	t.Helper()
	payload := map[string]any{
		"number": number,
		"status": status,
		"meta_data": []map[string]any{
			{"key": "Seleccionar una Fecha", "value": date},
		},
		"line_items": []any{},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewRequest(http.MethodPost, "/hook/woocommerce", strings.NewReader(string(body)))
}

// TestPendingWebhookIsCooked pins the status rule on the webhook: a pending
// order enters the day and notifies, it is not treated as a removal.
func TestPendingWebhookIsCooked(t *testing.T) {
	c := testClient(t)
	notified := 0
	c.cfg.Dispatcher = func(_, _, _ string) int {
		notified++
		return 1
	}
	date := bogotaDate(0)
	recorder := httptest.NewRecorder()
	c.handleWooCommerce(recorder, webhookRequest(t, "50", "pending", spanishDate(date)))
	if _, ok := c.kitchen.state.Orders[date]["50"]; !ok {
		t.Fatalf("pending order was not cooked: %v", c.kitchen.state.Orders[date])
	}
	if notified != 1 {
		t.Fatalf("notices = %d, want 1", notified)
	}
}
