// Package whatsapp runs the kitchen WhatsApp bot on top of whatsmeow: pairing
// with a QR, an allowlist, the list commands and message sending. It replaces
// the Node whatsapp-web.js bot (no headless browser).
package whatsapp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	_ "time/tzdata"

	_ "github.com/mattn/go-sqlite3"
	"go.mau.fi/whatsmeow"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"go.mau.fi/whatsmeow/store/sqlstore"
	"go.mau.fi/whatsmeow/types"
	"go.mau.fi/whatsmeow/types/events"
	waLog "go.mau.fi/whatsmeow/util/log"
	"google.golang.org/protobuf/proto"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/engine"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/labels"
)

// Status is the WhatsApp connection state.
type Status int

const (
	// StatusDisabled means the bot is turned off by configuration.
	StatusDisabled Status = iota
	// StatusUnlinked means there is no paired session yet.
	StatusUnlinked
	// StatusConnecting means a session exists and the client is connecting.
	StatusConnecting
	// StatusConnected means the client is online.
	StatusConnected
	// StatusDisconnected means the connection dropped.
	StatusDisconnected
)

// Label returns the Spanish status text for the tray.
func (s Status) Label() string {
	switch s {
	case StatusConnected:
		return labels.WhatsApp.Connected
	case StatusConnecting:
		return labels.WhatsApp.Connecting
	case StatusDisconnected:
		return labels.WhatsApp.Disconnected
	case StatusDisabled:
		return labels.WhatsApp.Disabled
	default:
		return labels.WhatsApp.Unlinked
	}
}

// Config is the bot configuration and its event callbacks.
type Config struct {
	DataDir      string
	CatalogPath  string
	Allowlist    []string
	PairingPhone string
	HookPort     int
	HookToken    string

	WebhookSecret string
	Debug         bool

	OnStatus func(Status)
	OnQR     func(page string)
	OnLinked func()
	// OnOrder runs after an order hook or a removal changed the kitchen state,
	// so the panel can push a live update.
	OnOrder func()
	// Dispatcher routes notifications through the panel's targets. Nil keeps the
	// legacy allowlist fan-out.
	Dispatcher func(text, kind, date string) int
	// Kitchen is the shared kitchen state. Nil creates a private one from
	// DataDir and CatalogPath, so a single bot keeps working unchanged.
	Kitchen *Kitchen
}

// Client wraps the whatsmeow client with the kitchen bot behaviour.
type Client struct {
	cfg     Config
	log     waLog.Logger
	cli     *whatsmeow.Client
	kitchen *Kitchen

	mu             sync.Mutex
	status         Status
	qrCode         string
	pairCode       string
	pairRequested  bool
	pairing        bool
	pairingExpired bool
	ctx            context.Context
}

// New opens (or creates) the session store and builds the client. It does not
// connect; call Start for that.
func New(cfg Config) (*Client, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o755); err != nil {
		return nil, err
	}
	level := "WARN"
	if cfg.Debug {
		level = "DEBUG"
	}
	log := waLog.Stdout("WhatsApp", level, true)

	dbPath := filepath.ToSlash(filepath.Join(cfg.DataDir, "whatsapp.db"))
	dsn := "file:" + dbPath + "?_foreign_keys=on&_busy_timeout=5000"
	container, err := sqlstore.New(context.Background(), "sqlite3", dsn, log)
	if err != nil {
		return nil, err
	}
	device, err := container.GetFirstDevice(context.Background())
	if err != nil {
		return nil, err
	}

	kitchen := cfg.Kitchen
	if kitchen == nil {
		kitchen = NewKitchen(cfg.DataDir, cfg.CatalogPath, log)
	}
	client := &Client{
		cfg:     cfg,
		log:     log,
		cli:     whatsmeow.NewClient(device, log),
		status:  StatusUnlinked,
		kitchen: kitchen,
	}
	client.cli.AddEventHandler(client.handleEvent)
	return client, nil
}

