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
// second host shares the WhatsApp session and both get kicked off, so the
// check is retried before this machine decides to become the host.
func remoteHost(cfg config.Config) bool {
	if cfg.TunnelHostname == "" {
		return false
	}
	for attempt := 0; attempt < 3; attempt++ {
		if attempt > 0 {
			time.Sleep(3 * time.Second)
		}
		remote, reached := checkRemoteHost(cfg)
		if !reached {
			continue
		}
		return remote
	}
	return false
}

// checkRemoteHost asks the tunnel's health endpoint who is serving it. reached
// is false when the endpoint could not be contacted at all (a timeout is not
// proof that the other host is off).
func checkRemoteHost(cfg config.Config) (remote, reached bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://"+cfg.TunnelHostname+"/health", nil)
	if err != nil {
		return false, false
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		fmt.Fprintln(os.Stderr, "host check:", err)
		return false, false
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return false, true
	}
	var health struct {
		Panel    bool   `json:"panel"`
		Instance string `json:"instance"`
		Machine  string `json:"machine"`
	}
	if err := json.NewDecoder(response.Body).Decode(&health); err != nil || !health.Panel {
		return false, true
	}
	// Same computer (a leftover cloudflared of a previous run): stay host.
	if health.Machine != "" && health.Machine == machineID() {
		return false, true
	}
	return health.Instance != instanceID, true
}

// waitForNewHost blocks until the tunnel answers as another machine, so an old
// host that just stepped down can point its window at the new one.
func waitForNewHost(cfg config.Config, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if remoteHost(cfg) {
			return
		}
		time.Sleep(2 * time.Second)
	}
}

// spectatorURL is where the spectator window goes: the host's panel, signed in
// with the station key when both apps share the webhook secret (imported
// configuration), or the invite screen otherwise.
func spectatorURL(cfg config.Config) string {
	base := "https://" + cfg.TunnelHostname + "/"
	secret := cfg.StationSecret
	if secret == "" {
		secret = cfg.WebhookSecret
	}
	key := panelserver.StationKey(secret)
	if key == "" {
		return base
	}
	label, _ := os.Hostname()
	if label == "" {
		label = "Estación"
	}
	return base + "api/panel/station?key=" + key + "&label=" + url.QueryEscape(label)
}
