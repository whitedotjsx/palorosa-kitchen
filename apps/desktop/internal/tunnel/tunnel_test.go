//go:build windows

package tunnel

import (
	"os"
	"path/filepath"
	"testing"
)

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
