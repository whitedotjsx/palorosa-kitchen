package whatsapp

import (
	"encoding/base64"
	"fmt"
	"html"
	"strings"

	qrcode "github.com/skip2/go-qrcode"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/labels"
)

// QRPNG renders the current pairing QR as a PNG, or an error when there is no
// code to show (already linked, or the code expired).
func (c *Client) QRPNG() ([]byte, error) {
	c.mu.Lock()
	code := c.qrCode
	c.mu.Unlock()
	if code == "" {
		return nil, fmt.Errorf("no qr code")
	}
	return qrcode.Encode(code, qrcode.Medium, 256)
}

// pairingPageLocked builds the pairing page. The caller holds the mutex.
func (c *Client) pairingPageLocked() string {
	if c.status == StatusConnected {
		return dataPage(pairingHTML("<p class=\"ok\">" + html.EscapeString(labels.WhatsApp.PairLinked) + "</p>"))
	}

	if c.pairingExpired {
		body := "<h1>" + html.EscapeString(labels.WhatsApp.PairTitle) + "</h1>" +
			"<p class=\"error\">" + html.EscapeString(labels.WhatsApp.PairExpired) + "</p>" +
			"<button onclick=\"window.palorosaRetryPairing &amp;&amp; window.palorosaRetryPairing()\">" +
			html.EscapeString(labels.WhatsApp.Retry) + "</button>"
		return dataPage(pairingHTML(body))
	}

	qrBlock := "<p class=\"muted\">" + html.EscapeString(labels.WhatsApp.PairWaiting) + "</p>"
	if c.qrCode != "" {
		if png, err := qrcode.Encode(c.qrCode, qrcode.Medium, 320); err == nil {
			source := "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)
			qrBlock = `<img alt="Código QR" src="` + source + `">`
		}
	}

	codeBlock := ""
	if c.pairCode != "" {
		codeBlock = "<p class=\"code\">" +
			strings.ReplaceAll(html.EscapeString(labels.WhatsApp.PairCode), "{phone}", html.EscapeString(c.cfg.PairingPhone)) +
			" <strong>" + html.EscapeString(c.pairCode) + "</strong></p>"
	}

	body := "<h1>" + html.EscapeString(labels.WhatsApp.PairTitle) + "</h1>" +
		"<p>" + html.EscapeString(labels.WhatsApp.PairIntro) + "</p>" +
		"<p>" + html.EscapeString(labels.WhatsApp.PairStep) + "</p>" +
		qrBlock + codeBlock
	return dataPage(pairingHTML(body))
}

func pairingHTML(body string) string {
	return `<!doctype html>
<html lang="es"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>` + html.EscapeString(labels.WhatsApp.PairTitle) + `</title>
<style>
:root { color-scheme: light }
body { font-family: system-ui, sans-serif; margin: 0; padding: 2.5rem 1.5rem; background: #fdf8f5; color: #6a4b20; text-align: center }
h1 { color: #a55d26; font-size: 1.5rem; margin: 0 0 1rem }
p { max-width: 34rem; margin: 0 auto 1rem; line-height: 1.5 }
img { width: 320px; height: 320px; margin: 1rem auto; display: block; background: #fff; padding: 12px; border-radius: 12px; box-shadow: 0 2px 12px rgba(106,75,32,.15) }
strong { font-size: 1.75rem; letter-spacing: .15em; color: #a55d26 }
.muted { color: #9a8478 }
.error { color: #b23b2e; font-weight: 600 }
.ok { color: #2e7d32; font-weight: 600 }
button { margin-top: .5rem; padding: .7rem 1.6rem; font-size: 1rem; font-weight: 600; color: #fff; background: #a55d26; border: 0; border-radius: .6rem; cursor: pointer }
button:hover { background: #8f4d1d }
</style></head>
<body>` + body + `</body></html>`
}

func dataPage(page string) string {
	return "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte(page))
}