// Start connects. With a stored session it just connects; without one it runs
// a single QR cycle and stops, leaving a Retry button for the operator.
func (c *Client) Start(ctx context.Context) error {
	c.mu.Lock()
	c.ctx = ctx
	c.mu.Unlock()
	if c.cli.Store.ID == nil {
		c.runPairingCycle(ctx)
		return nil
	}
	c.setStatus(StatusConnecting)
	return c.cli.Connect()
}

// RetryPairing starts a fresh QR cycle after the previous one expired. It is
// the callback behind the pairing page Retry button.
func (c *Client) RetryPairing() {
	if c.cli.Store.ID != nil {
		return
	}
	c.mu.Lock()
	if c.pairing {
		c.mu.Unlock()
		return
	}
	c.pairing = true
	ctx := c.ctx
	c.mu.Unlock()
	if ctx == nil {
		ctx = context.Background()
	}

	go func() {
		defer func() {
			c.mu.Lock()
			c.pairing = false
			c.mu.Unlock()
		}()
		c.runPairingCycle(ctx)
	}()
}

// runPairingCycle runs one QR channel (WhatsApp emits a handful of rotating
// codes) and, when it times out unpaired, shows the expired page. It does not
// loop: the operator taps Retry to get a new set.
func (c *Client) runPairingCycle(ctx context.Context) {
	c.setStatus(StatusUnlinked)
	c.resetPairing()

	qrChan, err := c.cli.GetQRChannel(ctx)
	if err != nil {
		c.log.Errorf("QR channel failed: %v", err)
		c.expirePairing()
		return
	}
	if err := c.cli.Connect(); err != nil {
		c.log.Errorf("Connect failed: %v", err)
		c.expirePairing()
		return
	}

	c.consumeQR(ctx, qrChan)
	if c.cli.Store.ID != nil || c.cli.IsConnected() {
		return
	}
	c.cli.Disconnect()
	c.log.Infof("QR codes expired, waiting for the operator to retry")
	c.expirePairing()
}

func (c *Client) resetPairing() {
	c.mu.Lock()
	c.qrCode = ""
	c.pairCode = ""
	c.pairRequested = false
	c.pairingExpired = false
	c.mu.Unlock()
}

func (c *Client) expirePairing() {
	c.mu.Lock()
	c.pairingExpired = true
	c.qrCode = ""
	c.pairCode = ""
	page := c.pairingPageLocked()
	c.mu.Unlock()
	c.emitQR(page)
}

// Close disconnects without logging out.
func (c *Client) Close() {
	c.cli.Disconnect()
}

// Logout unlinks the WhatsApp session and returns the account to the pairing
// state so it can be linked again with a fresh QR.
func (c *Client) Logout() error {
	if err := c.cli.Logout(context.Background()); err != nil {
		return err
	}
	c.cli.Store.ID = nil
	c.setStatus(StatusUnlinked)
	return nil
}

// PublishList recomputes and stores the kitchen list for a date on demand, so
// the operator can publish it from the panel.
func (c *Client) PublishList(date string) error {
	if c.kitchen.Index() == nil {
		return fmt.Errorf("catalog not loaded")
	}
	c.kitchen.mu.Lock()
	list := c.listFromStateLocked(date)
	if list != nil {
		c.kitchen.state.Lists[date] = *list
	} else {
		delete(c.kitchen.state.Lists, date)
	}
	err := saveBotState(c.kitchen.path, c.kitchen.state)
	c.kitchen.mu.Unlock()
	c.changed()
	return err
}

// Status returns the current connection state.
func (c *Client) Status() Status {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.status
}

// Linked reports whether a paired session is stored.
func (c *Client) Linked() bool {
	return c.cli.Store.ID != nil
}

// Phone returns the linked account's own number, or "" when it is not linked.
func (c *Client) Phone() string {
	if c.cli.Store.ID == nil {
		return ""
	}
	return c.cli.Store.ID.User
}

