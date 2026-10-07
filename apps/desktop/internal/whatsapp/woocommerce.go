package whatsapp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/engine"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/labels"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/selftest"
)

// woocommerceOrder is the subset of the WooCommerce order payload the bot uses.
type woocommerceOrder struct {
	Number    string        `json:"number"`
	ID        int64         `json:"id"`
	Status    string        `json:"status"`
	MetaData  []wooMeta     `json:"meta_data"`
	LineItems []wooLineItem `json:"line_items"`
}

type wooMeta struct {
	Key   string `json:"key"`
	Value any    `json:"value"`
}

type wooLineItem struct {
	ProductID   int64     `json:"product_id"`
	VariationID int64     `json:"variation_id"`
	Quantity    int       `json:"quantity"`
	Name        string    `json:"name"`
	MetaData    []wooMeta `json:"meta_data"`
}

// handleWooCommerce receives a native WooCommerce webhook (order.created /
// order.updated), rebuilds the wide-export row and applies it as an order.
func (c *Client) handleWooCommerce(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeJSON(w, http.StatusMethodNotAllowed, map[string]any{"ok": false, "error": "Method not allowed"})
		return
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Could not read body"})
		return
	}
	if c.cfg.WebhookSecret != "" && !verifyWooSignature(body, r.Header.Get("x-wc-webhook-signature"), c.cfg.WebhookSecret) {
		writeJSON(w, http.StatusUnauthorized, map[string]any{"ok": false, "error": "Invalid signature"})
		return
	}

	var order woocommerceOrder
	if err := json.Unmarshal(body, &order); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Invalid order JSON"})
		return
	}
	row := wooRow(&order)
	export := engine.ParseWideOrderRows([]map[string]any{row}, engine.WideExportOptions{})
	if export.DeliveryDate == "" {
		writeJSON(w, http.StatusBadRequest, map[string]any{"ok": false, "error": "Order has no delivery date"})
		return
	}
	number := order.Number
	if number == "" {
		number = row["ID orden"].(string)
	}
	// Self-test deliveries (Ajustes → Avanzado) stop here: signature, JSON and
	// delivery date are proven, and the kitchen list stays untouched.
	if r.Header.Get(selftest.Header) == "1" {
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "dryRun": true, "deliveryDate": export.DeliveryDate, "lines": len(export.Lines)})
		return
	}

	// Only inactive orders are left out: pending orders are cooked too, and
	// the rule lives in orderIsCooked so the export, the lookup and the
	// webhook all agree.
	status := strings.ToLower(strings.TrimSpace(order.Status))
	if !orderIsCooked(status) {
		title := statusTitle(status) + " #" + number
		result := c.applyRemoval(export.DeliveryDate, number, title)
		result["status"] = status
		writeJSON(w, http.StatusOK, result)
		return
	}

	topic := r.Header.Get("x-wc-webhook-topic")
	kind := eventNew
	title := labels.Bot.NewOrder
	if strings.Contains(topic, "updated") || strings.Contains(topic, "deleted") {
		kind = eventUpdate
		title = labels.Bot.UpdatedOrder
	}
	result := c.applyOrder(export.DeliveryDate, number, export.Lines, title+" #"+number, wooObservation(&order), export.Annotations[number], kind)
	result["lines"] = len(export.Lines)
	writeJSON(w, http.StatusOK, result)
}

func statusTitle(status string) string {
	switch status {
	case "trash", "deleted":
		return labels.Bot.Deleted
	case "cancelled", "canceled":
		return labels.Bot.Cancelled
	default:
		return labels.Bot.UpdatedOrder
	}
}

func wooObservation(order *woocommerceOrder) string {
	for _, item := range order.MetaData {
		if item.Key == "Observaciones" || item.Key == "observaciones" {
			if value, ok := item.Value.(string); ok {
				return strings.TrimSpace(value)
			}
		}
	}
	return ""
}

// wooRow rebuilds the wide export row the engine understands from a
// WooCommerce order: the excel text fields plus the choice metas.
func wooRow(order *woocommerceOrder) map[string]any {
	row := map[string]any{"ID orden": order.Number}

	meta := map[string]any{}
	for _, item := range order.MetaData {
		meta[item.Key] = item.Value
	}
	if value, ok := meta["Seleccionar una Fecha"]; ok {
		row["Fecha de Entrega"] = value
	} else if value, ok := meta["_orddd_delivery_date"]; ok {
		row["Fecha de Entrega"] = value
	}
	if value, ok := meta["desayuno_excel"]; ok {
		row["Productos"] = stringOfAny(value)
	}
	if value, ok := meta["adicionales_excel"]; ok {
		row["Adicionales"] = stringOfAny(value)
	}
	if value, ok := meta["color"]; ok {
		row["color"] = stringOfAny(value)
	}
	if value, ok := meta["motivo"]; ok {
		row["motivo"] = stringOfAny(value)
	}

	// Choices live on the line items. A chosen drink suppresses the export's
	// juice fallback and adds the real drink option; food choices and the
	// desayuno_* flags add their option column.
	choices := []wooMeta{}
	choices = append(choices, order.MetaData...)
	for _, item := range order.LineItems {
		choices = append(choices, item.MetaData...)
	}
	for _, item := range choices {
		switch {
		case item.Key == "pa_elige-la-bebida":
			row["Jugo de Naranja"] = 1
			if value, ok := item.Value.(string); ok && value != "" {
				row[value] = 1
			}
		case item.Key == "pa_elige-el-motivo":
			// Personalization, not a kitchen choice: kept as the order's
			// motivo annotation.
			if value, ok := item.Value.(string); ok && value != "" {
				if existing, _ := row["motivo"].(string); existing == "" {
					row["motivo"] = value
				}
			}
		case item.Key == "pa_globos":
			// The chosen balloon color, kept as the order's color annotation.
			if value, ok := item.Value.(string); ok && value != "" {
				if existing, _ := row["color"].(string); existing == "" {
					row["color"] = value
				}
			}
		case item.Key == "pa_elige-los-globos":
			// Personalization, not a kitchen choice.
		case strings.HasPrefix(item.Key, "pa_elige-"):
			if value, ok := item.Value.(string); ok && value != "" {
				row[value] = 1
			}
		case strings.HasPrefix(item.Key, "desayuno_") && item.Key != "desayuno_excel":
			if engine.IsPositive(item.Value) {
				row[strings.TrimPrefix(item.Key, "desayuno_")] = 1
			}
		}
	}
	return row
}

func verifyWooSignature(body []byte, signature, secret string) bool {
	if signature == "" {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := base64.StdEncoding.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(expected), []byte(signature))
}

func stringOfAny(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case nil:
		return ""
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return ""
		}
		return string(raw)
	}
}
