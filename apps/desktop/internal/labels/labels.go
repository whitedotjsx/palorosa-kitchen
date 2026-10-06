// Package labels holds the Spanish end-user strings. Code stays in English.
package labels

// TrayLabels are the system tray strings. {status} is replaced at runtime.
type TrayLabels struct {
	Title         string
	Tooltip       string
	Open          string
	OpenHint      string
	List          string
	ListHint      string
	Status        string
	Link          string
	LinkHint      string
	Autostart     string
	AutostartHint string
	Update        string
	UpdateHint    string
	Handoff       string
	HandoffHint   string
	Quit          string
	QuitHint      string
	Tunnel        string
}

// Tray is the tray icon and menu text.
var Tray = TrayLabels{
	Title:         "Cocina en Palorosa",
	Tooltip:       "Cocina en Palorosa — lista del día",
	Open:          "Abrir",
	OpenHint:      "Mostrar la ventana",
	List:          "Ver lista",
	ListHint:      "Volver a la lista de cocina",
	Status:        "WhatsApp: {status}",
	Link:          "Vincular WhatsApp",
	LinkHint:      "Mostrar el código QR para vincular el teléfono",
	Autostart:     "Iniciar con el sistema",
	AutostartHint: "Abrir la ventana al iniciar sesión",
	Update:        "Buscar actualización",
	UpdateHint:    "Descargar e instalar la última versión y reiniciar",
	Handoff:       "Tomar el control…",
	HandoffHint:   "Pasar la cocina a este equipo con el código del anfitrión",
	Quit:          "Salir",
	QuitHint:      "Cerrar la aplicación",
	Tunnel:        "Túnel: {status}",
}

// TunnelLabels are the Cloudflare tunnel states.
type TunnelLabels struct {
	Running  string
	Starting string
	Stopped  string
	Errored  string
	Disabled string
	// Spectator is shown when another computer is the host.
	Spectator string
}

// Tunnel is the tunnel state text.
var Tunnel = TunnelLabels{
	Running:   "activo",
	Starting:  "conectando…",
	Stopped:   "detenido",
	Errored:   "error",
	Disabled:  "apagado",
	Spectator: "espectador (otro equipo es el host)",
}

// Label maps a tunnel numeric status to its text.
func TunnelLabel(status int) string {
	switch status {
	case 1:
		return Tunnel.Starting
	case 2:
		return Tunnel.Running
	case 3:
		return Tunnel.Errored
	default:
		return Tunnel.Stopped
	}
}

// WhatsAppLabels are the connection states and pairing page text.
type WhatsAppLabels struct {
	Connected    string
	Connecting   string
	Unlinked     string
	Disconnected string
	Disabled     string

	PairTitle   string
	PairIntro   string
	PairStep    string
	PairWaiting string
	PairCode    string
	PairLinked  string
	PairExpired string
	Retry       string
}

// WhatsApp is the bot connection and pairing text.
var WhatsApp = WhatsAppLabels{
	Connected:    "conectado",
	Connecting:   "conectando…",
	Unlinked:     "sin vincular",
	Disconnected: "desconectado",
	Disabled:     "apagado",

	PairTitle:   "Vincular WhatsApp",
	PairIntro:   "Abre WhatsApp en el teléfono de la cocina, entra en Dispositivos vinculados y elige Vincular un dispositivo.",
	PairStep:    "Luego escanea este código:",
	PairWaiting: "Generando el código…",
	PairCode:    "O ingresa este código en {phone}:",
	PairLinked:  "WhatsApp quedó vinculado. Ya puedes cerrar esta pantalla.",
	PairExpired: "El código expiró. Pulsa Reintentar para generar uno nuevo.",
	Retry:       "Reintentar",
}