func (c *Client) consumeQR(ctx context.Context, channel <-chan whatsmeow.QRChannelItem) {
	for item := range channel {
		switch item.Event {
		case whatsmeow.QRChannelEventCode:
			c.setQR(item.Code)
			c.requestPairingCode(ctx)
		case whatsmeow.QRChannelEventError:
			if item.Error != nil {
				c.log.Errorf("Pairing error: %v", item.Error)
			}
		}
	}
}

func (c *Client) requestPairingCode(ctx context.Context) {
	if c.cfg.PairingPhone == "" {
		return
	}
	c.mu.Lock()
	if c.pairRequested {
		c.mu.Unlock()
		return
	}
	c.pairRequested = true
	c.mu.Unlock()

	go func() {
		code, err := c.cli.PairPhone(ctx, c.cfg.PairingPhone, true, whatsmeow.PairClientChrome, "Palorosa Kitchen")
		if err != nil {
			c.log.Errorf("Pairing code failed: %v", err)
			return
		}
		c.mu.Lock()
		c.pairCode = code
		page := c.pairingPageLocked()
		c.mu.Unlock()
		c.emitQR(page)
	}()
}

func (c *Client) handleEvent(raw any) {
	switch event := raw.(type) {
	case *events.Connected:
		c.setStatus(StatusConnected)
		if c.cfg.OnLinked != nil {
			c.cfg.OnLinked()
		}
	case *events.Disconnected:
		c.setStatus(StatusDisconnected)
	case *events.LoggedOut:
		c.setStatus(StatusUnlinked)
	case *events.Message:
		go c.handleMessage(event)
	}
}

func (c *Client) handleMessage(event *events.Message) {
	if event.Info.IsFromMe || event.Info.IsGroup {
		return
	}
	number := phoneNumber(event.Info.Sender, event.Info.SenderAlt, event.Info.Chat)
	if number == "" || !c.allowed(number) {
		c.log.Infof("Ignored message from %s (sender %s)", number, event.Info.Sender)
		return
	}
	c.log.Infof("Command from %s: %q", number, messageText(event.Message))
	reply := c.reply(context.Background(), messageText(event.Message))
	if reply == "" {
		return
	}
	if _, err := c.send(context.Background(), replyTarget(event.Info), reply); err != nil {
		c.log.Errorf("Reply failed: %v", err)
	}
}

// phoneNumber returns the phone-number form of the first JID that carries one.
// WhatsApp multi-device may address a message by LID instead of the number.
func phoneNumber(jids ...types.JID) string {
	for _, jid := range jids {
		if jid.Server == types.DefaultUserServer && jid.User != "" {
			return jid.User
		}
	}
	for _, jid := range jids {
		if jid.User != "" {
			return jid.User
		}
	}
	return ""
}

// replyTarget picks the phone-addressed JID to answer to, falling back to the
// chat as received.
func replyTarget(info types.MessageInfo) types.JID {
	for _, jid := range []types.JID{info.SenderAlt, info.Sender, info.Chat} {
		if jid.Server == types.DefaultUserServer && jid.User != "" {
			return jid
		}
	}
	return info.Chat
}

const (
	eventNew    = "new"
	eventUpdate = "update"
)

func (c *Client) reply(ctx context.Context, text string) string {
	normalized := normalizeCommand(text)
	fields := strings.Fields(normalized)
	if len(fields) > 0 && (fields[0] == "notificaciones" || fields[0] == "avisos") {
		return c.notificationCommand(fields)
	}
	switch normalized {
	case "lista", "lista hoy", "lista de hoy":
		return c.listReply(ctx, bogotaDate(0))
	case "lista manana", "lista de manana":
		return c.listReply(ctx, bogotaDate(1))
	case "estado":
		return c.statusReply(ctx)
	case "listo":
		c.saveCheckpoint()
		return labels.Bot.ReadySaved
	case "nuevo":
		return c.newSinceCheckpoint()
	case "ayuda", "comandos", "menu", "opciones":
		return labels.Bot.Commands
	case "hola", "buenas", "buenos dias", "buen dia":
		return labels.Bot.Greeting
	case "activar notificaciones":
		c.setNotification("enabled", true)
		return labels.Bot.NotifOn
	case "desactivar notificaciones":
		c.setNotification("enabled", false)
		return labels.Bot.NotifOff
	}

	// "lista <fecha>": the date is read from the raw text so the separators are
	// not lost to the command normalizer.
	if match := listWithDatePattern.FindStringSubmatch(text); match != nil {
		if date, ok := parseCommandDate(match[1]); ok {
			return c.listReply(ctx, date)
		}
		return labels.Bot.BadDate
	}
	if normalized == "" {
		return ""
	}
	return labels.Bot.Unknown
}

