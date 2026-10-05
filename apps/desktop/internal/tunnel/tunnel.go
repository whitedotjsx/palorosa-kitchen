//go:build windows

// Package tunnel manages a Cloudflare named tunnel with cloudflared as a
// sidecar, so WooCommerce or a plugin can reach the local hook server.
package tunnel

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
)

// ErrNotLoggedIn is returned by Ensure when cloudflared has no account
// certificate, so the panel can guide the operator through a one-time login.
var ErrNotLoggedIn = errors.New("cloudflared is not logged in")

// loginHint is the one-time command that creates the account certificate.
const loginHint = "cloudflared tunnel login"

// Status is the tunnel state.
type Status int

const (
	// Stopped means the sidecar is not running.
	Stopped Status = iota
	// Starting means cloudflared was launched and is connecting.
	Starting
	// Running means at least one edge connection registered.
	Running
	// Errored means setup or the process failed.
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
	Name            string
	Hostname        string
	Service         string
	DataDir         string
	CloudflaredPath string
	OnStatus        func(Status, string)
}

// Manager owns the cloudflared sidecar.
type Manager struct {
	cfg       Config
	binary    string
	home      string
	configYML string

	mu        sync.Mutex
	status    Status
	detail    string
	cmd       *exec.Cmd
	connected bool
}