// BotLabels are the WhatsApp bot command replies.
type BotLabels struct {
	Commands string

	ListTitle  string
	ListFor    string
	ListHint   string
	Empty      string
	Unresolved string

	NoList      string
	Unreachable string
	StatusReady string
	StatusNone  string

	NewOrder     string
	UpdatedOrder string
	Cancelled    string
	Deleted      string
	DiffTitle    string
	DiffEmpty    string
	Observation  string

	Added            string
	Changed          string
	Removed          string
	ChangedLine      string
	NotificationHint string
	WhenToday        string
	WhenTomorrow     string

	OrderPrefix   string
	NoFoodChange  string
	AddToMake     string
	TotalAfterAdd string
	EmptySection  string

	OrderOriginal string
	OrderUpdated  string
	ListAfter     string
	RemovedOne    string
	RemovedMany   string
	AddedOne      string
	AddedMany     string
	NewTotalNote  string
	NoLongerMake  string
	NewInList     string

	BadDate  string
	Unknown  string
	Greeting string

	ReadySaved    string
	NoCheckpoint  string
	NothingNew    string
	NewSinceTitle string

	NotifOn      string
	NotifOff     string
	NotifHeader  string
	NotifLine    string
	NotifSet     string
	OnWord       string
	OffWord      string
	NameNew      string
	NameUpdated  string
	NameToday    string
	NameTomorrow string
	BadOption    string
}

