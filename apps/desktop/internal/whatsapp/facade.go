package whatsapp

import (
	"context"
	"log"
	"net/http"

	waLog "go.mau.fi/whatsmeow/util/log"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/panelmodel"
)

// PanelOps is the kitchen-facing surface the panel needs without a WhatsApp
// connection: reads, order ingestion, order removal, notification settings,
// list publishing and the store sync. The bots manager implements it and keeps
// working while every account is off, unlinked or disconnected.
type PanelOps interface {
	PanelSnapshot() panelmodel.Snapshot
	OrderDetail(date, number string) (panelmodel.OrderDetail, error)
	RemoveOrder(date, number string) error
	SetNotifications(notifications panelmodel.Notifications)
	PublishList(date string) error
	SyncFromExport(ctx context.Context, fetch ExportRows, dates []string) (SyncResult, error)
	Handler() http.Handler
}

// NewPanelClient builds a Client that only serves the panel from the shared
// kitchen: it owns no WhatsApp session and never connects. Kitchen reads and
// edits work while no account is running; everything that needs WhatsApp
// (commands, pairing, sending) stays unavailable.
func NewPanelClient(kitchen *Kitchen, dispatcher func(text, kind, date string) int, onOrder func()) *Client {
	return &Client{
		cfg:     Config{Dispatcher: dispatcher, OnOrder: onOrder},
		log:     waLog.Stdout("Panel", "WARN", true),
		logger:  log.Default(),
		kitchen: kitchen,
	}
}
