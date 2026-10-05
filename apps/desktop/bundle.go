//go:build windows

package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// bundleFS carries the pieces that make the exe self-contained. The build
// script (scripts/build-desktop.ps1) drops them in assets/bundle before
// `go build`; a plain `go build` without them still works and falls back to
// the dev layout on disk.
//
//   - panel.html         the built Svelte panel (apps/panel/dist/index.html)
//   - cloudflared.exe.gz the Cloudflare tunnel connector, gzipped (52 MB raw,
//     about half that compressed), so the exe stays smaller
//
//go:embed all:assets/bundle
var bundleFS embed.FS

func bundled(name string) []byte {
	raw, err := fs.ReadFile(bundleFS, "assets/bundle/"+name)
	if err != nil {
		return nil
	}
	return raw
}

// bundledCloudflared writes the embedded cloudflared.exe to the data directory
// (once per payload version) and returns its path, or "" when the exe has
// none.
func bundledCloudflared(dataDir string) string {
	raw := bundled("cloudflared.exe.gz")
	if len(raw) == 0 {
		return ""
	}
	path := filepath.Join(dataDir, "bin", "cloudflared.exe")
	if err := deployPayload(raw, path); err != nil {
		fmt.Fprintln(os.Stderr, "cloudflared:", err)
		return ""
	}
	return path
}

// deployPayload gunzips raw into path unless the same payload was already
// deployed. A sha256 marker next to the exe identifies the payload version, so
// a restart does not rewrite (and does not fail against) a running connector.
func deployPayload(raw []byte, path string) error {
	sum := sha256.Sum256(raw)
	marker := path + ".sha256"
	want := hex.EncodeToString(sum[:])
	if current, err := os.ReadFile(marker); err == nil && strings.TrimSpace(string(current)) == want {
		if _, err := os.Stat(path); err == nil {
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	temporary := path + ".tmp"
	if err := writeGunzip(raw, temporary); err != nil {
		os.Remove(temporary)
		return err
	}
	if err := os.Rename(temporary, path); err != nil {
		os.Remove(temporary)
		// A running connector keeps the old file locked; it is still usable.
		if _, statErr := os.Stat(path); statErr == nil {
			return nil
		}
		return err
	}
	// Best effort: the marker only saves a rewrite on the next start.
	_ = os.WriteFile(marker, []byte(want), 0o644)
	return nil
}

// writeGunzip decompresses raw into the file at path.
func writeGunzip(raw []byte, path string) error {
	reader, err := gzip.NewReader(bytes.NewReader(raw))
	if err != nil {
		return err
	}
	defer reader.Close()
	out, err := os.OpenFile(path, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o755)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, reader); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