// Bot is the bot reply text, written in plain Spanish for the kitchen staff.
var Bot = BotLabels{
	Commands: "Soy el asistente de la cocina. Escríbeme una de estas opciones:\n\n" +
		"1. LISTA — la lista de hoy\n" +
		"2. LISTA MAÑANA — la lista de mañana\n" +
		"3. LISTA 5 OCTUBRE — la lista de otro día (también sirve LISTA 2026-10-05)\n" +
		"4. ESTADO — qué listas hay publicadas\n" +
		"5. LISTO — marca el corte (guardé lo preparado)\n" +
		"6. NUEVO — lo que llegó desde el último LISTO\n" +
		"7. NOTIFICACIONES — ver o cambiar los avisos\n" +
		"8. AYUDA — este mensaje\n\n" +
		"Para los avisos puedes escribir, por ejemplo:\n" +
		"\"notificaciones desactivar\" (apagarlos)\n" +
		"\"notificaciones activar\" (encenderlos)\n" +
		"\"notificaciones nuevos desactivar\" (no avisar de pedidos nuevos)\n" +
		"\"notificaciones cambios desactivar\" (no avisar de cambios)\n" +
		"\"notificaciones hoy desactivar\" o \"notificaciones mañana desactivar\"",

	ListTitle:  "🍳 Lista de cocina",
	ListFor:    "🍳 Lista de cocina para {when}",
	ListHint:   "ℹ️ Escribe AYUDA para ver las opciones.",
	Empty:      "🍽️ Todavía no hay nada para preparar ese día.",
	Unresolved: "⚠️ Falta definir estos productos:",

	NoList:      "📭 Todavía no hay lista para {date}.",
	Unreachable: "⚠️ No pude consultar el servidor de cocina ({url}).",
	StatusReady: "🤖 El asistente está trabajando.\nHoy es {date}.\n\nListas publicadas:\n{dates}",
	StatusNone:  "🤖 El asistente está trabajando.\nHoy es {date}.\n\nTodavía no hay listas publicadas.",

	NewOrder:     "🔔 Pedido nuevo",
	UpdatedOrder: "✏️ Pedido actualizado",
	Cancelled:    "❌ Pedido cancelado",
	Deleted:      "🗑️ Pedido eliminado",
	DiffTitle:    "Cambios en la lista",
	DiffEmpty:    "Sin cambios.",
	Observation:  "📝 Observación: {text}",

	Added:            "➕ Hay que agregar:",
	Changed:          "🔄 Hay que cambiar:",
	Removed:          "➖ Hay que quitar:",
	ChangedLine:      "{name}: antes {previous}, ahora {next}",
	NotificationHint: "ℹ️ Escribe LISTA para ver la lista completa.",
	WhenToday:        "HOY",
	WhenTomorrow:     "MAÑANA",

	OrderPrefix:   "🧾 Pedido: {text}",
	NoFoodChange:  "ℹ️ Este pedido no cambió nada de comida.",
	AddToMake:     "➕ Hay que agregar:",
	TotalAfterAdd: "📋 Total en la lista (después de agregar):",
	EmptySection:  "(nada)",

	OrderOriginal: "📄 Pedido original:",
	OrderUpdated:  "✏️ Pedido actualizado:",
	ListAfter:     "📋 Así queda la lista:",
	RemovedOne:    "(❌ se eliminó {n})",
	RemovedMany:   "(❌ se eliminaron {n})",
	AddedOne:      "(✅ se agregó {n})",
	AddedMany:     "(✅ se agregaron {n})",
	NewTotalNote:  "(🔄 nuevo total)",
	NoLongerMake:  "(🚫 ya no hay que hacer)",
	NewInList:     "(🆕 nuevo en la lista)",

	BadDate:  "⚠️ No entendí la fecha. Escribe por ejemplo \"lista 5 octubre\" o \"lista 2026-10-05\".",
	Unknown:  "🤔 No entendí. Escribe AYUDA para ver lo que puedo hacer.",
	Greeting: "👋 Hola. Escribe AYUDA para ver lo que puedo hacer por ti.",

	ReadySaved:    "✅ Listo. Guardé este momento. Escribe NUEVO para ver lo que llega después.",
	NoCheckpoint:  "⚠️ Todavía no has dicho LISTO. Escribe LISTO cuando termines de preparar la lista.",
	NothingNew:    "✅ No ha llegado nada nuevo desde el último LISTO.",
	NewSinceTitle: "🆕 Nuevo desde el último LISTO:",

	NotifOn:      "🔔 Listo, avisos encendidos.",
	NotifOff:     "🔕 Listo, avisos apagados.",
	NotifHeader:  "🔔 Avisos: {state}",
	NotifLine:    "- {name}: {value}",
	NotifSet:     "Listo, {name}: {value}.",
	OnWord:       "encendidos",
	OffWord:      "apagados",
	NameNew:      "pedidos nuevos",
	NameUpdated:  "cambios de pedido",
	NameToday:    "para hoy",
	NameTomorrow: "para mañana",
	BadOption:    "🤔 No entendí. Escribe \"notificaciones\" para ver cómo están, o usa \"notificaciones activar|desactivar\", \"notificaciones nuevos activar|desactivar\", \"notificaciones cambios activar|desactivar\", \"notificaciones hoy activar|desactivar\", \"notificaciones mañana activar|desactivar\".",
}

// ActivityLabels are the recent-activity feed texts. {n} is the order number.
type ActivityLabels struct {
	NewOrder    string
	UpdateOrder string
	Removed     string
}

// Activity is the panel's recent-activity feed text.
var Activity = ActivityLabels{
	NewOrder:    "Pedido #{n} nuevo",
	UpdateOrder: "Pedido #{n} actualizado",
	Removed:     "Pedido #{n} cancelado",
}

// CategoryLabels map a kitchen unit category to its list heading.
var CategoryLabels = map[string]string{
	"drink":     "🥤 Bebidas",
	"main":      "🍔 Comidas principales",
	"side":      "🥗 Acompañamientos",
	"dessert":   "🍰 Postres y dulces",
	"fruit":     "🍓 Frutas",
	"condiment": "🧂 Condimentos",
	"other":     "📦 Otros",
}

// ReasonLabels map an unresolved reason to its Spanish description.
var ReasonLabels = map[string]string{
	"unknown_product": "Producto no reconocido",
	"missing_recipe":  "Producto sin receta",
	"missing_choice":  "Opción no reconocida",
	"ambiguous_match": "Coincidencia ambigua",
	"missing_unit":    "Unidad no encontrada",
}
