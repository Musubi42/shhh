# 04 — Codex CLI support

## Context

The hook in `cmd/shhh/cmdhook/` is hard-wired to Claude Code's
hook protocol. The dispatch lives in
`cmd/shhh/cmdhook/cmdhook.go::Run`:

```go
switch target {
case "claude-code":
    return runClaudeCode(os.Stdin, os.Stdout)
default:
    return fmt.Errorf("hook: unknown target %q (supported: claude-code)", target)
}
```

`runClaudeCode` parses Claude Code's `{ session_id, hook_event_name,
tool_name, tool_input }` JSON and dispatches PreToolUse on Read /
Bash. The installer mirrors this — `cmd/shhh/cmdinstall/` only
knows how to write into `~/.claude/settings.json`.

Codex (OpenAI's coding agent CLI) is a different runtime with a
different (and possibly absent) hook story. This brief is **half
research, half implementation**. Do NOT skip the research half.

## Step 0 — Research first

Before any code lands, answer the following in a short scratch doc
at `docs/codex-research-2026-XX-XX.md`:

1. **Does Codex CLI support pre-tool-call hooks?** Check the
   current docs for the OpenAI Codex CLI (npm package, GitHub
   repo). Look for terms like "hook", "tool middleware",
   "tool_use plugin", "before_tool_call". Confirm the answer
   on the latest release (the answer might be different from
   even 3 months ago).
2. **What is Codex's tool surface?** Does it have a `Read` /
   `Bash` analog, or is everything one generic `shell` /
   `apply_patch`? The redaction strategy depends on this.
   shhh today intercepts Read (file content) and Bash (command
   output). If Codex uses `apply_patch` and `shell`, the
   interception points are different.
3. **How does Codex store session state?** Claude Code uses
   `~/.claude/projects/*.jsonl`. Codex's equivalent? This
   matters for `shhh audit` retrospective coverage as much as
   for the hook itself.
4. **What's the config file?** Claude Code: `~/.claude/settings.json`
   with a `hooks` block. Codex: probably an OpenAI-namespaced
   path; confirm.

Stop and report back to the user before coding. If the answer to
(1) is **no native hook support**, pivot to the alternative
strategies section below — that's a different brief in spirit.

## Step 1 (if Codex has a hook API) — implementation

### Add a `codex` target to the hook dispatcher

`cmd/shhh/cmdhook/cmdhook.go::Run`:
```go
switch target {
case "claude-code":
    return runClaudeCode(os.Stdin, os.Stdout)
case "codex":
    return runCodex(os.Stdin, os.Stdout)
default:
    ...
}
```

### Implement `runCodex` in a new file

`cmd/shhh/cmdhook/codex.go` — parses Codex's hook payload,
dispatches to the right tool handler. Aim to **reuse the
redaction core**: the `redactor.Redactor` created by
`LoadRedactor` should be the same shape. Only the I/O envelope
changes.

If Codex's tool surface differs (e.g., `apply_patch` instead of
`Read+Write`), implement new handlers but route through the same
redactor. Mirror `cmd/shhh/cmdhook/read.go` and `bash.go` patterns
(file content redaction; command-output redaction).

### Extend the installer

`cmd/shhh/cmdinstall/cmdinstall.go`:
- Add `case "codex":` to `RunInstall` / `RunUninstall`.
- `AgentSettingsPath` (in `cmdinstall.go`) needs a `codex` branch
  returning the right config file path (global + project as
  applicable; mirror the Claude Code shape).
- `DetectInstalledAgents` (also in `cmdinstall.go`) needs to look
  for the Codex config directory and append "codex" when present.

### Extend the interactive picker

`cmd/shhh/cmdinstall/interactive.go::agentOptions`:
- Add an option for Codex when detected.
- Currently the function early-returns on `claude-code`; refactor
  to enumerate `detected` and emit one huh.Option per supported
  agent.

### Extend the audit / config / doctor surfaces

- `cmd/shhh/cmdaudit/` — verify the audit logic works on Codex's
  session storage format. The transcript reader in `internal/audit/`
  is Claude-Code-specific (looks for JSONL events with
  `parentUuid`/`message`/`content` fields). If Codex uses a
  different schema, add a Codex-flavored transcript reader, then
  a dispatcher in `internal/audit/transcripts.go`.
- `cmd/shhh/cmddoctor/` — add Codex hook health checks alongside
  the existing Claude Code ones.
- Config — `Agents []string` already supports multiple. No change.

### Update the install summary attribution

`cmd/shhh/cmdinstall/cmdinstall.go::printInstallSummary` — currently
talks about Claude Code only. Generalize to print the active
agent set ("Hooks installed: claude-code, codex").

### Update README and docs

- README "Wire it into your agent" section — list both.
- `docs/engine-architecture.md` — no change (engines are
  agent-agnostic).

## Step 1 alternative — no native hook API

If Codex has no PreToolUse equivalent, the only paths are:

1. **MCP server.** Reframe shhh as an MCP server that Codex
   talks to. shhh exposes a `read_file_redacted` tool; Codex
   calls it instead of its built-in `read_file`. This requires
   Codex to support MCP (check). It is also at the boundary of
   `CLAUDE.md` rule 5 — read that rule before going down this
   path. Short-lived on-demand subcommand: OK. Background
   daemon: NO. The MCP server should start when Codex starts
   and die when it stops; lifecycle managed by Codex, not by
   shhh.
2. **System prompt injection.** Codex reads a user-supplied
   system prompt; shhh could install one that says "when you
   need to read X, call this shell helper which redacts". This
   is unreliable (LLM compliance varies) and not recommended.
3. **Wrapper shell.** A `shhh-codex` shim that wraps `codex`,
   intercepts the tool stream on the wire. Brittle, version-
   fragile, do not pick this unless (1) is impossible.

If you land in this branch, write up the alternative strategy
in a new design doc (`docs/codex-integration.md`) and stop. The
implementation belongs in its own session after the user picks.

## Acceptance

```sh
shhh install codex
# (interactive picker also offers codex)
codex
# Inside a codex session:
codex> read a file containing STRIPE_LIVE_KEY=sk_live_...
# Codex sees [STRIPE_LIVE_KEY:sk_live_...:hash], not the raw key.
shhh audit
# Output includes Codex session count, leak attribution split by agent.
```

End-to-end test mirror: copy
`cmd/shhh/cmdhook/cmdhook_test.go` (the runClaude helpers) into
a Codex-flavored test file. The hook protocol is different but
the assertion shape is the same.

## Files likely to touch

- `cmd/shhh/cmdhook/cmdhook.go` (dispatcher)
- `cmd/shhh/cmdhook/codex.go` (new)
- `cmd/shhh/cmdhook/codex_test.go` (new)
- `cmd/shhh/cmdinstall/cmdinstall.go` (`AgentSettingsPath`,
  `DetectInstalledAgents`, install/uninstall switches)
- `cmd/shhh/cmdinstall/interactive.go` (`agentOptions`)
- `cmd/shhh/cmdinstall/plan.go` (`Validate` allows "codex")
- `internal/audit/transcripts.go` (if Codex transcript format
  differs from Claude Code)
- `README.md`, `ROADMAP.md`

## Out of scope here

- Cursor support — separate brief in
  [`05-cursor-support.md`](05-cursor-support.md).
- New engines (trufflehog etc.) — orthogonal to agent support.
- Refactoring the cmdhook package into a per-agent subdirectory —
  consider only if a 3rd agent lands and the file count gets
  unwieldy.
