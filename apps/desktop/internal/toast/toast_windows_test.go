//go:build windows

package toast

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestScriptEncodesContent(t *testing.T) {
	title := "Pedido 'nuevo' <#1>"
	body := "línea uno\nlínea dos"
	text := script(title, body)

	if !strings.Contains(text, base64.StdEncoding.EncodeToString([]byte(title))) {
		t.Fatal("title is not base64 encoded")
	}
	if !strings.Contains(text, base64.StdEncoding.EncodeToString([]byte(body))) {
		t.Fatal("body is not base64 encoded")
	}
	if strings.Contains(text, title) {
		t.Fatal("raw title leaked into the script")
	}
}
