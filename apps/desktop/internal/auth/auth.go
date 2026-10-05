// Package auth manages spectator access to the panel. The host is the machine
// itself (a direct local request); spectators redeem a one-time invite for a
// long-lived session cookie. Tokens are stored hashed, never in clear (D27).
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Roles.
const (
	RoleHost      = "host"
	RoleSpectator = "spectator"
)

// Errors returned by the manager.
var (
	ErrInvalidToken = fmt.Errorf("auth: invalid or expired token")
	ErrUnknownID    = fmt.Errorf("auth: unknown id")
)

// Invite is a one-time link the host shares with a spectator.
type Invite struct {
	ID        string     `json:"id"`
	TokenHash string     `json:"tokenHash"`
	Label     string     `json:"label"`
	CreatedAt time.Time  `json:"createdAt"`
	ExpiresAt time.Time  `json:"expiresAt"`
	UsedAt    *time.Time `json:"usedAt,omitempty"`
}

// Session is an authenticated spectator.
type Session struct {
	ID        string    `json:"id"`
	TokenHash string    `json:"tokenHash"`
	Label     string    `json:"label"`
	Role      string    `json:"role"`
	CreatedAt time.Time `json:"createdAt"`
	LastSeen  time.Time `json:"lastSeen"`
	ExpiresAt time.Time `json:"expiresAt"`
}

type state struct {
	Invites  []Invite  `json:"invites"`
	Sessions []Session `json:"sessions"`
}

// Manager owns the invites and sessions, persisted to one JSON file.
type Manager struct {
	path  string
	now   func() time.Time
	state state
}

// New loads the store. A missing file starts empty.
func New(path string) (*Manager, error) {
	manager := &Manager{path: path, now: time.Now}
	raw, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return manager, nil
		}
		return nil, err
	}
	if err := json.Unmarshal(raw, &manager.state); err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}
	manager.prune()
	return manager, nil
}

// CreateInvite mints a one-time token and returns it once. Only its hash is
// stored.
func (m *Manager) CreateInvite(label string, ttl time.Duration) (Invite, string, error) {
	token, hash, err := newToken()
	if err != nil {
		return Invite{}, "", err
	}
	now := m.now()
	invite := Invite{
		ID:        newID(),
		TokenHash: hash,
		Label:     label,
		CreatedAt: now,
		ExpiresAt: now.Add(ttl),
	}
	m.state.Invites = append(m.state.Invites, invite)
	if err := m.save(); err != nil {
		return Invite{}, "", err
	}
	return invite, token, nil
}

// Redeem consumes an invite and creates a spectator session. It returns the
// session and the raw session token (the cookie value).
func (m *Manager) Redeem(token, label string) (Session, string, error) {
	m.prune()
	index := m.inviteIndex(token)
	if index < 0 {
		return Session{}, "", ErrInvalidToken
	}
	now := m.now()
	m.state.Invites[index].UsedAt = &now

	sessionToken, hash, err := newToken()
	if err != nil {
		return Session{}, "", err
	}
	if label == "" {
		label = m.state.Invites[index].Label
	}
	session := Session{
		ID:        newID(),
		TokenHash: hash,
		Label:     label,
		Role:      RoleSpectator,
		CreatedAt: now,
		LastSeen:  now,
		ExpiresAt: now.Add(sessionTTL),
	}
	m.state.Sessions = append(m.state.Sessions, session)
	if err := m.save(); err != nil {
		return Session{}, "", err
	}
	return session, sessionToken, nil
}

// Authenticate returns the session for a raw token. It updates LastSeen and
// persists at most once a minute.
func (m *Manager) Authenticate(token string) (Session, bool) {
	if token == "" {
		return Session{}, false
	}
	hash := hashToken(token)
	now := m.now()
	changed := false
	var found Session
	for i := range m.state.Sessions {
		session := &m.state.Sessions[i]
		if subtle.ConstantTimeCompare([]byte(session.TokenHash), []byte(hash)) != 1 {
			continue
		}
		if !session.ExpiresAt.IsZero() && now.After(session.ExpiresAt) {
			break
		}
		found = *session
		if now.Sub(session.LastSeen) > time.Minute {
			session.LastSeen = now
			changed = true
		}
		break
	}
	if found.ID == "" {
		return Session{}, false
	}
	if changed {
		_ = m.save()
	}
	return found, true
}

// ListInvites returns the pending invites without their token hashes.
func (m *Manager) ListInvites() []Invite {
	m.prune()
	out := make([]Invite, 0, len(m.state.Invites))
	for _, invite := range m.state.Invites {
		invite.TokenHash = ""
		out = append(out, invite)
	}
	return out
}

// RevokeInvite removes an invite by id.
func (m *Manager) RevokeInvite(id string) error {
	for i := range m.state.Invites {
		if m.state.Invites[i].ID == id {
			m.state.Invites = append(m.state.Invites[:i], m.state.Invites[i+1:]...)
			return m.save()
		}
	}
	return ErrUnknownID
}

// ListSessions returns the active sessions without their token hashes.
func (m *Manager) ListSessions() []Session {
	m.prune()
	out := make([]Session, 0, len(m.state.Sessions))
	for _, session := range m.state.Sessions {
		session.TokenHash = ""
		out = append(out, session)
	}
	return out
}

// RevokeSession removes a session by id, logging that spectator out.
func (m *Manager) RevokeSession(id string) error {
	for i := range m.state.Sessions {
		if m.state.Sessions[i].ID == id {
			m.state.Sessions = append(m.state.Sessions[:i], m.state.Sessions[i+1:]...)
			return m.save()
		}
	}
	return ErrUnknownID
}

const sessionTTL = 365 * 24 * time.Hour

// inviteIndex returns the index of a valid, unused invite for a token, or -1.
func (m *Manager) inviteIndex(token string) int {
	hash := hashToken(token)
	now := m.now()
	for i := range m.state.Invites {
		invite := &m.state.Invites[i]
		if invite.UsedAt != nil {
			continue
		}
		if !invite.ExpiresAt.IsZero() && now.After(invite.ExpiresAt) {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(invite.TokenHash), []byte(hash)) == 1 {
			return i
		}
	}
	return -1
}

// prune drops expired invites and sessions.
func (m *Manager) prune() {
	now := m.now()
	invites := m.state.Invites[:0]
	for _, invite := range m.state.Invites {
		expired := !invite.ExpiresAt.IsZero() && now.After(invite.ExpiresAt)
		if invite.UsedAt == nil && !expired {
			invites = append(invites, invite)
		}
	}
	m.state.Invites = invites

	sessions := m.state.Sessions[:0]
	for _, session := range m.state.Sessions {
		expired := !session.ExpiresAt.IsZero() && now.After(session.ExpiresAt)
		if !expired {
			sessions = append(sessions, session)
		}
	}
	m.state.Sessions = sessions
}

func (m *Manager) save() error {
	if err := os.MkdirAll(filepath.Dir(m.path), 0o755); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(m.state, "", "\t")
	if err != nil {
		return err
	}
	temporary := m.path + ".tmp"
	if err := os.WriteFile(temporary, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(temporary, m.path)
}

func newToken() (string, string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", err
	}
	token := base64.RawURLEncoding.EncodeToString(raw)
	return token, hashToken(token), nil
}

func newID() string {
	raw := make([]byte, 9)
	if _, err := rand.Read(raw); err != nil {
		return base64.RawURLEncoding.EncodeToString([]byte(time.Now().String()))
	}
	return hex.EncodeToString(raw)
}

func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
