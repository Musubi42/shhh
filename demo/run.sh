#!/usr/bin/env bash
#
# shhh leak test — proof that shhh keeps secrets out of a real coding agent.
#
# This drives a REAL Claude Code session against demo/leaktest/ twice:
#   1. shhh OFF  — control. The agent reads the fixture's fake secrets;
#                  they land raw in the session transcript.
#   2. shhh ON   — the shhh hook is wired in via a project-scoped
#                  settings.json; the agent sees placeholders instead.
#
# Afterwards verify.py greps both transcripts against demo/manifest.json
# and prints a scorecard. Pass = 0 raw secrets in the shhh-ON transcript.
#
# This is NOT a simulation. `claude` actually runs and actually calls the
# API (~$0.03-0.10 per run with --model sonnet). The transcript on disk is
# the authority — verify.py never trusts the model's narration.
#
# Requirements: `claude` (Claude Code) and `python3` on PATH, bin/shhh
# built (run: go build -o bin/shhh ./cmd/shhh).
#
# The OFF run uses `--setting-sources project`, which loads ONLY the
# project settings.json and ignores the user's global ~/.claude config —
# so a globally-installed shhh hook does not contaminate the control run.

set -euo pipefail

demo_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "$demo_dir/.." && pwd)"
shhh="$repo_root/bin/shhh"
src_repo="$demo_dir/leaktest"
manifest="$demo_dir/manifest.json"
prompt_file="$demo_dir/prompts/audit.txt"
model="${SHHH_LEAKTEST_MODEL:-sonnet}"
projects_dir="$HOME/.claude/projects"

# The agent runs against a neutral copy of leaktest/ in a temp directory,
# not demo/leaktest/ itself. This keeps the shhh repo path and its parent
# CLAUDE.md out of the agent's view — otherwise the agent can tell it is
# inside a redaction tool's test fixture and refuse or behave differently.
workbase="$(mktemp -d -t shhh-leaktest.XXXXXX)"
appdir="$workbase/acme-payments"
settings_dir="$appdir/.claude"
settings="$settings_dir/settings.json"

cleanup() { rm -rf "$workbase"; }
trap cleanup EXIT

# ---- preflight ---------------------------------------------------------
command -v claude  >/dev/null || { echo "leaktest: 'claude' (Claude Code) not on PATH" >&2; exit 1; }
command -v python3 >/dev/null || { echo "leaktest: python3 is required" >&2; exit 1; }
[[ -x "$shhh" ]] || { echo "leaktest: $shhh not built — run: go build -o bin/shhh ./cmd/shhh" >&2; exit 1; }

prompt="$(cat "$prompt_file")"

# Stage a neutral copy of the fixture (includes dotfiles via "/.").
mkdir -p "$appdir"
cp -R "$src_repo/." "$appdir/"
rm -rf "$settings_dir"

# run_session <off|on> — runs one headless claude session, echoes its
# session_id on stdout. All progress goes to stderr so stdout is clean.
run_session() {
  local mode="$1" out sid
  rm -rf "$settings_dir"
  if [[ "$mode" == on ]]; then
    mkdir -p "$settings_dir"
    cat > "$settings" <<JSON
{
  "hooks": {
    "PreToolUse": [
      { "matcher": "Read", "hooks": [ { "type": "command", "command": "$shhh hook claude-code", "timeout": 10 } ] },
      { "matcher": "Bash", "hooks": [ { "type": "command", "command": "$shhh hook claude-code", "timeout": 10 } ] }
    ]
  }
}
JSON
  fi
  out="$(cd "$appdir" && claude -p "$prompt" \
      --output-format json \
      --setting-sources project \
      --permission-mode bypassPermissions \
      --model "$model" \
      --max-turns 25)"
  rm -rf "$settings_dir"
  sid="$(printf '%s' "$out" | python3 -c 'import sys, json; print(json.load(sys.stdin)["session_id"])')"
  printf '%s' "$sid"
}

# find_transcript <session_id> — echoes the transcript .jsonl path.
find_transcript() {
  local sid="$1" path
  path="$(find "$projects_dir" -name "$sid.jsonl" -type f 2>/dev/null | head -n1)"
  [[ -n "$path" ]] || { echo "leaktest: transcript for session $sid not found under $projects_dir" >&2; exit 1; }
  printf '%s' "$path"
}

echo "shhh leak test — driving real Claude Code sessions (model: $model)"
echo

echo "  [1/2] shhh OFF (control) — the agent reads raw secrets..."
sid_off="$(run_session off)"
tr_off="$(find_transcript "$sid_off")"
echo "        session: $sid_off"
echo

echo "  [2/2] shhh ON — the hook redacts before the agent sees anything..."
sid_on="$(run_session on)"
tr_on="$(find_transcript "$sid_on")"
echo "        session: $sid_on"

python3 "$demo_dir/verify.py" "$manifest" "$tr_off" "$tr_on"
