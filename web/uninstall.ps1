# shhh uninstaller — Windows PowerShell.
#
# Usage:
#   irm https://musubi42.github.io/shhh/uninstall.ps1 | iex
#
# Env overrides:
#   $env:SHHH_KEEP_DATA = '1'   keep %USERPROFILE%\.shhh

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Say($msg) { Write-Host "==> $msg" }

$binPath = (Get-Command shhh -ErrorAction SilentlyContinue)?.Source

if (-not $binPath) {
    Say "no shhh binary on PATH — nothing to remove"
} else {
    if (Test-Path "$env:USERPROFILE\.claude\settings.json") {
        Say "removing shhh hook from Claude Code"
        try { & $binPath uninstall claude-code } catch { Say "(hook removal failed — continuing)" }
    }

    Say "removing binary at $binPath"
    Remove-Item -Force $binPath
}

$dataDir = Join-Path $env:USERPROFILE '.shhh'
if ($env:SHHH_KEEP_DATA -eq '1') {
    Say "SHHH_KEEP_DATA=1 — keeping $dataDir"
} elseif (Test-Path $dataDir) {
    Say "removing $dataDir (session cache + audit log)"
    Remove-Item -Recurse -Force $dataDir
}

Say "done. shhh is uninstalled."
Say "if it was installed via 'go install', also remove: \$env:GOPATH\bin\shhh.exe"
