#requires -Version 7
<#
  Builds the self-contained desktop exe (apps/desktop/dist/palorosa-kitchen.exe).

  1. Builds the Svelte panel and copies it into apps/desktop/assets/bundle.
  2. Downloads cloudflared.exe (once; -RefreshCloudflared forces a new download
     of the latest release), gzips it into the same folder and drops the raw
     copy, so the embedded connector is about half its size.
  3. Builds the Go exe, stripped (-s -w) and with both files embedded.

      pnpm desktop:build
      pwsh -File scripts/build-desktop.ps1 [-SkipPanel] [-RefreshCloudflared]
#>
[CmdletBinding()]
param(
  [switch]$SkipPanel,
  [switch]$RefreshCloudflared,
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
$cloudflared = Join-Path $bundle 'cloudflared.exe'
$cloudflaredGz = Join-Path $bundle 'cloudflared.exe.gz'
$cloudflaredUrl = 'https://github.com/cloudflare/cloudflared/releases/latest/download/cloudflared-windows-amd64.exe'

function Write-Step($text) { Write-Host "`n==> $text" -ForegroundColor Cyan }

function Compress-Gzip([string]$source, [string]$target) {
  $input = [System.IO.File]::OpenRead($source)
  try {
    $output = [System.IO.File]::Create($target)
    try {
      $gzip = [System.IO.Compression.GZipStream]::new($output, [System.IO.Compression.CompressionLevel]::SmallestSize, $false)
      try { $input.CopyTo($gzip) } finally { $gzip.Dispose() }
    } finally { $output.Dispose() }
  } finally { $input.Dispose() }
}

New-Item -ItemType Directory -Force -Path $bundle | Out-Null

if (-not $SkipPanel) {
  Write-Step 'Building panel'
  & pnpm --dir $root --filter '@palorosa-kitchen/panel' run build
  if ($LASTEXITCODE -ne 0) { throw 'Panel build failed' }
}
if (-not (Test-Path $panelHtml)) { throw "Panel not built: $panelHtml" }
Copy-Item -Force $panelHtml (Join-Path $bundle 'panel.html')

if ($RefreshCloudflared -or (-not (Test-Path $cloudflared) -and -not (Test-Path $cloudflaredGz))) {
  Write-Step 'Downloading cloudflared'
  $temporary = "$cloudflared.download"
  Invoke-WebRequest -Uri $cloudflaredUrl -OutFile $temporary
  Move-Item -Force $temporary $cloudflared
}

if (Test-Path $cloudflared) {
  Write-Step 'Compressing cloudflared'
  $temporary = "$cloudflaredGz.download"
  Compress-Gzip $cloudflared $temporary
  Move-Item -Force $temporary $cloudflaredGz
  Remove-Item -Force $cloudflared
}
if (-not (Test-Path $cloudflaredGz)) { throw "cloudflared payload missing: $cloudflaredGz" }

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
