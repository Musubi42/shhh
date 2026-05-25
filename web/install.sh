#!/usr/bin/env sh
#
# shhh installer — POSIX sh, downloads the latest release from GitHub.
#
# Usage:
#   curl -fsSL https://raw.githubusercontent.com/Musubi42/shhh/main/install.sh | sh
#
# Env overrides:
#   SHHH_VERSION   pin to a specific tag (e.g. v0.1.0); defaults to latest
#   SHHH_INSTALL   install dir; defaults to /usr/local/bin if writable,
#                  otherwise $HOME/.local/bin
#   SHHH_REPO      override GitHub repo (default Musubi42/shhh)

set -eu

REPO="${SHHH_REPO:-Musubi42/shhh}"

err() { printf 'shhh-install: %s\n' "$*" >&2; exit 1; }
say() { printf '==> %s\n' "$*"; }

need() {
  command -v "$1" >/dev/null 2>&1 || err "missing required command: $1"
}

need uname
need tar
need mkdir
need mv
need rm
# curl OR wget
if command -v curl >/dev/null 2>&1; then
  DL="curl -fsSL -o"
elif command -v wget >/dev/null 2>&1; then
  DL="wget -qO"
else
  err "need curl or wget"
fi

# shasum OR sha256sum
if command -v sha256sum >/dev/null 2>&1; then
  SHA="sha256sum"
elif command -v shasum >/dev/null 2>&1; then
  SHA="shasum -a 256"
else
  err "need sha256sum or shasum"
fi

# --- OS / arch detection ----------------------------------------------------

uname_s=$(uname -s)
uname_m=$(uname -m)

case "$uname_s" in
  Linux)  os="linux" ;;
  Darwin) os="macos" ;;
  *) err "unsupported OS: $uname_s (use install.ps1 on Windows)" ;;
esac

case "$uname_m" in
  x86_64|amd64) arch="x86_64" ;;
  arm64|aarch64) arch="arm64" ;;
  *) err "unsupported arch: $uname_m" ;;
esac

# goreleaser archive name template:
#   shhh_<version-no-v>_<os>_<arch>.tar.gz
# where os is "linux" or "macos", arch is "x86_64" or "arm64".

# --- resolve version --------------------------------------------------------

if [ -n "${SHHH_VERSION:-}" ]; then
  tag="$SHHH_VERSION"
else
  say "resolving latest release from github.com/$REPO"
  tag=$($DL - "https://api.github.com/repos/$REPO/releases/latest" 2>/dev/null \
        | grep -E '"tag_name"' \
        | head -n 1 \
        | sed -E 's/.*"tag_name"[[:space:]]*:[[:space:]]*"([^"]+)".*/\1/')
  [ -n "$tag" ] || err "could not determine latest release tag"
fi
version="${tag#v}"

say "installing shhh $tag for $os/$arch"

# --- pick install dir -------------------------------------------------------

if [ -n "${SHHH_INSTALL:-}" ]; then
  install_dir="$SHHH_INSTALL"
elif [ -w /usr/local/bin ] 2>/dev/null; then
  install_dir="/usr/local/bin"
else
  install_dir="$HOME/.local/bin"
fi
mkdir -p "$install_dir" || err "cannot create $install_dir"

# --- download + verify ------------------------------------------------------

tmp=$(mktemp -d 2>/dev/null || mktemp -d -t shhh-install)
trap 'rm -rf "$tmp"' EXIT

archive="shhh_${version}_${os}_${arch}.tar.gz"
base="https://github.com/$REPO/releases/download/$tag"

say "downloading $archive"
$DL "$tmp/$archive" "$base/$archive" || err "failed to download $archive"

say "downloading checksums.txt"
$DL "$tmp/checksums.txt" "$base/checksums.txt" || err "failed to download checksums.txt"

say "verifying sha256"
expected=$(grep "  $archive\$" "$tmp/checksums.txt" | awk '{print $1}')
[ -n "$expected" ] || err "no checksum for $archive in checksums.txt"
actual=$(cd "$tmp" && $SHA "$archive" | awk '{print $1}')
[ "$expected" = "$actual" ] || err "checksum mismatch: expected $expected got $actual"

say "extracting"
tar -xzf "$tmp/$archive" -C "$tmp"

mv "$tmp/shhh" "$install_dir/shhh"
chmod +x "$install_dir/shhh"

say "installed $install_dir/shhh"

# PATH hint
case ":$PATH:" in
  *":$install_dir:"*) ;;
  *)
    printf '\n'
    printf 'NOTE: %s is not on your $PATH.\n' "$install_dir"
    printf 'Add this to your shell rc file:\n'
    printf '  export PATH="%s:$PATH"\n' "$install_dir"
    ;;
esac

printf '\n'
printf 'Next:\n'
printf '  shhh install claude-code   # wire shhh into Claude Code\n'
printf '  shhh help                  # see all commands\n'
