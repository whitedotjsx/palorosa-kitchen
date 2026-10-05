//go:build windows

package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/config"
	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/panelserver"
)

// instanceID identifies this running process in /health.
var instanceID = func() string {
	buffer := make([]byte, 8)
	_, _ = rand.Read(buffer)
	return hex.EncodeToString(buffer)
}()

// machineID is a short hash of the computer name: enough to tell this
// machine's own leftover tunnel apart from another computer's, without
// publishing the name.
func machineID() string {
	name, _ := os.Hostname()
	sum := sha256.Sum256([]byte(strings.ToLower(name)))
	return hex.EncodeToString(sum[:])[:12]
}

// remoteHost reports whether another computer is already serving the tunnel
// hostname. Only one host may run the tunnel, the bots and the webhooks; a
// second app that finds a live host becomes a spectator.
func remoteHost(cfg config.Config) bool {
	if cfg.TunnelHostname == "" {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+cfg.TunnelHostname+"/health", nil)
	if err != nil {
		return false
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		fmt.Fprintln(os.Stderr, "host check:", err)
		return false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false
	}
	var health struct {
		Panel    bool   `json:"panel"`
		Instance string `json:"instance"`
		Machine  string `json:"machine"`
	}
	if err := json.NewDecoder(response.Body).Decode(&health); err != nil || !health.Panel {
		return false
	}
	// Same computer (a leftover cloudflared of a previous run): stay host.
	if health.Machine != "" && health.Machine == machineID() {
		return false
	}
	return health.Instance != instanceID
}

// spectatorURL is where the spectator window goes: the host's panel, signed in
// with the station key when both apps share the webhook secret (imported
// configuration), or the invite screen otherwise.
func spectatorURL(cfg config.Config) string {
	base := "https://" + cfg.TunnelHostname + "/"
	key := panelserver.StationKey(cfg.WebhookSecret)
	if key == "" {
		return base
	}
	label, _ := os.Hostname()
	if label == "" {
		label = "Estación"
	}
	return base + "api/panel/station?key=" + key + "&label=" + url.QueryEscape(label)
}