// whenLabel is "HOY" or "MAÑANA" for those dates, empty otherwise.
func whenLabel(date string) string {
	switch date {
	case bogotaDate(0):
		return labels.Bot.WhenToday
	case bogotaDate(1):
		return labels.Bot.WhenTomorrow
	default:
		return ""
	}
}

// saveCheckpoint freezes the current orders and lists (the LISTO command).
func (c *Client) saveCheckpoint() {
	c.kitchen.mu.Lock()
	c.kitchen.state.Checkpoint = &checkpointState{
		Orders: cloneOrders(c.kitchen.state.Orders),
		Lists:  cloneLists(c.kitchen.state.Lists),
	}
	_ = saveBotState(c.kitchen.path, c.kitchen.state)
	c.kitchen.mu.Unlock()
}

// newSinceCheckpoint lists what arrived after the last LISTO: the new or
// changed orders (their breakfasts) and the new food units, numbered in one
// single list (the NUEVO command).
func (c *Client) newSinceCheckpoint() string {
	c.kitchen.mu.Lock()
	defer c.kitchen.mu.Unlock()
	checkpoint := c.kitchen.state.Checkpoint
	if checkpoint == nil {
		return labels.Bot.NoCheckpoint
	}

	var items []string
	for _, date := range sortedKeys(c.kitchen.state.Orders) {
		for _, number := range sortedKeys(c.kitchen.state.Orders[date]) {
			lines := c.kitchen.state.Orders[date][number]
			if reflect.DeepEqual(checkpoint.Orders[date][number], lines) {
				continue
			}
			label := orderText(lines)
			if when := whenLabel(date); when != "" {
				label = when + ": " + label
			} else {
				label = engine.HumanDate(date) + ": " + label
			}
			items = append(items, label)
		}
	}

	dates := map[string]bool{}
	for date := range c.kitchen.state.Lists {
		dates[date] = true
	}
	for date := range checkpoint.Lists {
		dates[date] = true
	}
	for _, date := range sortedKeys(dates) {
		previous := checkpoint.Lists[date]
		diff := engine.DiffLists(&previous, c.kitchen.state.Lists[date])
		for _, entry := range diff.Added {
			items = append(items, fmt.Sprintf("%d × %s", entry.Next, entry.Name))
		}
		for _, entry := range diff.Changed {
			if entry.Next > entry.Previous {
				items = append(items, fmt.Sprintf("%d × %s", entry.Next, entry.Name))
			}
		}
	}

	if len(items) == 0 {
		return labels.Bot.NothingNew
	}
	var builder strings.Builder
	builder.WriteString(labels.Bot.NewSinceTitle)
	for index, item := range items {
		fmt.Fprintf(&builder, "\n%d. %s", index+1, item)
	}
	return builder.String()
}

func sortedKeys[M ~map[string]V, V any](m M) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}

func (c *Client) notificationCommand(fields []string) string {
	switch len(fields) {
	case 1:
		return c.notificationSummary()
	case 2:
		if enabled, ok := notifValue(fields[1]); ok {
			c.setNotification("enabled", enabled)
			return onOffMessage(enabled)
		}
		return labels.Bot.BadOption
	case 3:
		enabled, ok := notifValue(fields[2])
		if !ok {
			return labels.Bot.BadOption
		}
		name := ""
		switch fields[1] {
		case "nuevos", "nuevo":
			c.setNotification("new", enabled)
			name = labels.Bot.NameNew
		case "cambios", "cambio", "actualizados":
			c.setNotification("updated", enabled)
			name = labels.Bot.NameUpdated
		case "hoy":
			c.setNotification("today", enabled)
			name = labels.Bot.NameToday
		case "manana":
			c.setNotification("tomorrow", enabled)
			name = labels.Bot.NameTomorrow
		default:
			return labels.Bot.BadOption
		}
		return strings.NewReplacer("{name}", name, "{value}", onOffWord(enabled)).Replace(labels.Bot.NotifSet)
	default:
		return labels.Bot.BadOption
	}
}

