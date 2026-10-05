//go:build windows

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

// This file is the legacy path: an installed cloudflared can provision a named
// tunnel from the account certificate (login, create, route DNS) and run it as
// a fallback when the in-process connector library is not available.

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
	return m.writeConfigFile(config)
}

// writeConfigFile writes the config only when the content changes.
func (m *Manager) writeConfigFile(config string) error {
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

func (m *Manager) findTunnel(ctx context.Context) (string, error) {
	output, err := m.cloudflaredOutput(ctx, "tunnel", "list", "--output", "json")
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

func (m *Manager) runCloudflaredCommand(ctx context.Context, args ...string) error {
	command := hideWindow(exec.CommandContext(ctx, m.binary, args...))
	output, err := command.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%v: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (m *Manager) cloudflaredOutput(ctx context.Context, args ...string) ([]byte, error) {
	return hideWindow(exec.CommandContext(ctx, m.binary, args...)).Output()
}

// runCloudflared starts the cloudflared sidecar and blocks until the context
// is cancelled or the process exits. It is the fallback when the connector
// library is not available.
func (m *Manager) runCloudflared(ctx context.Context) error {
	m.setStatus(Starting, "conectando")
	args := []string{"--config", m.configYML, "--no-autoupdate", "tunnel", "run", m.cfg.Name}
	command := hideWindow(exec.CommandContext(ctx, m.binary, args...))
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

// findCloudflared locates the optional cloudflared binary: an explicit path,
// then PATH, then the usual install locations.
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
	return "", errors.New("cloudflared not found")
}
