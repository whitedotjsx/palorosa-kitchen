//go:build windows

// Package tunnel manages the Cloudflare named tunnel that exposes the local
// hook server. The connector runs in process through the vendored
// cf-quick-tunnel shared library (third_party/cf-quick-tunnel-rs), so no
// cloudflared subprocess is needed. An installed cloudflared binary is only
// used to provision a tunnel from the account certificate (login, create,
// route DNS).
package tunnel

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wsuites/palorosa-kitchen/apps/desktop/internal/cftunnel"
)

// ErrNotLoggedIn is returned when the tunnel has no token and no account
// certificate, so the panel can guide the operator through a one-time setup.
var ErrNotLoggedIn = errors.New("no hay token del túnel ni certificado de cloudflared")

// loginHint is the one-time command that creates the account certificate.
const loginHint = "cloudflared tunnel login"

// Status is the tunnel state.
type Status int

const (
	// Stopped means the connector is not running.
	Stopped Status = iota
	// Starting means the connector is connecting to the edge.
	Starting
	// Running means at least one edge connection registered.
	Running
	// Errored means setup or the connector failed.
	Errored
)

// Name returns the machine readable status for the panel API. User text comes
// from the label maps.
func (s Status) Name() string {
	switch s {
	case Starting:
		return "starting"
	case Running:
		return "running"
	case Errored:
		return "errored"
	default:
		return "stopped"
	}
}

// Config configures the tunnel manager.
type Config struct {
	Name     string
	Hostname string
	Service  string
	// Token runs the tunnel without the account certificate. It is the only
	// credential a new computer needs (exported with the rest of the settings).
	Token   string
	DataDir string
	// CloudflaredPath is an optional cloudflared binary used only to provision
	// a named tunnel from the account certificate.
	CloudflaredPath string
	// Library is the cf-quick-tunnel shared library embedded in the exe. The
	// manager deploys it under DataDir and loads it.
	Library  []byte
	OnStatus func(Status, string)
}

// Manager owns the tunnel connector.
type Manager struct {
	cfg       Config
	binary    string // cloudflared, optional (cert-mode provisioning)
	home      string
	configYML string

	mu     sync.Mutex
	status Status
	detail string
	lib    *cftunnel.Tunnel
	libErr error
	cmd    *exec.Cmd
}

// New prepares the manager: it locates the optional cloudflared binary and
// deploys and loads the embedded connector library. It does not touch the
// network or the Cloudflare account.
func New(cfg Config) (*Manager, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	manager := &Manager{
		cfg:       cfg,
		home:      filepath.Join(home, ".cloudflared"),
		configYML: filepath.Join(cfg.DataDir, "cloudflared.yml"),
		status:    Stopped,
	}
	if binary, err := findCloudflared(cfg.CloudflaredPath); err == nil {
		manager.binary = binary
	}
	if len(cfg.Library) > 0 {
		path, err := cftunnel.Materialize(cfg.DataDir, cfg.Library)
		if err != nil {
			return nil, err
		}
		lib, err := cftunnel.Load(path)
		if err != nil {
			return nil, err
		}
		manager.lib = lib
	} else {
		manager.libErr = errors.New("el conector del túnel no está compilado; ejecuta pnpm desktop:build")
	}
	return manager, nil
}

// PublicURL is the HTTPS URL the tunnel exposes.
func (m *Manager) PublicURL() string {
	return "https://" + m.cfg.Hostname
}

// Hostname is the configured tunnel hostname.
func (m *Manager) Hostname() string { return m.cfg.Hostname }

// Service is the local origin the tunnel fronts.
func (m *Manager) Service() string { return m.cfg.Service }

// CertificatePath is where cloudflared keeps the account certificate.
func (m *Manager) CertificatePath() string { return certificatePath(m.home) }

// TokenMode reports whether the tunnel authenticates with a token instead of
// the account certificate.
func (m *Manager) TokenMode() bool { return m.cfg.Token != "" }

// LoggedIn reports whether the tunnel can run: a configured token, or the
// account certificate that also allows creating tunnels and routing DNS.
func (m *Manager) LoggedIn() bool { return m.TokenMode() || loggedIn(m.home) }

// Login runs `cloudflared tunnel login` once, so the operator authorizes the
// zone in the browser. It is a no-op when a token is configured or the
// certificate already exists.
func (m *Manager) Login(ctx context.Context) error {
	if m.LoggedIn() {
		return nil
	}
	if m.binary == "" {
		return fmt.Errorf("%w: configura un token del túnel en Ajustes", ErrNotLoggedIn)
	}
	return m.runCloudflaredCommand(ctx, "tunnel", "login")
}

// Status returns the current state and detail.
func (m *Manager) Status() (Status, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status, m.detail
}

