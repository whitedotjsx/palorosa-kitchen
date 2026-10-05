//go:build windows

package tunnel

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestTokenTunnelLive runs the real cloudflared against Cloudflare using a
// tunnel token, and waits for a registered edge connection. It is skipped
// unless PALOROSA_E2E_TUNNEL_TOKEN is set, because it needs network access and
// a real tunnel; it proves the no-login path end to end.
func TestTokenTunnelLive(t *testing.T) {
	token := os.Getenv("PALOROSA_E2E_TUNNEL_TOKEN")
	if token == "" {
		t.Skip("set PALOROSA_E2E_TUNNEL_TOKEN to run the live tunnel test")
	}
	hostname := os.Getenv("PALOROSA_E2E_TUNNEL_HOSTNAME")
	if hostname == "" {
		hostname = "cocina.whitesu.dev"
	}
	binary := os.Getenv("CLOUDFLARED_PATH")
	if binary == "" {
		t.Skip("set CLOUDFLARED_PATH to the cloudflared.exe to test")
	}

	manager, err := New(Config{
		Name:            "palorosa-kitchen",
		Hostname:        hostname,
		Service:         "http://127.0.0.1:5211",
		Token:           token,
		DataDir:         t.TempDir(),
		CloudflaredPath: binary,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if err := manager.Ensure(ctx); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 1)
	go func() { done <- manager.Run(ctx) }()
	deadline := time.After(30 * time.Second)
	for {
		select {
		case <-deadline:
			status, detail := manager.Status()
			t.Fatalf("tunnel did not register a connection: status=%v detail=%s", status.Name(), detail)
		case err := <-done:
			t.Fatalf("cloudflared exited early: %v", err)
		case <-time.After(250 * time.Millisecond):
			if status, _ := manager.Status(); status == Running {
				cancel()
				<-done
				return
			}
		}
	}
}
