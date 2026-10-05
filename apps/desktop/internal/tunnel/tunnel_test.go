//go:build windows

package tunnel

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// newStubManager builds a manager whose cloudflared path is a stub file, so
// token mode can be tested without running the real binary.
func newStubManager(t *testing.T, token string) *Manager {
	t.Helper()
	dir := t.TempDir()
	binary := filepath.Join(dir, "cloudflared.exe")
	if err := os.WriteFile(binary, []byte("stub"), 0o755); err != nil {
		t.Fatal(err)
	}
	manager, err := New(Config{
		Name:            "palorosa-kitchen",
		Hostname:        "cocina.example.com",
		Service:         "http://127.0.0.1:5211",
		Token:           token,
		DataDir:         dir,
		CloudflaredPath: binary,
	})
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func TestTokenModeWritesIngressOnlyConfig(t *testing.T) {
	manager := newStubManager(t, "tok")
	if !manager.TokenMode() || !manager.LoggedIn() {
		t.Fatal("token mode should be ready without cert.pem")
	}
	if err := manager.Login(context.Background()); err != nil {
		t.Fatalf("login should be a no-op in token mode: %v", err)
	}
	if err := manager.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(manager.configYML)
	if err != nil {
		t.Fatal(err)
	}
	config := string(raw)
	if strings.Contains(config, "credentials-file") || strings.Contains(config, "tunnel:") {
		t.Fatalf("token config must not use local credentials:\n%s", config)
	}
	if !strings.Contains(config, "hostname: cocina.example.com") || !strings.Contains(config, "service: http://127.0.0.1:5211") {
		t.Fatalf("token config lost the ingress:\n%s", config)
	}
	args := manager.runArgs()
	if !slices.Contains(args, "--token") || !slices.Contains(args, "tok") {
		t.Fatalf("runArgs = %v, want --token", args)
	}
	if slices.Contains(args, "palorosa-kitchen") {
		t.Fatalf("token mode should not pass the tunnel name: %v", args)
	}
}

func TestRunArgsUsesTunnelNameWithoutToken(t *testing.T) {
	manager := newStubManager(t, "")
	if manager.TokenMode() {
		t.Fatal("no token configured")
	}
	args := manager.runArgs()
	if !slices.Contains(args, "palorosa-kitchen") || slices.Contains(args, "--token") {
		t.Fatalf("runArgs = %v, want the tunnel name", args)
	}
}

func TestLoggedIn(t *testing.T) {
	home := t.TempDir()
	if loggedIn(home) {
		t.Fatal("no certificate should mean not logged in")
	}
	if err := os.WriteFile(certificatePath(home), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if loggedIn(home) {
		t.Fatal("an empty certificate should not count")
	}
	if err := os.WriteFile(certificatePath(home), []byte("cert"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !loggedIn(home) {
		t.Fatal("a non-empty certificate should count")
	}
}

func TestCertificatePath(t *testing.T) {
	home := filepath.Join("C:", "Users", "me", ".cloudflared")
	if got, want := certificatePath(home), filepath.Join(home, "cert.pem"); got != want {
		t.Fatalf("certificatePath = %q, want %q", got, want)
	}
}

func TestStatusName(t *testing.T) {
	cases := map[Status]string{
		Stopped:  "stopped",
		Starting: "starting",
		Running:  "running",
		Errored:  "errored",
	}
	for status, want := range cases {
		if got := status.Name(); got != want {
			t.Fatalf("Status(%d).Name() = %q, want %q", int(status), got, want)
		}
	}
}
