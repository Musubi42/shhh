#!/usr/bin/env bash
#
# make demo: end-to-end smoke test for the Claude Code hook.
#
# This does NOT drive a real `claude` binary — that step is manual (see
# docs/dev/implementation-roadmap.md milestone 1, work item 5). What it does
# do is simulate Claude Code calling the hook on a Read of a .env file
# containing a fake Stripe live key, then assert:
#   1. The hook rewrote tool_input.file_path to a shhh cache location.
#   2. The redacted file does not contain the raw key.
#   3. The redacted file does contain a [STRIPE_LIVE_KEY:...] placeholder.
#
# If any of those fail, exit non-zero so `make demo` breaks the build.

set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
shhh="$repo_root/bin/shhh"

if [[ ! -x "$shhh" ]]; then
  echo "demo: $shhh not built (run: make build)" >&2
  exit 1
fi

work="$(mktemp -d -t shhh-demo.XXXXXX)"
trap 'rm -rf "$work"' EXIT

cache="$work/cache"
mkdir -p "$cache"

envfile="$work/.env"
key="sk_live_4eC39HqLyjWDarjtT1zdp7dc"
cat >"$envfile" <<EOF
# fake stripe key for the shhh demo
STRIPE_LIVE_KEY=$key
DB_URL=postgresql://admin:hunter2@prod-db.example.com:5432/myapp
EOF

payload=$(cat <<EOF
{
  "session_id": "shhh-demo-0001",
  "hook_event_name": "PreToolUse",
  "tool_name": "Read",
  "tool_input": {
    "file_path": "$envfile"
  }
}
EOF
)

echo "demo: firing PreToolUse/Read at $envfile"
response=$(SHHH_CACHE_DIR="$cache" printf '%s' "$payload" | "$shhh" hook claude-code)

if [[ -z "$response" || "$response" == "{}" ]]; then
  echo "demo: FAIL — hook returned empty response, expected updatedInput" >&2
  echo "response: $response" >&2
  exit 1
fi

# Extract updatedInput.file_path without depending on jq.
redacted_path=$(printf '%s' "$response" | python3 -c '
import sys, json
d = json.loads(sys.stdin.read())
print(d["hookSpecificOutput"]["updatedInput"]["file_path"])
')

if [[ ! -f "$redacted_path" ]]; then
  echo "demo: FAIL — redacted file $redacted_path does not exist" >&2
  exit 1
fi

redacted=$(cat "$redacted_path")

if grep -qF "$key" <<<"$redacted"; then
  echo "demo: FAIL — raw Stripe key leaked into redacted file" >&2
  echo "--- redacted ---"; echo "$redacted"
  exit 1
fi

if ! grep -qF "[STRIPE_LIVE_KEY:" <<<"$redacted"; then
  echo "demo: FAIL — STRIPE_LIVE_KEY placeholder missing from redacted file" >&2
  echo "--- redacted ---"; echo "$redacted"
  exit 1
fi

echo
echo "demo: OK"
echo "  raw .env (fake secret):"
sed 's/^/    /' "$envfile"
echo "  redacted copy (what Claude would see):"
sed 's/^/    /' "$redacted_path"
