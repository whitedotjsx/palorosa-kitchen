package engine

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
)

// OrdersExport is the parsed result of an order export.
type OrdersExport struct {
	DeliveryDate string
	Lines        []ParsedOrderLine
	Skipped      []SkippedText
	// Annotations holds the color and motivo of each order, keyed by order
	// number. The wide export carries them as metadata columns.
	Annotations map[string]OrderAnnotation
}

// DefaultWideExportMetadataColumns are the columns that are not food choices.
var DefaultWideExportMetadataColumns = []string{
	"ID orden",
	"Fecha de Entrega",
	"Productos",
	"Elige los Globos",
	"Fotos",
	"Adicionales",
	"Observaciones",
	"color",
	"stiker",
	"motivo",
	"vegetariano",
	"Tiene Carta",
	"Barrio",
}

var spanishMonths = map[string]string{
	"enero": "01", "febrero": "02", "marzo": "03", "abril": "04",
	"mayo": "05", "junio": "06", "julio": "07", "agosto": "08",
	"septiembre": "09", "setiembre": "09", "octubre": "10",
	"noviembre": "11", "diciembre": "12",
}

// ParseSpanishDeliveryDate reads "29 Septiembre, 2026" or an Excel serial.
func ParseSpanishDeliveryDate(value any) string {
	switch typed := value.(type) {
	case float64:
		return excelSerialDate(typed)
	case int:
		return excelSerialDate(float64(typed))
	case string:
		text := strings.ToLower(CollapseWhitespace(typed))
		if len(text) == 10 && text[4] == '-' && text[7] == '-' {
			return text
		}
		fields := strings.FieldsFunc(text, func(r rune) bool {
			return r == ' ' || r == ',' || r == '-'
		})
		// Expected: day month year (month name in Spanish).
		if len(fields) < 3 {
			return ""
		}
		day := fields[0]
		monthName := fields[1]
		year := fields[2]
		month, ok := spanishMonths[StripAccents(monthName)]
		if !ok {
			return ""
		}
		if len(day) == 1 {
			day = "0" + day
		}
		if _, err := strconv.Atoi(day); err != nil {
			return ""
		}
		if _, err := strconv.Atoi(year); err != nil {
			return ""
		}
		return year + "-" + month + "-" + day
	default:
		return ""
	}
}

func excelSerialDate(serial float64) string {
	seconds := (serial - 25569) * 86400
	date := time.Unix(int64(math.Round(seconds)), 0).UTC()
	return date.Format("2006-01-02")
}

// IsPositive reports whether a choice cell marks the option.
func IsPositive(value any) bool {
	switch typed := value.(type) {
	case bool:
		return typed
	case float64:
		return typed > 0
	case int:
		return typed > 0
	case string:
		text := strings.ToLower(strings.TrimSpace(typed))
		return text != "" && text != "0" && text != "no" && text != "false"
	default:
		return false
	}
}

// IsWideOrderHeader reports whether a header row is the wide order export.
func IsWideOrderHeader(header []string) bool {
	normalized := make([]string, 0, len(header))
	for _, cell := range header {
		normalized = append(normalized, NormalizeName(cell))
	}
	return contains(normalized, "id orden") && contains(normalized, "productos")
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

// TableToRecords turns a header + rows into records.
func TableToRecords(rows [][]string) []map[string]any {
	if len(rows) == 0 {
		return nil
	}
	header := make([]string, 0, len(rows[0]))
	for _, cell := range rows[0] {
		header = append(header, CollapseWhitespace(cell))
	}
	records := make([]map[string]any, 0, len(rows)-1)
	for _, row := range rows[1:] {
		record := map[string]any{}
		for index, key := range header {
			if key == "" {
				continue
			}
			if index < len(row) {
				record[key] = row[index]
			} else {
				record[key] = ""
			}
		}
		records = append(records, record)
	}
	return records
}

// WideExportOptions tunes ParseWideOrderRows.
type WideExportOptions struct {
	MetadataColumns      []string
	JuiceColumn          string
	JuiceFallbackOptions []string
}

// ParseWideOrderRows parses wide export records into lines (mirrors core
// parseWideOrderRows).
func ParseWideOrderRows(rows []map[string]any, options WideExportOptions) OrdersExport {
	metadata := map[string]bool{}
	columns := options.MetadataColumns
	if columns == nil {
		columns = DefaultWideExportMetadataColumns
	}
	for _, column := range columns {
		metadata[column] = true
	}
	juiceColumn := options.JuiceColumn
	if juiceColumn == "" {
		juiceColumn = "Jugo de Naranja"
	}
	juiceFallbacks := options.JuiceFallbackOptions
	if juiceFallbacks == nil {
		juiceFallbacks = []string{"Hatsu té", "Café Mocca"}
	}

	result := OrdersExport{Lines: []ParsedOrderLine{}, Skipped: []SkippedText{}, Annotations: map[string]OrderAnnotation{}}
	for _, record := range rows {
		orderNumber := firstString(record["ID orden"], record["ID"], record["orderNumber"])
		context := orderNumber
		if result.DeliveryDate == "" {
			result.DeliveryDate = ParseSpanishDeliveryDate(record["Fecha de Entrega"])
		}
		annotation := OrderAnnotation{
			Color:  AnnotationLabel(stringOf(record["color"])),
			Reason: AnnotationLabel(stringOf(record["motivo"])),
		}
		if orderNumber != "" && (annotation.Color != "" || annotation.Reason != "") {
			result.Annotations[orderNumber] = annotation
		}

		var optionTexts []string
		for key, value := range record {
			if metadata[key] {
				continue
			}
			if IsPositive(value) {
				optionTexts = append(optionTexts, key)
			}
		}
		if !IsPositive(record[juiceColumn]) {
			optionTexts = append(optionTexts, juiceFallbacks...)
		}

		breakfast := ParseKitchenField(stringOf(record["Productos"]), "breakfast", context)
		for _, line := range breakfast.Lines {
			line.Options = append(append([]string{}, line.Options...), optionTexts...)
			result.Lines = append(result.Lines, line)
		}
		result.Skipped = append(result.Skipped, breakfast.Skipped...)

		additionals := ParseKitchenField(stringOf(record["Adicionales"]), "add_on", context)
		result.Lines = append(result.Lines, additionals.Lines...)
		result.Skipped = append(result.Skipped, additionals.Skipped...)

		if len(breakfast.Lines) == 0 && len(additionals.Lines) == 0 {
			result.Skipped = append(result.Skipped, SkippedText{Raw: fmt.Sprintf("%v", record), Reason: "malformed", OrderNumber: context})
		}
	}
	return result
}

func firstString(values ...any) string {
	for _, value := range values {
		switch typed := value.(type) {
		case string:
			if trimmed := CollapseWhitespace(typed); trimmed != "" {
				return trimmed
			}
		case float64:
			return strconv.FormatFloat(typed, 'f', -1, 64)
		case int:
			return strconv.Itoa(typed)
		}
	}
	return ""
}

func stringOf(value any) string {
	switch typed := value.(type) {
	case string:
		return typed
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", typed)
	}
}
