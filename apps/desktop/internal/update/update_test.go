package update

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// githubStub serves the release API and the asset download.
type githubStub struct {
	server  *httptest.Server
	content []byte
	tag     string
	digest  string
	missing bool
	present bool
}

// newGithubStub starts a fake GitHub: /repos/o/r/releases/latest plus the
// asset URL the payload advertises.
func newGithubStub(t *testing.T, content []byte, tag string, digest string) *githubStub {
	t.Helper()
	stub := &githubStub{content: content, tag: tag, digest: digest}
	mux := http.NewServeMux()
	mux.HandleFunc("/repos/o/r/releases/latest", func(w http.ResponseWriter, r *http.Request) {
		if stub.missing {
			http.NotFound(w, r)
			return
		}
		if stub.present {
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		payload := map[string]any{
			"tag_name":     tag,
			"body":         "notas",
			"published_at": "2026-10-01T00:00:00Z",
			"assets": []map[string]any{{
				"name":                 "Palorosa-Kitchen.EXE",
				"browser_download_url": stub.server.URL + "/asset",
				"size":                 len(content),
				"digest":               digest,
			}},
		}
		_ = json.NewEncoder(w).Encode(payload)
	})
	mux.HandleFunc("/asset", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(content)
	})
	server := httptest.NewServer(mux)
	stub.server = server
	t.Cleanup(server.Close)
	return stub
}

func testManager(t *testing.T, stub *githubStub, exe string, restart *int) *Manager {
	t.Helper()
	return New(Config{
		Repo:    "o/r",
		Current: "0.1.0",
		APIBase: stub.server.URL,
		ExePath: exe,
		HTTP:    stub.server.Client(),
		Restart: func() error {
			if restart != nil {
				*restart++
			}
			return nil
		},
		Logger: log.New(io.Discard, "", 0),
	})
}

func writeExe(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "palorosa-kitchen.exe")
	if err := os.WriteFile(path, []byte(content), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"0.1.0", "0.1.0", 0},
		{"0.1.0", "v0.1.0", 0},
		{"0.1.0", "0.1.1", -1},
		{"0.2.0", "0.1.9", 1},
		{"1.0.0", "0.9.9", 1},
		{"0.1", "0.1.0", 0},
		{"0.1.0", "0.1.0-rc1", 1},
		{"0.1.0-beta", "0.1.0-rc1", -1},
		{"junk", "0.0.0", 0},
	}
	for _, test := range cases {
		if got := CompareVersions(test.a, test.b); got != test.want {
			t.Errorf("CompareVersions(%q, %q) = %d, want %d", test.a, test.b, got, test.want)
		}
	}
}

func TestCheckFindsNewerRelease(t *testing.T) {
	stub := newGithubStub(t, []byte("new"), "v0.2.0", "")
	manager := testManager(t, stub, writeExe(t, "old"), nil)

	release, newer, err := manager.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !newer || release.Version != "0.2.0" || release.Tag != "v0.2.0" {
		t.Fatalf("release = %+v newer = %v", release, newer)
	}
	if release.AssetName != "Palorosa-Kitchen.EXE" || release.AssetURL == "" {
		t.Fatalf("asset = %+v", release)
	}
	state := manager.State()
	if !state.Available || state.Latest != "0.2.0" || state.Status != StatusAvailable {
		t.Fatalf("state = %+v", state)
	}
}

