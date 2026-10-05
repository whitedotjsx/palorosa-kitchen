//go:build !windows

// Package cftunnel is the non-Windows stub: the desktop app and its tunnel
// only target Windows.
package cftunnel

import "errors"

// State mirrors the CFT_* constants of the shared library.
type State int32

const (
	Stopped  State = 0
	Starting State = 1
	Running  State = 2
	Errored  State = 3
	Stopping State = 4
)

// Tunnel is the non-Windows stub.
type Tunnel struct{}

// Materialize is unavailable outside Windows.
func Materialize(string, []byte) (string, error) {
	return "", errors.New("cftunnel: windows only")
}

// Load is unavailable outside Windows.
func Load(string) (*Tunnel, error) {
	return nil, errors.New("cftunnel: windows only")
}

// Start is unavailable outside Windows.
func (t *Tunnel) Start(string, string) error { return errors.New("cftunnel: windows only") }

// State is unavailable outside Windows.
func (t *Tunnel) State() State { return Stopped }

// LastError is unavailable outside Windows.
func (t *Tunnel) LastError() string { return "windows only" }

// URL is unavailable outside Windows.
func (t *Tunnel) URL() string { return "" }

// Stop is a no-op outside Windows.
func (t *Tunnel) Stop() {}

// Free is a no-op outside Windows.
func (t *Tunnel) Free() {}
