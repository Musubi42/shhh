# Feature: Codex hook (`shhh install codex`)

**Status:** research-first draft. The implementation shape
depends on what extension point Codex actually exposes, and I
don't know that yet without reading Codex's own docs. Do not
estimate this feature from the Claude Code experience alone.

## Forcing function

```
$ shhh install codex
$ codex
codex> read .env
  (Codex sees placeholders, not the raw key.)
```

Same scenario as milestone 1, different agent. The bar is:
the same `testdata/fixtures/leaky-project/.env` demo that
works against Claude Code also works against Codex.

## What already exists in the codebase

- `cmd/shhh/cmdhook` — the PreToolUse/Read, PreToolUse/Bash,
  and SessionEnd dispatchers. If Codex's extension mechanism
  looks anything like Claude Code's (stdin JSON → stdout JSON
  hook protocol), ~80% of the dispatcher is reusable.
- `cmd/shhh/cmdhook/sessionstore.go` — per-session placeholder
  map. Agent-agnostic. Reusable as-is.
- `cmd/shhh/cmdinstall` — settings-file merger for Claude Code.
  The abstract shape (load file → insert sentinel entries →
  atomic write → print diff) ports to whatever config format
  Codex uses. The concrete JSON-path logic does not.
- `internal/redactor`, `internal/detector`, `internal/session`,
  `internal/rules` — fully reusable. The library is agent-
  agnostic by design.

## What's new

Unknown until the research step lands. Three possibilities
based on what Codex might plausibly expose:

### If Codex has a hook system analogous to Claude Code

- New file `cmd/shhh/cmdhook/codex.go` — the dispatcher for
  Codex's event schema. Reuses the sessionstore, the redactor
  wiring, and the read/bash interceptor helpers from
  `cmd/shhh/cmdhook/read.go` and `bash.go`.
- New subcommand target `shhh hook codex` routed via the
  existing `cmdhook.Run` switch.
- New file `cmd/shhh/cmdinstall/codex.go` — reads/writes
  Codex's config file format (TOML? JSON? YAML?). Same
  install/uninstall/diff contract as Claude Code.
- New sub-target `shhh install codex` / `shhh uninstall codex`.
- Manual verification against a real Codex session.

**Estimated new code:** ~300 lines.

### If Codex only supports plugins/SDK middleware

- Write a plugin in Codex's plugin format (TypeScript? Rust?
  whatever it uses) that exec's the Go `shhh` binary per
  event. Same dispatcher logic, wrapped.
- Installer drops the plugin into Codex's plugin directory.

**Estimated new code:** ~200 lines of plugin + ~100 lines of
dispatcher. Harder if the plugin language isn't something we
already have tooling for.

### If Codex exposes nothing meaningful

- shhh wraps the `codex` binary: `shhh codex-wrap` or a shim
  on `$PATH` that sits in front of the real codex and filters
  its tool calls. This is architecturally dirtier and worse
  UX (users have to remember to run `shhh codex` instead of
  `codex`).
- Installer aliases `codex` → `shhh codex-wrap` in the user's
  shell config.

**Recommendation:** don't ship this feature in the "no
extension point" case. Better to say "Codex support is blocked
on Codex exposing a hook API" and move on than to ship a
wrapper that's brittle and confusing.

## Open questions

### Q1. Research: what is Codex's extension point?

I need to read Codex's current documentation before this spec
is implementable. Until then, the "What's new" section above is
three branches, not a plan.

Before this spec becomes implementable, answer:
- Does Codex have a hook system with per-tool-call events?
- If yes: what's the protocol (stdin/stdout? HTTP? named pipe)?
- If no: does it have a plugin API? SDK middleware?
- If neither: does it have an MCP client so we can expose
  shhh as an MCP tool?
- Where does Codex store its config? What format?

This is ~30 minutes of reading. It has to happen before code
is written. I should not speculate further in this doc.

### Q2. Does "Codex" mean OpenAI Codex CLI, or GitHub Copilot
in Codex mode, or something else?

The name "Codex" has been applied to several distinct
products over the years. Which one are we targeting? The one
that the user actually uses. Confirm.

### Q3. Can we reuse the same placeholder format?

Claude Code's `additionalContext` field lets us tell the agent
*"shhh redacted X secrets — tell the user."* Does Codex have
an equivalent mechanism? If not, the UX will be slightly
degraded for Codex users: they'll see redacted content but no
natural-language narration from the agent.

Not a blocker, but worth knowing before we promise the UX
matches Claude Code's.

## Out of scope for this feature

- **The NPX installer.** The installer calls `shhh install
  codex` as one option in its multi-select prompt. The NPX
  installer feature is the one that lets Codex be chosen; this
  feature is the one that actually does something when it *is*
  chosen. Two separate silos.
- **Codex-specific detection rules.** If Codex uses a
  proprietary token format, the detector might need a new rule.
  That's an `internal/rules` change, not a hook change. Address
  if we actually see such tokens in the wild.

## What I need from you to start implementing

Answer to Q1 (the research step) and Q2 (which "Codex"). The
research step I can drive myself — give me a green light and I
go read the docs. The "which Codex" is a one-word answer.