func onOffMessage(enabled bool) string {
	if enabled {
		return labels.Bot.NotifOn
	}
	return labels.Bot.NotifOff
}

func (c *Client) notificationSummary() string {
	c.kitchen.mu.Lock()
	settings := c.kitchen.state.Notifications
	c.kitchen.mu.Unlock()

	global := strings.ReplaceAll(labels.Bot.NotifHeader, "{state}", onOffWord(settings == nil || settings.Enabled == nil || *settings.Enabled))
	var nuevos, cambios, hoy, manana *bool
	if settings != nil {
		nuevos, cambios, hoy, manana = settings.New, settings.Updated, settings.Today, settings.Tomorrow
	}
	lines := []string{
		global,
		notificationLine(labels.Bot.NameNew, nuevos),
		notificationLine(labels.Bot.NameUpdated, cambios),
		notificationLine(labels.Bot.NameToday, hoy),
		notificationLine(labels.Bot.NameTomorrow, manana),
	}
	return strings.Join(lines, "\n")
}

func notificationLine(name string, value *bool) string {
	return strings.NewReplacer("{name}", name, "{value}", onOffWord(value == nil || *value)).Replace(labels.Bot.NotifLine)
}

func onOffWord(enabled bool) string {
	if enabled {
		return labels.Bot.OnWord
	}
	return labels.Bot.OffWord
}

// notifValue maps a Spanish token to a boolean: activar/encender/sí vs
// desactivar/apagar/no.
func notifValue(value string) (bool, bool) {
	switch value {
	case "activar", "encender", "encendidos", "encenderlos", "si":
		return true, true
	case "desactivar", "apagar", "apagados", "apagarlos", "no":
		return false, true
	default:
		return false, false
	}
}

var listWithDatePattern = regexp.MustCompile(`(?i)^\s*lista\s+(.+)$`)