// New locates cloudflared and prepares the manager. It does not touch the
// network or the Cloudflare account.
func New(cfg Config) (*Manager, error) {
	binary, err := findCloudflared(cfg.CloudflaredPath)
	if err != nil {
		return nil, err
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	return &Manager{
		cfg:       cfg,
		binary:    binary,
		home:      filepath.Join(home, ".cloudflared"),
		configYML: filepath.Join(cfg.DataDir, "cloudflared.yml"),
		status:    Stopped,
	}, nil
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

// LoggedIn reports whether the account certificate exists, which is what
// allows the manager to create tunnels and route DNS.
func (m *Manager) LoggedIn() bool { return loggedIn(m.home) }

// Login runs `cloudflared tunnel login` once, so the operator authorizes the
// zone in the browser. It is a no-op when the certificate already exists.
func (m *Manager) Login(ctx context.Context) error {
	if m.LoggedIn() {
		return nil
	}
	return m.run(ctx, "tunnel", "login")
}

// Status returns the current state and detail.
func (m *Manager) Status() (Status, string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.status, m.detail
}

// Ensure creates the named tunnel if missing, routes its DNS hostname and
// writes the config file. When it is already provisioned it only checks the
// config and credentials, so a transient Cloudflare API failure to list does
// not stop the sidecar from running.
func (m *Manager) Ensure(ctx context.Context) error {
	// Already provisioned: rewrite the config anyway (no network needed), so a
	// changed hostname or local port reaches cloudflared. Before, the file was
	// left as first written and the tunnel kept pointing at an old port (502).
	if id, ok := m.provisioned(); ok {
		return m.writeConfig(id)
	}
	if !m.LoggedIn() {
		return fmt.Errorf("%w: run %q once", ErrNotLoggedIn, loginHint)
	}

	id, err := m.findTunnel(ctx)
	if err != nil {
		return err
	}
	if id == "" {
		if err := m.run(ctx, "tunnel", "create", m.cfg.Name); err != nil {
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
	if err := m.run(ctx, "tunnel", "route", "dns", m.cfg.Name, m.cfg.Hostname); err != nil {
		message := strings.ToLower(err.Error())
		if !strings.Contains(message, "already exists") && !strings.Contains(message, "record with that host already exists") {
			return fmt.Errorf("route dns: %w", err)
		}
	}

	return m.writeConfig(id)
}

// writeConfig writes the cloudflared config for the tunnel id, only touching
// the file when the content changes.
func (m *Manager) writeConfig(id string) error {
	config := fmt.Sprintf(
		"# Managed by palorosa-kitchen. Do not edit by hand.\ntunnel: %s\ncredentials-file: %s\ningress:\n  - hostname: %s\n    service: %s\n  - service: http_status:404\n",
		id,
		filepath.ToSlash(filepath.Join(m.home, id+".json")),
		m.cfg.Hostname,
		m.cfg.Service,
	)
	if current, err := os.ReadFile(m.configYML); err == nil && string(current) == config {
		return nil
	}
	if err := os.MkdirAll(m.cfg.DataDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(m.configYML, []byte(config), 0o644)
}

// provisioned reports the tunnel id when our config file and its credentials
// exist already.
func (m *Manager) provisioned() (string, bool) {
	raw, err := os.ReadFile(m.configYML)
	if err != nil {
		return "", false
	}
	id, credentials := "", false
	for _, line := range strings.Split(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		switch {
		case strings.HasPrefix(trimmed, "tunnel:"):
			id = strings.TrimSpace(strings.TrimPrefix(trimmed, "tunnel:"))
		case strings.HasPrefix(trimmed, "credentials-file:"):
			path := strings.TrimSpace(strings.TrimPrefix(trimmed, "credentials-file:"))
			if info, err := os.Stat(path); err == nil && !info.IsDir() {
				credentials = true
			}
		}
	}
	return id, id != "" && credentials
}

// Run starts the sidecar and blocks until the context is cancelled or the
// process exits. Ensure must run first.
func (m *Manager) Run(ctx context.Context) error {
	m.setStatus(Starting, "conectando")
	command := hideWindow(exec.CommandContext(ctx, m.binary, "--config", m.configYML, "--no-autoupdate", "tunnel", "run", m.cfg.Name))
	stderr, err := command.StderrPipe()
	if err != nil {
		m.setStatus(Errored, err.Error())
		return err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		m.setStatus(Errored, err.Error())
		return err
	}

	m.mu.Lock()
	m.cmd = command
	m.connected = false
	m.mu.Unlock()

	if err := command.Start(); err != nil {
		m.setStatus(Errored, err.Error())
		return err
	}
	// Tie cloudflared to our lifetime: if the app exits or is killed, Windows
	// kills it too instead of leaving an orphan connector on the tunnel.
	if err := bindToProcessLifetime(command.Process.Pid); err != nil {
		fmt.Fprintln(os.Stderr, "tunnel: job object:", err)
	}

	var wg sync.WaitGroup
	wg.Add(2)
	go m.scan(&wg, stdout)
	go m.scan(&wg, stderr)
	wg.Wait()

	runErr := command.Wait()
	if ctx.Err() != nil {
		m.setStatus(Stopped, "detenido")
		return nil
	}
	if runErr != nil {
		m.setStatus(Errored, runErr.Error())
		return runErr
	}
	m.setStatus(Stopped, "detenido")
	return nil
}

// Stop terminates the sidecar.
func (m *Manager) Stop() {
	m.mu.Lock()
	command := m.cmd
	m.mu.Unlock()
	if command != nil && command.Process != nil {
		_ = command.Process.Kill()
	}
}

func (m *Manager) scan(wg *sync.WaitGroup, pipe interface{ Read([]byte) (int, error) }) {
	defer wg.Done()
	scanner := bufio.NewScanner(pipe)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		// Surface cloudflared warnings and errors in the app log (Ajustes).
		if strings.Contains(line, " ERR ") || strings.Contains(line, " WRN ") {
			fmt.Fprintln(os.Stderr, "cloudflared:", line)
		}
		if strings.Contains(line, "Registered tunnel connection") || strings.Contains(line, "Connection registered") {
			m.setStatus(Running, "activo")
		}
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

func (m *Manager) findTunnel(ctx context.Context) (string, error) {
	output, err := m.output(ctx, "tunnel", "list", "--output", "json")
	if err != nil {
		return "", fmt.Errorf("tunnel list: %w", err)
	}
	var tunnels []struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(output, &tunnels); err != nil {
		return "", fmt.Errorf("tunnel list parse: %w", err)
	}
	for _, tunnel := range tunnels {
		if tunnel.Name == m.cfg.Name {
			return tunnel.ID, nil
		}
	}
	return "", nil
}

func (m *Manager) run(ctx context.Context, args ...string) error {
	command := hideWindow(exec.CommandContext(ctx, m.binary, args...))
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (m *Manager) output(ctx context.Context, args ...string) ([]byte, error) {
	return hideWindow(exec.CommandContext(ctx, m.binary, args...)).Output()
}

// hideWindow stops Windows from opening a console for the child process
// (cloudflared is a console program) when the app runs as a GUI with no console.
func hideWindow(cmd *exec.Cmd) *exec.Cmd {
	const createNoWindow = 0x08000000
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
	return cmd
}

// certificatePath is where cloudflared writes the account certificate after a
// one-time login.
func certificatePath(home string) string {
	return filepath.Join(home, "cert.pem")
}

// loggedIn reports whether the account certificate exists and is not empty.
func loggedIn(home string) bool {
	info, err := os.Stat(certificatePath(home))
	return err == nil && !info.IsDir() && info.Size() > 0
}

func findCloudflared(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	if path, err := exec.LookPath("cloudflared"); err == nil {
		return path, nil
	}
	candidates := []string{
		`C:\Program Files (x86)\cloudflared\cloudflared.exe`,
		`C:\Program Files\cloudflared\cloudflared.exe`,
	}
	if local := os.Getenv("LOCALAPPDATA"); local != "" {
		candidates = append(candidates, filepath.Join(local, "cloudflared", "cloudflared.exe"))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("cloudflared not found; install it or set CLOUDFLARED_PATH")
}
