package engine

import (
	"strings"
	"unicode"
)

// OrderAnnotation is the order personalization the kitchen needs to see: the
// chosen color and the occasion (motivo). Store exports carry both as slugs
// ("oro-rosa", "cumpleanos"); the panel shows the labels.
type OrderAnnotation struct {
	Color  string `json:"color,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// annotationLabels maps the store's option slugs to their display labels,
// taken from the WooCommerce attribute options.
var annotationLabels = map[string]string{
	// Colors (Elige los globos).
	"dorado":        "Dorado",
	"rosado":        "Rosado",
	"azul-claro":    "Azul claro",
	"amarillo":      "Amarillo",
	"rosa-pastel":   "Rosa pastel",
	"dorado-negro":  "Dorado/negro",
	"naranja":       "Naranja",
	"blanco":        "Blanco",
	"chocolate":     "Chocolate",
	"vainilla":      "Vainilla",
	"palorosa":      "Palorosa",
	"oro-rosa":      "Oro rosa",
	"rojo":          "Rojo",
	"verde-oliva":   "Verde oliva",
	"azul":          "Azul",
	"plateado":      "Plateado",
	"negro":         "Negro",
	"morado":        "Morado",
	"azul-clarito":  "Azul clarito",
	"dorado-rosado": "Dorado/rosado",
	"arequipe":      "Arequipe",
	// Motivos (Elige el motivo).
	"aniversario":                  "Aniversario",
	"cumpleanos":                   "Cumpleaños",
	"feliz-dia":                    "Feliz día",
	"grado":                        "Grado",
	"sin-motivo":                   "Sin motivo",
	"bienvenida":                   "Bienvenida",
	"recuperacion":                 "Recuperación",
	"te-amo":                       "Te amo",
	"nina":                         "Niña",
	"nino":                         "Niño",
	"otros":                        "Otros",
	"quieres-ser-mi-dama-de-honor": "¿Quieres ser mi dama de honor?",
	"quieres-ser-mi-madrina":       "¿Quieres ser mi madrina?",
	"quieres-ser-mi-novio-a":       "¿Quieres ser mi novio/a?",
	"amor":                         "Amor",
	"dia-de-la-mujer":              "Día de la mujer",
	"levantar-el-animo":            "Levantar el ánimo",
	"san-valentin":                 "San valentín",
	"felicitaciones":               "Felicitaciones",
	"hombre":                       "Hombre",
	"mujer":                        "Mujer",
	"dia-del-nino":                 "Día del niño",
	"empresarial":                  "Empresarial",
	"amor-amistad":                 "Amor & Amistad",
	"viaje":                        "Viaje",
	"dia-del-hombre":               "Día del hombre",
	"dia-de-la-madre":              "Día de la madre",
	"dia-del-maestro":              "Día del maestro",
	"te-quiero":                    "Te quiero",
}

// AnnotationLabel turns a store slug into the panel label. Unknown values fall
// back to a humanized form ("dorado-rosado" would be "Dorado rosado").
func AnnotationLabel(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	if label, ok := annotationLabels[strings.ToLower(value)]; ok {
		return label
	}
	return humanizeSlug(value)
}

func humanizeSlug(value string) string {
	value = strings.ReplaceAll(value, "-", " ")
	value = strings.ReplaceAll(value, "_", " ")
	value = CollapseWhitespace(value)
	if value == "" {
		return ""
	}
	runes := []rune(value)
	runes[0] = unicode.ToUpper(runes[0])
	return string(runes)
}
