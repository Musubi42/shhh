# shhh installer — Windows PowerShell.
#
# Usage:
#   irm https://raw.githubusercontent.com/Musubi42/shhh/main/install.ps1 | iex
#
# Env overrides:
#   $env:SHHH_VERSION  pin to a tag (e.g. v0.1.0); defaults to latest
#   $env:SHHH_INSTALL  install dir; defaults to $env:LOCALAPPDATA\Programs\shhh
#   $env:SHHH_REPO     override repo (default Musubi42/shhh)

$ErrorActionPreference = 'Stop'
Set-StrictMode -Version Latest

function Say($msg) { Write-Host "==> $msg" }
function Die($msg) { Write-Error "shhh-install: $msg"; exit 1 }

$repo = if ($env:SHHH_REPO) { $env:SHHH_REPO } else { 'Musubi42/shhh' }

# --- arch detection --------------------------------------------------------

$arch = switch -Regex ($env:PROCESSOR_ARCHITECTURE) {
    'AMD64' { 'x86_64' }
    'ARM64' { 'arm64' }
    default { Die "unsupported arch: $env:PROCESSOR_ARCHITECTURE" }
}

# --- resolve version -------------------------------------------------------

if ($env:SHHH_VERSION) {
    $tag = $env:SHHH_VERSION
} else {
    Say "resolving latest release from github.com/$repo"
    try {
        $tag = (Invoke-RestMethod "https://api.github.com/repos/$repo/releases/latest").tag_name
    } catch {
        Die "could not resolve latest release: $_"
    }
}
$version = $tag.TrimStart('v')

Say "installing shhh $tag for windows/$arch"

# --- install dir -----------------------------------------------------------

$installDir = if ($env:SHHH_INSTALL) {
    $env:SHHH_INSTALL
} else {
    Join-Path $env:LOCALAPPDATA 'Programs\shhh'
}
New-Item -ItemType Directory -Force -Path $installDir | Out-Null

# --- download + verify -----------------------------------------------------

$tmp = New-Item -ItemType Directory -Force -Path (Join-Path $env:TEMP "shhh-install-$([guid]::NewGuid())")
try {
    $archive = "shhh_${version}_windows_${arch}.zip"
    $base = "https://github.com/$repo/releases/download/$tag"

    Say "downloading $archive"
    Invoke-WebRequest -Uri "$base/$archive" -OutFile (Join-Path $tmp $archive) -UseBasicParsing

    Say "downloading checksums.txt"
    Invoke-WebRequest -Uri "$base/checksums.txt" -OutFile (Join-Path $tmp 'checksums.txt') -UseBasicParsing

    Say "verifying sha256"
    $expectedLine = Select-String -Path (Join-Path $tmp 'checksums.txt') -Pattern "  $archive$" | Select-Object -First 1
    if (-not $expectedLine) { Die "no checksum entry for $archive" }
    $expected = ($expectedLine.Line -split '\s+')[0]
    $actual = (Get-FileHash -Path (Join-Path $tmp $archive) -Algorithm SHA256).Hash.ToLower()
    if ($expected -ne $actual) { Die "checksum mismatch: expected $expected got $actual" }

    Say "extracting"
    Expand-Archive -Path (Join-Path $tmp $archive) -DestinationPath $tmp -Force

    $dst = Join-Path $installDir 'shhh.exe'
    Move-Item -Force -Path (Join-Path $tmp 'shhh.exe') -Destination $dst

    Say "installed $dst"
}
finally {
    Remove-Item -Recurse -Force $tmp -ErrorAction SilentlyContinue
}

# PATH hint
$userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
if ($userPath -notlike "*$installDir*") {
    Write-Host ""
    Write-Host "NOTE: $installDir is not on your user PATH."
    Write-Host "Add it for this session:"
    Write-Host "  `$env:Path = `"$installDir;`$env:Path`""
    Write-Host "Or persistently:"
    Write-Host "  [Environment]::SetEnvironmentVariable('Path', `"$installDir;`" + [Environment]::GetEnvironmentVariable('Path','User'), 'User')"
}

Write-Host ""
Write-Host "Next:"
Write-Host "  shhh install claude-code   # wire shhh into Claude Code"
Write-Host "  shhh help                  # see all commands"