// Ensure provisions the tunnel. Token mode needs nothing: the token already
// identifies the tunnel and the ingress travels with the connector. Cert mode
// creates the named tunnel and routes its DNS hostname with cloudflared.
func (m *Manager) Ensure(ctx context.Context) error {
	if m.TokenMode() {
		return nil
	}
	if _, ok := m.provisioned(); ok {
		return nil
	}
	if !m.LoggedIn() || m.binary == "" {
		return fmt.Errorf("%w: run %q once or configure a tunnel token", ErrNotLoggedIn, loginHint)
	}

	id, err := m.findTunnel(ctx)
	if err != nil {
		return err
	}
	if id == "" {
		if err := m.runCloudflaredCommand(ctx, "tunnel", "create", m.cfg.Name); err != nil {
			return fmt.Errorf("create tunnel: %w", err)
		}
		if id, err = m.findTunnel(ctx); err != nil {
			return err
		}
		if id == "" {
			return fmt.Errorf("tunnel %q was not created", m.cfg.Name)
		}
	}

	// Route the hostname; an existing record is fine.
	if err := m.runCloudflaredCommand(ctx, "tunnel", "route", "dns", m.cfg.Name, m.cfg.Hostname); err != nil {
		message := strings.ToLower(err.Error())
		if !strings.Contains(message, "already exists") && !strings.Contains(message, "record with that host already exists") {
			return fmt.Errorf("route dns: %w", err)
		}
	}

	return m.writeConfig(id)
}

// Run starts the connector and blocks until the context is cancelled, the
// tunnel stops or an error occurs. Ensure must run first.
func (m *Manager) Run(ctx context.Context) error {
	if m.lib == nil {
		// Token mode only runs through the in-process connector; the legacy
		// path is for a host with a provisioned certificate and cloudflared.
		if !m.TokenMode() && m.binary != "" {
			if _, ok := m.provisioned(); ok {
				return m.runCloudflared(ctx)
			}
		}
		runErr := m.libErr
		if runErr == nil {
			runErr = errors.New("el conector del túnel no está disponible")
		}
		m.setStatus(Errored, runErr.Error())
		return runErr
	}

	m.setStatus(Starting, "conectando")
	credential, err := m.credential()
	if err != nil {
		m.setStatus(Errored, err.Error())
		return err
	}
	if err := m.lib.Start(credential, m.ingressYAML()); err != nil {
		m.setStatus(Errored, err.Error())
		return err
	}

	ticker := time.NewTicker(500 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			m.lib.Stop()
			m.awaitStopped(5 * time.Second)
			m.setStatus(Stopped, "detenido")
			return nil
		case <-ticker.C:
			switch m.lib.State() {
			case cftunnel.Running:
				if status, _ := m.Status(); status != Running {
					m.setStatus(Running, "activo")
				}
			case cftunnel.Errored:
				runErr := errors.New(m.lib.LastError())
				m.setStatus(Errored, runErr.Error())
				return runErr
			case cftunnel.Stopped:
				if status, _ := m.Status(); status != Stopped {
					m.setStatus(Stopped, "detenido")
					return nil
				}
			}
		}
	}
}

// Stop requests the connector shutdown.
func (m *Manager) Stop() {
	m.mu.Lock()
	lib := m.lib
	command := m.cmd
	m.mu.Unlock()
	if lib != nil {
		lib.Stop()
		return
	}
	if command != nil && command.Process != nil {
		_ = command.Process.Kill()
	}
}

// ingressYAML is the local ingress the connector routes: the public hostname
// to the hook server, then a 404 catch-all.
func (m *Manager) ingressYAML() string {
	return fmt.Sprintf(
		"ingress:\n  - hostname: %s\n    service: %s\n  - service: http_status:404\n",
		m.cfg.Hostname,
		m.cfg.Service,
	)
}

// credential is the token, or the credentials JSON of the provisioned tunnel
// in certificate mode.
func (m *Manager) credential() (string, error) {
	if m.cfg.Token != "" {
		return m.cfg.Token, nil
	}
	id, ok := m.provisioned()
	if !ok {
		return "", fmt.Errorf("%w: configura un token del túnel en Ajustes", ErrNotLoggedIn)
	}
	raw, err := os.ReadFile(filepath.Join(m.home, id+".json"))
	if err != nil {
		return "", fmt.Errorf("credentials de cloudflared: %w", err)
	}
	return string(raw), nil
}

// awaitStopped waits for the library to finish its graceful shutdown.
func (m *Manager) awaitStopped(limit time.Duration) {
	deadline := time.Now().Add(limit)
	for time.Now().Before(deadline) {
		if m.lib.State() == cftunnel.Stopped {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func (m *Manager) setStatus(status Status, detail string) {
	m.mu.Lock()
	m.status = status
	m.detail = detail
	m.mu.Unlock()
	if m.cfg.OnStatus != nil {
		m.cfg.OnStatus(status, detail)
	}
}
