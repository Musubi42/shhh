# Codex CLI research — 2026-05-26

Scratch doc per brief 04 step 0. Captures the state of the OpenAI
Codex CLI as it relates to shhh's hook integration. Not normative
code — input for the design decision.

## Q1 — Does Codex support pre-tool-call hooks?

**Verdict: partial yes.** Codex CLI v0.117+ ships a hook system
that is a near-1:1 mirror of Claude Code's. Events:
`PreToolUse`, `PostToolUse`, `SessionStart`, `UserPromptSubmit`,
`PreCompact`, `Stop`, and subagent variants. Hooks are configured
in `~/.codex/hooks.json` (or `~/.codex/config.toml`, or per-repo
equivalents).

The stdin-JSON payload the hook receives is **the same shape**
shhh already parses for Claude Code: `session_id`, `tool_name`,
`tool_use_id`, `tool_input`, `cwd`, `transcript_path`. The hook
can return:

- `permissionDecision: "allow"` + `updatedInput` (rewrite the call)
- `permissionDecision: "deny"` (block)
- `additionalContext` (inject text into the model's view)

**Critical caveat:** today, `PreToolUse` fires reliably only for
the `Bash` tool. `apply_patch`, `read_file`, `grep`, and most
built-in tools either don't emit `PreToolUse` or are
mis-tagged as `tool_name: "Bash"` (hardcoded in
`hook_runtime.rs`). Tracking issue:
[`openai/codex#18491`](https://github.com/openai/codex/issues/18491).
MCP tool calls *do* fire `PreToolUse` correctly.

Source:
[Hooks – Codex](https://developers.openai.com/codex/hooks),
[`openai/codex#16732`](https://github.com/openai/codex/issues/16732),
[`openai/codex#18491`](https://github.com/openai/codex/issues/18491).

## Q2 — Tool surface

**Verdict: collapsed.** Codex does not separate `Read` and `Bash`
like Claude Code does. File reads happen through `cat`/`rg`/`sed`
inside the unified shell tool, which fires as `Bash`. File writes
go through `apply_patch` (matcher aliases: `apply_patch`, `Edit`,
`Write`).

Tools a Codex hook can observe today:
- `Bash` — every shell command (this is the only one we can
  reliably intercept).
- `apply_patch` — edits (hook does not fire reliably yet).
- `mcp__<server>__<tool>` — MCP tools (fully working).
- Internal `read_file` / `grep` — present but don't emit
  `PreToolUse` yet.

**Implication:** the Claude Code `Read` interception point has
no Codex twin. shhh's coverage on Codex collapses into "what
does the shell command output?" — concretely, `cat .env`,
`rg STRIPE_LIVE_KEY`, `head package-lock.json`, etc. The typical
"agent reads my .env" path still gets covered (because the model
does `cat .env`). The `apply_patch`-only edge case (edit a file
that contains a secret in-place) is not covered until upstream
fixes #18491.

## Q3 — Session state on disk

**Verdict: yes, well-documented.** Path:

```
~/.codex/sessions/YYYY/MM/DD/rollout-<ISO-ts>-<id>.jsonl
```

Recent versions write `.jsonl.zst` (Zstandard-compressed).
Format is JSONL: prompts, model responses, tool calls, tool
results, timestamps. Always-on; no opt-in.

**Implication for `shhh audit`:** doable, requires a zstd
decoder in `internal/audit/transcripts.go` and a Codex-flavored
event-schema parser. The retrospective-scan loop is unchanged.

Source:
[PixelPaw-Labs/codex-trace](https://github.com/PixelPaw-Labs/codex-trace),
[Codex CLI Resume guide – Verdent](https://www.verdent.ai/guides/codex-cli-resume-continue-save-chat).

## Q4 — Config file

**Verdict:** `~/.codex/config.toml` (TOML, not JSON) is the
primary config. Per-project override: `<repo>/.codex/config.toml`.
Hooks specifically can live in a dedicated `~/.codex/hooks.json`
or `<repo>/.codex/hooks.json` (cleaner for our install/uninstall
shape because it isolates shhh's footprint).

Source:
[Configuration Reference – Codex](https://developers.openai.com/codex/config-reference),
[Config basics – Codex](https://developers.openai.com/codex/config-basic).

## Bonus

- **MCP support: yes**, first-class. Configured as
  `[mcp_servers.<name>]` in `config.toml`. Viable fallback for
  tools where `PreToolUse` doesn't fire. Not needed for v1.
  [MCP – Codex](https://developers.openai.com/codex/mcp).
- **Prior art: none** for secret-redaction on Codex. The only
  related project surfaced is
  [Agentic Control Plane](https://agenticcontrolplane.com/integrations/codex)
  (governance/audit, not redaction). shhh would be the first in
  this niche on Codex.
- **Latest version:** v0.116.0 (2026-03-19). Hooks landed around
  v0.117. Active development; expect rapid surface changes.
  [Changelog](https://developers.openai.com/codex/changelog),
  [npm](https://www.npmjs.com/package/@openai/codex).

## Recommendation

**Proceed down the implementation path** with a Bash-only v1 and
an honest README disclosure of the limit. Rationale:

1. The hook *mechanism* is a structural twin — same stdin JSON,
   same `updatedInput` rewrite semantics, same payload field
   names. shhh's Go binary can dispatch to a `runCodex` handler
   that reuses `LoadRedactor` and the entire detection core
   without modification.
2. The single real limitation (only `Bash` fires reliably) maps
   directly onto how Codex actually reads files in practice
   (`cat .env`, `rg`, etc.). The typical attack-surface case
   ("Codex reads my .env") *is* covered. The `apply_patch`-only
   case is a hole, but tracked upstream — when OpenAI ships
   `#18491`'s fix, shhh's coverage auto-completes.
3. Coverage hole is honest-to-document, mirroring how option D
   handled the Read→Edit ledger limit on Claude Code. Same
   pattern: ship the value that exists today, document the gap,
   link the upstream tracking issue, auto-improve when upstream
   ships.

## Open questions for implementation (parking lot)

These don't block the design decision but need to be answered
during coding:

- Does Codex's `PreToolUse.Bash` payload contain the command
  *output* or just the *to-be-run command*? shhh's existing Bash
  handler intercepts output via `PostToolUse` on Claude Code.
  Confirm whether Codex's hook fires pre-command (we'd intercept
  the command string) or post-command (we'd intercept the
  output).
- Confirm the exact `tool_input` shape for Bash on Codex —
  Claude Code's is `{command: string, description?: string}`.
  Test against a real Codex hook.
- Decide install footprint: write to `~/.codex/hooks.json` (clean,
  isolated, easy uninstall) or merge into `~/.codex/config.toml`
  (matches user's primary config but harder to round-trip
  cleanly). Recommend the former.
- Audit reader needs a zstd decoder. Either `klauspost/compress/zstd`
  (already in our indirect deps via gitleaks) or
  `DataDog/zstd`. Check go.mod before pulling a new dep.
