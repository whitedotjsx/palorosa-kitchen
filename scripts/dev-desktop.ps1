#requires -Version 7
<#
  Dev watcher for the desktop app.

  Rebuilds the panel, rebuilds the Go exe, kills the running instance and
  relaunches it whenever a source file changes.

      pnpm dev:desktop
      pwsh -File scripts/dev-desktop.ps1
      pwsh -File scripts/dev-desktop.ps1 -Once   # build + relaunch once, no watch
#>
[CmdletBinding()]
param(
  [switch]$Once,
  [int]$PollMs = 1000
)

$ErrorActionPreference = 'Stop'

$root = Split-Path -Parent $PSScriptRoot
Set-Location $root

$exe = Join-Path $root 'apps\desktop\dist\palorosa-kitchen.exe'
$procName = 'palorosa-kitchen'
$watchRoots = @('apps\desktop', 'apps\panel\src')
$extensions = @('.go', '.svelte', '.ts', '.js', '.css', '.html', '.json', '.png', '.ico', '.woff2', '.webmanifest')
$exclude = '\\node_modules\\|\\dist\\|\\bundle\\|\\data\\|\\.git\\|\\.codegraph\\'

function Write-Step($text) { Write-Host "`n==> $text" -ForegroundColor Cyan }
function Write-Warn($text) { Write-Host "!!! $text" -ForegroundColor Yellow }

function Build-Desktop {
  # Panel + embedded bundle (panel.html, cloudflared.exe) + Go exe.
  try {
    & (Join-Path $PSScriptRoot 'build-desktop.ps1')
    return $true
  } catch {
    Write-Warn $_
    return $false
  }
}

function Stop-App {
  $procs = @(Get-Process -Name $procName -ErrorAction SilentlyContinue)
  if ($procs.Count -eq 0) { return }
  Write-Step "Stopping $($procs.Count) running instance(s)"
  $procs | ForEach-Object { try { $_.Kill() } catch { } }
  foreach ($i in 1..50) {
    if (-not (Get-Process -Name $procName -ErrorAction SilentlyContinue)) { return }
    Start-Sleep -Milliseconds 100
  }
  Write-Warn 'Old instance did not exit in time'
}

function Start-App {
  if (-not (Test-Path $exe)) { Write-Warn "Executable not found: $exe"; return }
  Start-Process -FilePath $exe | Out-Null
  Write-Host "==> Launched $exe" -ForegroundColor Green
}

function Restart-App {
  if (-not (Build-Desktop)) {
    Write-Warn 'Desktop build failed; keeping the previous instance'
    return
  }
  Stop-App
  Start-App
}

function Get-WatchStamp {
  $latest = [datetime]::MinValue
  foreach ($rel in $watchRoots) {
    $path = Join-Path $root $rel
    if (-not (Test-Path $path)) { continue }
    Get-ChildItem -Path $path -Recurse -File -ErrorAction SilentlyContinue |
      Where-Object { $_.FullName -notmatch $exclude -and $extensions -contains $_.Extension.ToLowerInvariant() } |
      ForEach-Object { if ($_.LastWriteTimeUtc -gt $latest) { $latest = $_.LastWriteTimeUtc } }
  }
  return $latest
}

Restart-App
if ($Once) { return }

Write-Host "`nWatching for changes (Ctrl+C to stop)..." -ForegroundColor DarkGray
$stamp = Get-WatchStamp
try {
  while ($true) {
    Start-Sleep -Milliseconds $PollMs
    $current = Get-WatchStamp
    if ($current -le $stamp) { continue }
    # Wait for the edits to settle before rebuilding.
    do {
      Start-Sleep -Milliseconds 400
      $stamp = $current
      $current = Get-WatchStamp
    } while ($current -gt $stamp)
    Restart-App
    $stamp = Get-WatchStamp
  }
} finally {
  Write-Host 'Watcher stopped.' -ForegroundColor DarkGray
}
