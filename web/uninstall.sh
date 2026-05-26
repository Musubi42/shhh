#!/usr/bin/env sh
#
# shhh uninstaller — POSIX sh.
#
# Usage:
#   curl -fsSL https://musubi42.github.io/shhh/uninstall.sh | sh
#
# What it does:
#   1. If shhh is wired into Claude Code, runs `shhh uninstall claude-code`
#      first so the hook entry is removed cleanly.
#   2. Deletes the shhh binary from your PATH.
#   3. Removes ~/.shhh (session cache, audit log).
#
# Env overrides:
#   SHHH_KEEP_DATA=1   skip removing ~/.shhh (keep cache + audit history)
#
# IMPORTANT: env vars must sit BEFORE the `sh` after the pipe, not before
# `curl`. `SHHH_KEEP_DATA=1 curl ... | sh` does NOT work — the var ends
# up in curl's env only. Correct invocation:
#
#   curl -fsSL https://musubi42.github.io/shhh/uninstall.sh | SHHH_KEEP_DATA=1 sh

set -eu

err() { printf 'shhh-uninstall: %s\n' "$*" >&2; exit 1; }
say() { printf '==> %s\n' "$*"; }

bin_path=$(command -v shhh 2>/dev/null || true)

if [ -z "$bin_path" ]; then
  say "no shhh binary on PATH — nothing to remove from /usr/local/bin or ~/.local/bin"
else
  # Try to detach from Claude Code first, while the binary still exists.
  if [ -f "$HOME/.claude/settings.json" ]; then
    say "removing shhh hook from Claude Code"
    "$bin_path" uninstall claude-code || say "(hook removal returned non-zero — continuing)"
  fi

  say "removing binary at $bin_path"
  if [ -w "$bin_path" ] || [ -w "$(dirname "$bin_path")" ]; then
    rm -f "$bin_path"
  else
    err "no write permission on $bin_path — re-run with: sudo rm $bin_path"
  fi
fi

if [ "${SHHH_KEEP_DATA:-0}" = "1" ]; then
  say "SHHH_KEEP_DATA=1 — keeping $HOME/.shhh"
elif [ -d "$HOME/.shhh" ]; then
  say "removing $HOME/.shhh (session cache + audit log)"
  rm -rf "$HOME/.shhh"
fi

say "done. shhh is uninstalled."
say "if it was installed via 'go install', also remove: \$GOPATH/bin/shhh"