func TestCheckUpToDate(t *testing.T) {
	stub := newGithubStub(t, []byte("new"), "v0.1.0", "")
	manager := testManager(t, stub, writeExe(t, "old"), nil)

	_, newer, err := manager.Check(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if newer {
		t.Fatal("same version reported as newer")
	}
	if state := manager.State(); state.Available || state.Status != StatusUpToDate {
		t.Fatalf("state = %+v", state)
	}
}

func TestCheckWithoutReleases(t *testing.T) {
	stub := newGithubStub(t, []byte("new"), "v0.2.0", "")
	stub.missing = true
	manager := testManager(t, stub, writeExe(t, "old"), nil)

	_, newer, err := manager.Check(context.Background())
	if err != nil || newer {
		t.Fatalf("newer = %v, err = %v", newer, err)
	}
	if state := manager.State(); state.Status != StatusUpToDate {
		t.Fatalf("state = %+v", state)
	}
}

func TestCheckReportsAPIError(t *testing.T) {
	stub := newGithubStub(t, []byte("new"), "v0.2.0", "")
	stub.present = true
	manager := testManager(t, stub, writeExe(t, "old"), nil)

	_, _, err := manager.Check(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if state := manager.State(); state.Status != StatusError || state.Error == "" {
		t.Fatalf("state = %+v", state)
	}
}

func TestUpdateReplacesExecutable(t *testing.T) {
	content := []byte("fresh-exe-content")
	sum := sha256.Sum256(content)
	stub := newGithubStub(t, content, "v0.2.0", "sha256:"+hex.EncodeToString(sum[:]))
	exe := writeExe(t, "old-exe-content")
	restarts := 0
	manager := testManager(t, stub, exe, &restarts)

	release, updated, err := manager.Update(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !updated || release.Version != "0.2.0" {
		t.Fatalf("updated = %v release = %+v", updated, release)
	}
	if got := readFile(t, exe); got != string(content) {
		t.Fatalf("exe = %q, want the downloaded content", got)
	}
	if got := readFile(t, exe+".old"); got != "old-exe-content" {
		t.Fatalf("old = %q", got)
	}
	if restarts != 1 {
		t.Fatalf("restarts = %d, want 1", restarts)
	}
	if state := manager.State(); state.Status != StatusUpdated || state.Available || state.UpdatedAt.IsZero() {
		t.Fatalf("state = %+v", state)
	}
}

func TestUpdateRejectsBadDigest(t *testing.T) {
	content := []byte("tampered")
	stub := newGithubStub(t, content, "v0.2.0", "sha256:"+strings.Repeat("0", 64))
	exe := writeExe(t, "old")
	manager := testManager(t, stub, exe, nil)

	_, updated, err := manager.Update(context.Background())
	if err == nil || updated {
		t.Fatalf("updated = %v, err = %v", updated, err)
	}
	if got := readFile(t, exe); got != "old" {
		t.Fatalf("exe changed to %q", got)
	}
	if _, statErr := os.Stat(exe + ".new"); !os.IsNotExist(statErr) {
		t.Fatal("downloaded file was not removed")
	}
	if state := manager.State(); state.Status != StatusError {
		t.Fatalf("state = %+v", state)
	}
}

func TestUpdateUpToDateDoesNotDownload(t *testing.T) {
	stub := newGithubStub(t, []byte("new"), "v0.1.0", "")
	exe := writeExe(t, "old")
	manager := testManager(t, stub, exe, nil)

	_, updated, err := manager.Update(context.Background())
	if err != nil || updated {
		t.Fatalf("updated = %v, err = %v", updated, err)
	}
	if got := readFile(t, exe); got != "old" {
		t.Fatalf("exe = %q", got)
	}
}

func TestReplaceRestoresOnFailure(t *testing.T) {
	exe := writeExe(t, "old")
	err := Replace(filepath.Join(t.TempDir(), "missing.new"), exe)
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := readFile(t, exe); got != "old" {
		t.Fatalf("exe = %q, want the original content", got)
	}
}

func TestCleanupRemovesLeftovers(t *testing.T) {
	exe := writeExe(t, "old")
	for _, suffix := range []string{".old", ".new"} {
		if err := os.WriteFile(exe+suffix, []byte("x"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	Cleanup(exe)
	for _, suffix := range []string{".old", ".new"} {
		if _, err := os.Stat(exe + suffix); !os.IsNotExist(err) {
			t.Fatalf("%s not removed", suffix)
		}
	}
	if _, err := os.Stat(exe); err != nil {
		t.Fatal("cleanup removed the executable")
	}
}
