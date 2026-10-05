package auth

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newManager(t *testing.T) *Manager {
	t.Helper()
	manager, err := New(filepath.Join(t.TempDir(), "sessions.json"))
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func TestInviteRedeemAuthenticate(t *testing.T) {
	manager := newManager(t)
	invite, token, err := manager.CreateInvite("Doña Marta", 24*time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	if invite.ID == "" || token == "" {
		t.Fatalf("empty invite: %+v", invite)
	}

	session, sessionToken, err := manager.Redeem(token, "")
	if err != nil {
		t.Fatal(err)
	}
	if session.Label != "Doña Marta" || session.Role != RoleSpectator {
		t.Fatalf("unexpected session: %+v", session)
	}

	got, ok := manager.Authenticate(sessionToken)
	if !ok || got.ID != session.ID {
		t.Fatalf("authenticate failed: %+v ok=%v", got, ok)
	}

	if _, _, err := manager.Redeem(token, ""); err == nil {
		t.Fatal("invite must be one-time")
	}
}

func TestPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	manager, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	_, inviteToken, err := manager.CreateInvite("x", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	_, sessionToken, err := manager.Redeem(inviteToken, "")
	if err != nil {
		t.Fatal(err)
	}

	reopened, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := reopened.Authenticate(sessionToken); !ok {
		t.Fatal("session lost after reopen")
	}
}

func TestRevoke(t *testing.T) {
	manager := newManager(t)
	_, inviteToken, _ := manager.CreateInvite("x", time.Hour)
	session, sessionToken, _ := manager.Redeem(inviteToken, "")

	if err := manager.RevokeSession(session.ID); err != nil {
		t.Fatal(err)
	}
	if _, ok := manager.Authenticate(sessionToken); ok {
		t.Fatal("revoked session still valid")
	}
	if err := manager.RevokeSession("missing"); err != ErrUnknownID {
		t.Fatalf("err = %v, want ErrUnknownID", err)
	}
}

func TestInviteRevoke(t *testing.T) {
	manager := newManager(t)
	invite, token, _ := manager.CreateInvite("x", time.Hour)
	if err := manager.RevokeInvite(invite.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := manager.Redeem(token, ""); err == nil {
		t.Fatal("revoked invite still redeemable")
	}
}

func TestExpiry(t *testing.T) {
	manager := newManager(t)
	now := time.Now()
	manager.now = func() time.Time { return now }

	_, token, err := manager.CreateInvite("x", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	if _, _, err := manager.Redeem(token, ""); err == nil {
		t.Fatal("expired invite redeemed")
	}
}

func TestTokensAreHashed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sessions.json")
	manager, err := New(path)
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := manager.CreateInvite("x", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(raw, []byte(token)) {
		t.Fatalf("invite token stored in clear:\n%s", raw)
	}
	if bytes.Contains(raw, []byte(hashToken(token))) == false {
		t.Fatalf("expected the token hash in the file:\n%s", raw)
	}
}

func TestListStripsHashes(t *testing.T) {
	manager := newManager(t)
	_, inviteToken, _ := manager.CreateInvite("x", time.Hour)
	_, _, _ = manager.Redeem(inviteToken, "")

	for _, invite := range manager.ListInvites() {
		if invite.TokenHash != "" {
			t.Fatalf("invite list leaked a hash: %+v", invite)
		}
	}
	for _, session := range manager.ListSessions() {
		if session.TokenHash != "" {
			t.Fatalf("session list leaked a hash: %+v", session)
		}
	}
}
