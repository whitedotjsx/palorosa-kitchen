package engine

import (
	"strings"
	"unicode"

	"golang.org/x/text/runes"
	"golang.org/x/text/transform"
	"golang.org/x/text/unicode/norm"
)

var accentTransformer = transform.Chain(norm.NFD, runes.Remove(runes.In(unicode.Mn)), norm.NFC)

// StripAccents removes diacritics (mirrors core stripAccents).
func StripAccents(text string) string {
	out, _, err := transform.String(accentTransformer, text)
	if err != nil {
		return text
	}
	return out
}

// NormalizeName lowercases, strips accents and collapses non alphanumerics to
// single spaces (mirrors core normalizeName).
func NormalizeName(text string) string {
	var builder strings.Builder
	pendingSpace := false
	for _, r := range strings.ToLower(StripAccents(text)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			if pendingSpace && builder.Len() > 0 {
				builder.WriteByte(' ')
			}
			pendingSpace = false
			builder.WriteRune(r)
			continue
		}
		pendingSpace = true
	}
	return builder.String()
}
