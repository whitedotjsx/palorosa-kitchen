#requires -Version 7
<#
  Builds the self-contained desktop exe (apps/desktop/dist/palorosa-kitchen.exe).

  1. Builds the Svelte panel and copies it into apps/desktop/assets/bundle.
  2. Builds the vendored cf-quick-tunnel connector (Rust, static CRT) and
     copies it into the same folder as cf-tunnel.dll.
  3. Builds the Go exe, stripped (-s -w) and with both files embedded.

      pnpm desktop:build
      pwsh -File scripts/build-desktop.ps1 [-SkipPanel] [-RefreshTunnel]
#>
[CmdletBinding()]
param(
  [switch]$SkipPanel,
  # Rebuilds the Rust connector from scratch (cargo clean + build).
  [switch]$RefreshTunnel,
  # Release version (the git tag without the leading v). It feeds the exe
  # metadata and the updater comparison.
  [string]$Version = '0.1.0'
)

$ErrorActionPreference = 'Stop'

$semver = $Version.TrimStart('v')
if ($semver -notmatch '^\d+\.\d+\.\d+$') { throw "Version '$Version' is not x.y.z" }

$root = Split-Path -Parent $PSScriptRoot
$bundle = Join-Path $root 'apps\desktop\assets\bundle'
$panelHtml = Join-Path $root 'apps\panel\dist\index.html'
$tunnelCrate = Join-Path $root 'apps\desktop\third_party\cf-quick-tunnel-rs'
$tunnelDll = Join-Path $tunnelCrate 'target\release\cloudflare_quick_tunnel.dll'
$tunnelBundle = Join-Path $bundle 'cf-tunnel.dll'

function Write-Step($text) { Write-Host "`n==> $text" -ForegroundColor Cyan }

New-Item -ItemType Directory -Force -Path $bundle | Out-Null
# Older builds embedded the cloudflared binary; drop any leftover payload so
# it does not get embedded into the exe again.
Remove-Item -Force (Join-Path $bundle 'cloudflared.exe.gz'), (Join-Path $bundle 'cloudflared.exe') -ErrorAction SilentlyContinue

if (-not $SkipPanel) {
  Write-Step 'Building panel'
  & pnpm --dir $root --filter '@palorosa-kitchen/panel' run build
  if ($LASTEXITCODE -ne 0) { throw 'Panel build failed' }
}
if (-not (Test-Path $panelHtml)) { throw "Panel not built: $panelHtml" }
Copy-Item -Force $panelHtml (Join-Path $bundle 'panel.html')

Write-Step 'Building the tunnel connector (Rust)'
if (-not (Get-Command cargo -ErrorAction SilentlyContinue)) {
  throw 'cargo not found; install Rust (https://rustup.rs) to build cf-tunnel.dll'
}
if ($RefreshTunnel) { & cargo clean --manifest-path (Join-Path $tunnelCrate 'Cargo.toml') }
# Static CRT: the DLL has no VCRUNTIME dependency, so it runs on any Windows.
# cargo is a no-op when the sources and the flags are unchanged.
$env:RUSTFLAGS = '-C target-feature=+crt-static'
try {
  & cargo build --release --lib --locked --manifest-path (Join-Path $tunnelCrate 'Cargo.toml')
  if ($LASTEXITCODE -ne 0) { throw 'Tunnel connector build failed' }
} finally {
  Remove-Item Env:\RUSTFLAGS -ErrorAction SilentlyContinue
}
if (-not (Test-Path $tunnelDll)) { throw "tunnel connector missing: $tunnelDll" }
Copy-Item -Force $tunnelDll $tunnelBundle

Write-Step 'Embedding exe icon (assets/panel-icon.png)'
Push-Location (Join-Path $root 'apps\desktop')
try {
  & go run github.com/tc-hib/go-winres@v0.3.3 simply --icon assets/panel-icon.png --manifest gui `
    --product-name 'Palorosa Kitchen' --file-description 'Palorosa Kitchen' `
    --product-version $semver --file-version $semver
  if ($LASTEXITCODE -ne 0) { throw 'go-winres failed' }
} finally {
  Pop-Location
}

Write-Step 'Building desktop exe'
# -s -w drops the DWARF symbols (about 22 MB); -H=windowsgui hides the console.
& go -C (Join-Path $root 'apps\desktop') build -ldflags="-s -w -H=windowsgui -X main.version=$semver" -o dist/palorosa-kitchen.exe .
if ($LASTEXITCODE -ne 0) { throw 'Desktop build failed' }

$size = (Get-Item (Join-Path $root 'apps\desktop\dist\palorosa-kitchen.exe')).Length / 1MB
Write-Host ("==> apps\desktop\dist\palorosa-kitchen.exe ({0:N1} MB)" -f $size) -ForegroundColor Green
