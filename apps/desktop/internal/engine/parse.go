package engine

import (
	"regexp"
	"strings"
)

// SkippedText is a label fragment that produced no line.
type SkippedText struct {
	Raw         string `json:"raw"`
	Reason      string `json:"reason"`
	OrderNumber string `json:"orderNumber,omitempty"`
}

// ParsedField is the result of parsing one label field.
type ParsedField struct {
	Lines   []ParsedOrderLine
	Skipped []SkippedText
}

var (
	quantityPatterns = []*regexp.Regexp{
		regexp.MustCompile(`^(?P<name>.+?)\s+X\s+(?P<quantity>\d+)(?:\s*[:,]\s*(?P<options>.*))?$`),
	}
	productSeparators = []string{"|"}
	optionSeparators  = []string{",", "|"}
	noisePatterns     = []*regexp.Regexp{
		regexp.MustCompile(`(?i)^DOMICILIO EXCLUSIVO`),
		regexp.MustCompile(`(?i)^Observaciones:`),
		regexp.MustCompile(`(?i)^es oficina`),
		regexp.MustCompile(`(?i)^es un local`),
	}
)

// CollapseWhitespace trims and collapses internal whitespace.
func CollapseWhitespace(text string) string {
	return strings.Join(strings.Fields(text), " ")
}

func escapeRegExp(text string) string {
	return regexp.QuoteMeta(text)
}

// SplitBySeparators splits text on any of the separators.
func SplitBySeparators(text string, separators []string) []string {
	if text == "" {
		return nil
	}
	if len(separators) == 0 {
		return []string{text}
	}
	quoted := make([]string, 0, len(separators))
	for _, separator := range separators {
		quoted = append(quoted, escapeRegExp(separator))
	}
	pattern := regexp.MustCompile(strings.Join(quoted, "|"))
	return pattern.Split(text, -1)
}

func isNoise(text string) bool {
	for _, pattern := range noisePatterns {
		if pattern.MatchString(text) {
			return true
		}
	}
	return false
}

func matchQuantity(text string) (productText string, quantity int, options []string, ok bool) {
	for _, pattern := range quantityPatterns {
		match := pattern.FindStringSubmatch(text)
		if match == nil {
			continue
		}
		name := CollapseWhitespace(subexp(match, pattern, "name"))
		rawQuantity := subexp(match, pattern, "quantity")
		value := 0
		for _, r := range rawQuantity {
			if r < '0' || r > '9' {
				value = 0
				break
			}
			value = value*10 + int(r-'0')
		}
		if name == "" || value <= 0 {
			continue
		}
		options := make([]string, 0)
		for _, part := range SplitBySeparators(CollapseWhitespace(subexp(match, pattern, "options")), optionSeparators) {
			if trimmed := strings.TrimSpace(part); trimmed != "" {
				options = append(options, trimmed)
			}
		}
		return name, value, options, true
	}
	return "", 0, nil, false
}

func subexp(match []string, pattern *regexp.Regexp, name string) string {
	index := pattern.SubexpIndex(name)
	if index < 0 || index >= len(match) {
		return ""
	}
	return match[index]
}

// ParseKitchenField parses one rótulo/order field into lines (mirrors core
// parseKitchenField with the default rules).
func ParseKitchenField(text, source, orderNumber string) ParsedField {
	field := ParsedField{}
	for _, part := range SplitBySeparators(CollapseWhitespace(text), productSeparators) {
		raw := strings.TrimSpace(part)
		if raw == "" {
			continue
		}
		if isNoise(raw) {
			field.Skipped = append(field.Skipped, SkippedText{Raw: raw, Reason: "noise", OrderNumber: orderNumber})
			continue
		}
		productText, quantity, options, ok := matchQuantity(raw)
		if !ok {
			field.Lines = append(field.Lines, ParsedOrderLine{
				Raw: raw, ProductText: raw, Quantity: 1, Options: []string{},
				Source: source, OrderNumber: orderNumber, Warning: "missing_quantity",
			})
			continue
		}
		field.Lines = append(field.Lines, ParsedOrderLine{
			Raw: raw, ProductText: productText, Quantity: quantity, Options: options,
			Source: source, OrderNumber: orderNumber,
		})
	}
	return field
}

// ParseLabelFields parses the breakfast and add-ons text of one order.
func ParseLabelFields(breakfastText, additionalsText, orderNumber string) ParsedField {
	breakfast := ParseKitchenField(breakfastText, "breakfast", orderNumber)
	additionals := ParseKitchenField(additionalsText, "add_on", orderNumber)
	return ParsedField{
		Lines:   append(append([]ParsedOrderLine{}, breakfast.Lines...), additionals.Lines...),
		Skipped: append(append([]SkippedText{}, breakfast.Skipped...), additionals.Skipped...),
	}
}
