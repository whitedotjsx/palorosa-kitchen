package whatsapp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The panel client must ingest orders into the shared kitchen with no WhatsApp
// account at all: viewing and filling the kitchen cannot depend on the bot.
func TestPanelClientIngestsHooksWithoutAccount(t *testing.T) {
	kitchen := NewKitchen(t.TempDir(), "../../../../data/seed.json", nil)
	if kitchen.Index() == nil {
		t.Skip("seed catalog not available")
	}
	client := NewPanelClient(kitchen, nil, nil)

	body := `{"deliveryDate":"2026-10-04","orderNumber":"7","lines":[{"productText":"Box Mujer","quantity":1,"source":"breakfast"}]}`
	request := httptest.NewRequest(http.MethodPost, "/hook/order", strings.NewReader(body))
	recorder := httptest.NewRecorder()
	client.Handler().ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("hook status = %d, body %s", recorder.Code, recorder.Body)
	}
	if _, ok := kitchen.state.Orders["2026-10-04"]["7"]; !ok {
		t.Fatal("the order was not stored in the shared kitchen")
	}

	// The panel snapshot exposes it without any account running.
	snapshot := client.PanelSnapshot()
	if len(snapshot.Days) != 1 || snapshot.Days[0].Orders[0].Number != "7" {
		t.Fatalf("snapshot days = %+v", snapshot.Days)
	}
	if snapshot.WhatsApp.Status != "disabled" {
		t.Fatalf("whatsapp status = %q", snapshot.WhatsApp.Status)
	}
}