var (
	isoDatePattern    = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})$`)
	slashDatePattern  = regexp.MustCompile(`^(\d{1,2})[/-](\d{1,2})(?:[/-](\d{2,4}))?$`)
	wordDatePattern   = regexp.MustCompile(`^(\d{1,2})\s*(?:de\s+)?([a-z]+)(?:\s*(?:de\s+)?(\d{4}))?$`)
	spanishMonthNames = map[string]string{
		"enero": "01", "febrero": "02", "marzo": "03", "abril": "04",
		"mayo": "05", "junio": "06", "julio": "07", "agosto": "08",
		"septiembre": "09", "setiembre": "09", "octubre": "10",
		"noviembre": "11", "diciembre": "12",
	}
)

// parseCommandDate accepts "2026-10-05", "5/10", "5/10/2026", "5 octubre" or
// "5 de octubre de 2026".
func parseCommandDate(value string) (string, bool) {
	text := accentReplacer.Replace(strings.ToLower(strings.TrimSpace(value)))
	if match := isoDatePattern.FindStringSubmatch(text); match != nil {
		return match[1] + "-" + match[2] + "-" + match[3], true
	}
	if match := slashDatePattern.FindStringSubmatch(text); match != nil {
		year := match[3]
		if year == "" {
			year = bogotaDate(0)[:4]
		} else if len(year) == 2 {
			year = "20" + year
		}
		return year + "-" + pad2(match[2]) + "-" + pad2(match[1]), true
	}
	if match := wordDatePattern.FindStringSubmatch(text); match != nil {
		month, ok := spanishMonthNames[match[2]]
		if !ok {
			return "", false
		}
		year := match[3]
		if year == "" {
			year = bogotaDate(0)[:4]
		}
		return year + "-" + month + "-" + pad2(match[1]), true
	}
	return "", false
}

func pad2(value string) string {
	if len(value) == 1 {
		return "0" + value
	}
	return value
}

func (c *Client) listReply(ctx context.Context, date string) string {
	list := c.listForDate(ctx, date)
	if list == nil || len(list.Entries) == 0 {
		return strings.ReplaceAll(labels.Bot.NoList, "{date}", engine.HumanDate(date))
	}
	return engine.FormatListText(*list, date, whenLabel(date))
}

// listForDate returns the list for a date computed from the orders received
// through the hooks (or published from the panel).
func (c *Client) listForDate(_ context.Context, date string) *engine.KitchenList {
	return c.listFromState(date)
}

func (c *Client) listFromState(date string) *engine.KitchenList {
	if c.kitchen.Index() == nil {
		return nil
	}
	c.kitchen.mu.Lock()
	defer c.kitchen.mu.Unlock()
	return c.listFromStateLocked(date)
}

func (c *Client) listFromStateLocked(date string) *engine.KitchenList {
	orders := c.kitchen.state.Orders[date]
	if len(orders) == 0 {
		return nil
	}
	var lines []engine.ParsedOrderLine
	for _, orderLines := range orders {
		lines = append(lines, orderLines...)
	}
	index := c.kitchen.Index()
	if len(lines) == 0 || index == nil {
		return nil
	}
	list := engine.AggregateUnits(engine.ResolveLines(lines, index), index)
	return &list
}

func (c *Client) statusReply(ctx context.Context) string {
	today := engine.HumanDate(bogotaDate(0))
	dates := c.knownDates(ctx)
	if len(dates) == 0 {
		return strings.ReplaceAll(labels.Bot.StatusNone, "{date}", today)
	}
	lines := make([]string, 0, len(dates))
	for _, date := range dates {
		lines = append(lines, "- "+engine.HumanDate(date))
	}
	reply := strings.ReplaceAll(labels.Bot.StatusReady, "{date}", today)
	return strings.ReplaceAll(reply, "{dates}", strings.Join(lines, "\n"))
}

func (c *Client) knownDates(_ context.Context) []string {
	seen := map[string]bool{}
	c.kitchen.mu.Lock()
	for date := range c.kitchen.state.Lists {
		seen[date] = true
	}
	c.kitchen.mu.Unlock()

	dates := make([]string, 0, len(seen))
	for date := range seen {
		dates = append(dates, date)
	}
	sort.Strings(dates)
	return dates
}

func (c *Client) send(ctx context.Context, to types.JID, text string) (whatsmeow.SendResponse, error) {
	return c.cli.SendMessage(ctx, to, &waE2E.Message{Conversation: proto.String(text)})
}

// SendTo sends a text to a phone number. The notification dispatcher uses it to
// reach a target through this account.
func (c *Client) SendTo(phone, text string) error {
	if !c.cli.IsConnected() {
		return fmt.Errorf("whatsapp no conectado")
	}
	_, err := c.send(context.Background(), types.NewJID(phone, types.DefaultUserServer), text)
	return err
}

// notifyDiff sends the change text (whole-day replace) to the allowlist.
func (c *Client) notifyDiff(diff engine.ListDiff, title, note, date, kind string) int {
	if engine.IsListDiffEmpty(diff) {
		return 0
	}
	text := engine.FormatDiffText(diff, title, note, date, whenLabel(date))
	return c.sendNotification(text, kind, date)
}

// sendNotification applies the settings and the today/tomorrow window, then
// sends the text to every allowlisted number.
func (c *Client) sendNotification(text, kind, date string) int {
	if !c.shouldNotify(kind, date) {
		c.log.Infof("Notification skipped (%s, %s) by settings", kind, date)
		return 0
	}
	if c.cfg.Dispatcher != nil {
		return c.cfg.Dispatcher(text, kind, date)
	}
	if !c.cli.IsConnected() {
		c.log.Warnf("WhatsApp is not connected, notification skipped")
		return 0
	}
	sent := 0
	for _, number := range c.cfg.Allowlist {
		to := types.NewJID(number, types.DefaultUserServer)
		if _, err := c.send(context.Background(), to, text); err != nil {
			c.log.Errorf("Notification to %s failed: %v", number, err)
			continue
		}
		sent++
	}
	return sent
}

// notifiableDate reports whether a delivery date is worth notifying: today or
// tomorrow (America/Bogota). Orders for other days are ignored.
func (c *Client) notifiableDate(date string) bool {
	return date == bogotaDate(0) || date == bogotaDate(1)
}

// shouldNotify combines the today/tomorrow window with the granular settings.
func (c *Client) shouldNotify(kind, date string) bool {
	if !c.notifiableDate(date) {
		return false
	}
	c.kitchen.mu.Lock()
	settings := c.kitchen.state.Notifications
	c.kitchen.mu.Unlock()
	if settings == nil {
		return true
	}
	if settings.Enabled != nil && !*settings.Enabled {
		return false
	}
	if kind == eventNew && settings.New != nil && !*settings.New {
		return false
	}
	if kind == eventUpdate && settings.Updated != nil && !*settings.Updated {
		return false
	}
	if date == bogotaDate(0) && settings.Today != nil && !*settings.Today {
		return false
	}
	if date == bogotaDate(1) && settings.Tomorrow != nil && !*settings.Tomorrow {
		return false
	}
	return true
}

func (c *Client) notificationValue(value *bool) string {
	if value != nil && !*value {
		return labels.Bot.OffWord
	}
	return labels.Bot.OnWord
}

func (c *Client) setNotification(target string, enabled bool) {
	c.kitchen.mu.Lock()
	if c.kitchen.state.Notifications == nil {
		c.kitchen.state.Notifications = &notificationSettings{}
	}
	settings := c.kitchen.state.Notifications
	switch target {
	case "enabled":
		settings.Enabled = &enabled
	case "new":
		settings.New = &enabled
	case "updated":
		settings.Updated = &enabled
	case "today":
		settings.Today = &enabled
	case "tomorrow":
		settings.Tomorrow = &enabled
	}
	_ = saveBotState(c.kitchen.path, c.kitchen.state)
	c.kitchen.mu.Unlock()
}

func (c *Client) allowed(user string) bool {
	for _, number := range c.cfg.Allowlist {
		if number == user {
			return true
		}
	}
	return false
}

func (c *Client) setStatus(status Status) {
	c.mu.Lock()
	c.status = status
	c.mu.Unlock()
	if c.cfg.OnStatus != nil {
		c.cfg.OnStatus(status)
	}
}

func (c *Client) setQR(code string) {
	c.mu.Lock()
	c.qrCode = code
	page := c.pairingPageLocked()
	c.mu.Unlock()
	c.emitQR(page)
}

// PairingPage renders the current pairing page (QR and status) as a data URI.
func (c *Client) PairingPage() string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.pairingPageLocked()
}

func (c *Client) emitQR(page string) {
	if c.cfg.OnQR != nil {
		c.cfg.OnQR(page)
	}
}

func messageText(message *waE2E.Message) string {
	if message == nil {
		return ""
	}
	if text := message.GetConversation(); text != "" {
		return text
	}
	if extended := message.GetExtendedTextMessage(); extended != nil {
		return extended.GetText()
	}
	return ""
}

var (
	accentReplacer = strings.NewReplacer(
		"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n",
	)
	nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)
	bogota   = loadBogota()
)

func normalizeCommand(text string) string {
	lower := accentReplacer.Replace(strings.ToLower(text))
	return strings.TrimSpace(nonAlnum.ReplaceAllString(lower, " "))
}

func loadBogota() *time.Location {
	location, err := time.LoadLocation("America/Bogota")
	if err != nil {
		return time.FixedZone("America/Bogota", -5*60*60)
	}
	return location
}

func bogotaDate(offsetDays int) string {
	return time.Now().In(bogota).AddDate(0, 0, offsetDays).Format("2006-01-02")
}
