//go:build windows

// Package cftunnel loads the vendored cf-quick-tunnel shared library
// (third_party/cf-quick-tunnel-rs) and exposes its C ABI to Go. The library
// speaks QUIC + capnp-RPC to the Cloudflare edge, so the desktop app runs its
// named tunnel in process instead of spawning the cloudflared binary.
package cftunnel

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

// State mirrors the CFT_* constants of the shared library.
type State int32

const (
	Stopped  State = 0
	Starting State = 1
	Running  State = 2
	Errored  State = 3
	Stopping State = 4
)

// Materialize writes the built shared library to <dataDir>/bin/cf-tunnel.dll,
// once per payload version (a sha256 marker identifies it), and returns its
// path. The caller embeds the payload in the exe at build time.
func Materialize(dataDir string, payload []byte) (string, error) {
	if len(payload) == 0 {
		return "", errors.New("cftunnel: empty library payload")
	}
	path := filepath.Join(dataDir, "bin", "cf-tunnel.dll")
	sum := sha256.Sum256(payload)
	want := hex.EncodeToString(sum[:])
	marker := path + ".sha256"
	if current, err := os.ReadFile(marker); err == nil && strings.TrimSpace(string(current)) == want {
		if _, err := os.Stat(path); err == nil {
			return path, nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	temporary := path + ".tmp"
	if err := os.WriteFile(temporary, payload, 0o755); err != nil {
		return "", err
	}
	if err := os.Rename(temporary, path); err != nil {
		os.Remove(temporary)
		// A DLL loaded by this process stays locked; the previous file is
		// still usable, so keep it.
		if _, statErr := os.Stat(path); statErr == nil {
			return path, nil
		}
		return "", err
	}
	// Best effort: the marker only saves a rewrite on the next start.
	_ = os.WriteFile(marker, []byte(want), 0o644)
	return path, nil
}

// Tunnel is an open handle to the shared library.
type Tunnel struct {
	dll    *syscall.DLL
	handle uintptr

	newProc   *syscall.Proc
	startProc *syscall.Proc
	stopProc  *syscall.Proc
	stateProc *syscall.Proc
	errorProc *syscall.Proc
	urlProc   *syscall.Proc
	freeProc  *syscall.Proc

	mu   sync.Mutex
	dead bool
}

// Load opens the shared library at path and creates its tunnel handle.
func Load(path string) (*Tunnel, error) {
	dll, err := syscall.LoadDLL(path)
	if err != nil {
		return nil, fmt.Errorf("cftunnel: load %s: %w", filepath.Base(path), err)
	}
	tunnel := &Tunnel{dll: dll}
	procedures := []struct {
		target **syscall.Proc
		name   string
	}{
		{&tunnel.newProc, "cft_new"},
		{&tunnel.startProc, "cft_start"},
		{&tunnel.stopProc, "cft_stop"},
		{&tunnel.stateProc, "cft_state"},
		{&tunnel.errorProc, "cft_error_text"},
		{&tunnel.urlProc, "cft_url"},
		{&tunnel.freeProc, "cft_free"},
	}
	for _, procedure := range procedures {
		proc, err := dll.FindProc(procedure.name)
		if err != nil {
			_ = dll.Release()
			return nil, fmt.Errorf("cftunnel: %s: %w", procedure.name, err)
		}
		*procedure.target = proc
	}
	handle, _, callErr := tunnel.newProc.Call()
	if handle == 0 {
		_ = dll.Release()
		return nil, fmt.Errorf("cftunnel: cft_new: %v", callErr)
	}
	tunnel.handle = handle
	return tunnel, nil
}

// Start launches the named tunnel. credential is the dashboard token or the
// JSON of a credentials file; ingress is the YAML/JSON ingress list.
func (t *Tunnel) Start(credential, ingress string) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.dead {
		return errors.New("cftunnel: tunnel released")
	}
	credentialPtr, err := syscall.BytePtrFromString(credential)
	if err != nil {
		return err
	}
	ingressPtr, err := syscall.BytePtrFromString(ingress)
	if err != nil {
		return err
	}
	result, _, _ := t.startProc.Call(
		t.handle,
		uintptr(unsafe.Pointer(credentialPtr)),
		uintptr(unsafe.Pointer(ingressPtr)),
	)
	if State(result) == Errored {
		return errors.New("cftunnel: " + t.readTextLocked(t.errorProc))
	}
	return nil
}

// State returns the current tunnel state.
func (t *Tunnel) State() State {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.dead {
		return Stopped
	}
	result, _, _ := t.stateProc.Call(t.handle)
	return State(result)
}

// LastError returns the library's last error message.
func (t *Tunnel) LastError() string { return t.readText(t.errorProc) }

// URL returns the public tunnel URL once it is up.
func (t *Tunnel) URL() string { return t.readText(t.urlProc) }

func (t *Tunnel) readText(proc *syscall.Proc) string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.readTextLocked(proc)
}

// readTextLocked is readText for callers that already hold t.mu (Go mutexes
// are not reentrant).
func (t *Tunnel) readTextLocked(proc *syscall.Proc) string {
	if t.dead || proc == nil {
		return ""
	}
	var buffer [1024]byte
	length, _, _ := proc.Call(t.handle, uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)))
	if length == 0 {
		return ""
	}
	if length >= uintptr(len(buffer)) {
		length = uintptr(len(buffer) - 1)
	}
	return string(buffer[:int(length)])
}

// Stop requests a graceful shutdown of the tunnel.
func (t *Tunnel) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.dead {
		return
	}
	_, _, _ = t.stopProc.Call(t.handle)
}

// Free stops the tunnel and releases the handle and the library.
func (t *Tunnel) Free() {
	t.mu.Lock()
	if t.dead {
		t.mu.Unlock()
		return
	}
	_, _, _ = t.stopProc.Call(t.handle)
	_, _, _ = t.freeProc.Call(t.handle)
	t.dead = true
	dll := t.dll
	t.mu.Unlock()
	_ = dll.Release()
}
