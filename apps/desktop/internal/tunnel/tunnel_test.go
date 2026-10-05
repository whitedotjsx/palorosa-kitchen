//go:build windows

package tunnel

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// newTestManager builds a manager without the connector library or a
// cloudflared binary, so the configuration side can be tested in isolation.
func newTestManager(t *testing.T, cfg Config) *Manager {
	t.Helper()
	if cfg.DataDir == "" {
		cfg.DataDir = t.TempDir()
	}
	if cfg.Name == "" {
		cfg.Name = "palorosa-kitchen"
	}
	if cfg.Hostname == "" {
		cfg.Hostname = "cocina.example.com"
	}
	if cfg.Service == "" {
		cfg.Service = "http://127.0.0.1:5211"
	}
	manager, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func TestTokenModeIsReadyWithoutCertificate(t *testing.T) {
	manager := newTestManager(t, Config{Token: "tok"})
	if !manager.TokenMode() || !manager.LoggedIn() {
		t.Fatal("token mode should be ready without cert.pem")
	}
	if err := manager.Login(context.Background()); err != nil {
		t.Fatalf("login should be a no-op in token mode: %v", err)
	}
}

func TestEnsureTokenModeIsNoop(t *testing.T) {
	// The token identifies the tunnel and the ingress travels with the
	// in-process connector, so nothing is provisioned locally.
	manager := newTestManager(t, Config{Token: "tok"})
	if err := manager.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(manager.configYML); !os.IsNotExist(err) {
		t.Fatalf("token mode should not write a cloudflared config: %v", err)
	}
}

func TestEnsureWithoutTokenOrCertificateFails(t *testing.T) {
	// An isolated home, so the real account certificate of the test machine
	// cannot provision anything.
	dir := t.TempDir()
	manager := &Manager{
		cfg: Config{
			Name:     "palorosa-kitchen",
			Hostname: "cocina.example.com",
			Service:  "http://127.0.0.1:5211",
			DataDir:  dir,
		},
		home:      filepath.Join(dir, "home"),
		configYML: filepath.Join(dir, "cloudflared.yml"),
	}
	if err := manager.Ensure(context.Background()); err == nil {
		t.Fatal("expected an error without token or certificate")
	}
}

func TestIngressYAML(t *testing.T) {
	manager := newTestManager(t, Config{Token: "tok"})
	ingress := manager.ingressYAML()
	for _, want := range []string{"hostname: cocina.example.com", "service: http://127.0.0.1:5211", "http_status:404"} {
		if !strings.Contains(ingress, want) {
			t.Fatalf("ingress = %q, want %q", ingress, want)
		}
	}
}

func TestCredentialReadsProvisionedFile(t *testing.T) {
	dir := t.TempDir()
	home := t.TempDir()
	id := "95ece313-03c2-4ba6-91be-d19fbf384cf7"
	credentialsPath := filepath.Join(home, id+".json")
	if err := os.WriteFile(credentialsPath, []byte(`{"AccountTag":"acct"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	config := "tunnel: " + id + "\ncredentials-file: " + filepath.ToSlash(credentialsPath) + "\n"
	manager := &Manager{
		cfg:       Config{DataDir: dir},
		home:      home,
		configYML: filepath.Join(dir, "cloudflared.yml"),
	}
	if err := os.WriteFile(manager.configYML, []byte(config), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := manager.credential()
	if err != nil {
		t.Fatal(err)
	}
	if got != `{"AccountTag":"acct"}` {
		t.Fatalf("credential = %q", got)
	}
}

func TestCredentialWithoutSetupFails(t *testing.T) {
	manager := newTestManager(t, Config{})
	if _, err := manager.credential(); err == nil {
		t.Fatal("expected an error without a token or provisioned credentials")
	}
}

func TestRunWithoutConnectorFails(t *testing.T) {
	manager := newTestManager(t, Config{Token: "tok"})
	if err := manager.Run(context.Background()); err == nil {
		t.Fatal("expected an error without the connector library")
	}
	if status, _ := manager.Status(); status != Errored {
		t.Fatalf("status = %v, want errored", status.Name())
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
