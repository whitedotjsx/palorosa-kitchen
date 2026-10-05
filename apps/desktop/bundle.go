//go:build windows

package main

import (
	"embed"
	"io/fs"
)

// bundleFS carries the pieces that make the exe self-contained. The build
// script (scripts/build-desktop.ps1) drops them in assets/bundle before
// `go build`; a plain `go build` without them still works and falls back to
// the dev layout on disk.
//
//   - panel.html    the built Svelte panel (apps/panel/dist/index.html)
//   - cf-tunnel.dll the vendored cf-quick-tunnel connector
//     (third_party/cf-quick-tunnel-rs), built as a shared library so the
//     tunnel runs in process with no cloudflared subprocess
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
