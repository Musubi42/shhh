# 05 — Cursor support

## Context

Cursor is an IDE (VS Code fork) with built-in agent features. Its
integration model is fundamentally different from Claude Code or
the Codex CLI:

- **Process model**: Cursor is a GUI app, not a CLI. There is no
  obvious place to "shell out to shhh on every tool call" the way
  the Claude Code hook does (Claude Code spawns `shhh hook
  claude-code` as a subprocess per tool call).
- **Tool surface**: Composer / Agent mode uses internal tools the
  user does not directly control. Cursor's "Rules" feature
  (`.cursor/rules/`) lets the user inject instructions at the
  system-prompt level but cannot intercept tool input/output the
  way Claude Code hooks can.
- **What might exist**: Cursor has been adding MCP support;
  Cursor Composer can be configured to use external MCP servers
  as tool providers. That is the most likely integration vector.

This brief is **mostly research** until the user picks a strategy.
Do not write integration code before the research phase lands.

## Step 0 — Research first

Answer in a short doc at `docs/cursor-research-2026-XX-XX.md`:

1. **Does Cursor support MCP servers from end-user config?** If
   yes, what config file? Is the MCP toolset replaceable, or only
   additive to built-in tools?
2. **Can a Cursor rule force Composer to call a specific MCP
   `read_file` tool instead of the built-in one?** This is the
   linchpin question. If Composer keeps falling back to its
   internal read, redaction is bypassed.
3. **What is Cursor's session/transcript storage?** Audit
   retrospective coverage matters for the marketing pitch
   ("Cursor has probably seen your secrets too"). Find the
   on-disk format if any.
4. **Is there a "before each LLM call" hook similar to Claude
   Code's PreToolUse?** As of this brief's writing the answer is
   believed to be **no**, but verify against the latest Cursor
   release notes.
5. **What's the Cursor user count and intent?** If it's the
   dominant Claude-replacement for many devs, shipping a partial
   integration (e.g., audit-only, no hook) is still a worthwhile
   first step.

Stop and report back. The strategy chosen below depends on the
research outcome.

## Likely strategies (post-research)

### A — MCP server (preferred if Cursor supports it cleanly)

shhh exposes an MCP server (`shhh mcp-server`) that provides
`read_file_redacted`, `run_command_redacted`, etc. Users add the
server to their Cursor config; a `.cursor/rules/shhh.md` rule
file instructs Composer to use those tools instead of built-ins
when reading code/files.

- Server lives in a new package `cmd/shhh/cmdmcp/`.
- Long-lived in the sense that it stays open for the duration of
  the Cursor session, but it's launched by Cursor (not by shhh),
  which keeps it inside the boundary of CLAUDE.md rule 5
  ("subcommand the user just typed" → fine; the user, via
  Cursor, typed it).
- Re-uses the same `redactor.Redactor` core as the hook. Only
  the I/O envelope (MCP protocol vs Claude Code hook JSON)
  changes.
- Installer (`shhh install cursor`) writes the MCP block into
  Cursor's config + a starter `.cursor/rules/shhh.md`.

### B — Audit-only first integration

`shhh audit` already scans Claude Code's history. Add a Cursor
transcript reader, even if no real-time hook is possible. Ship
it as "shhh works with Cursor for retrospective audit; live
redaction coming when Cursor exposes the hook API."

- Low risk, low payoff, ships fast.
- Sets up the "Cursor partial support" claim honestly.
- Good companion to the launch post: "your existing Cursor
  history has secrets too — here's what we found in mine."

### C — Rules-only nudging (NOT RECOMMENDED)

Install a `.cursor/rules/shhh.md` that instructs Composer
"redact secrets in your output." LLM compliance is unreliable;
this is closer to placebo than redaction. Mention only to rule
out.

## Step 1 — Implementation outline (strategy A specifics)

Most of the file paths below are speculative until the research
in Step 0 confirms the shape. Edit this brief during that
session to lock the details before coding.

- `cmd/shhh/cmdmcp/cmdmcp.go` — MCP server entry point.
- Reuse `internal/redactor` + `internal/detector` directly.
- Map MCP tool inputs (`read_file`'s `path` argument) to the
  same redaction pipeline used by `cmd/shhh/cmdhook/read.go`.
- `cmd/shhh/cmdinstall/cmdinstall.go` — add `cursor` agent
  target. Write the MCP server config (path TBD) and seed
  `.cursor/rules/shhh.md` with a one-paragraph instruction.
- `cmd/shhh/cmdinstall/interactive.go` — add cursor to the agent
  options once detected.
- `internal/audit/` — Cursor transcript reader (strategy B
  applies whether or not A ships).
- `cmd/shhh/main.go` — dispatcher entry for `shhh mcp-server`.

For the MCP server itself, evaluate Go MCP libraries:
- `github.com/mark3labs/mcp-go` — actively maintained
- Anthropic's official Go SDK (if released) — preferred if
  available

Pick a library, pin the version, treat it like gitleaks: thin
wrapper, MIT-compatible.

## Acceptance (strategy A)

```sh
shhh install cursor
# Output: MCP server registered, .cursor/rules/shhh.md seeded.
# Restart Cursor.
# In Composer: "read .env"
# Composer calls shhh's MCP read_file_redacted.
# Composer sees: STRIPE_LIVE_KEY=[STRIPE_LIVE_KEY:sk_live_...:hash]
```

## Acceptance (strategy B alone)

```sh
shhh audit  # finds Cursor sessions, attributes leaks per agent.
# README documents: "Cursor: audit only; live redaction blocked on
# Cursor MCP hook stability."
```

## Risks to call out

- **Cursor's tool routing may not be deterministic.** Even with
  an MCP `read_file_redacted` tool registered, Composer might
  fall back to the built-in read for cached files or system
  paths. Cite the relevant Cursor docs in the research note.
- **MCP server lifecycle.** If the server crashes, the redaction
  silently fails open or fails closed? Decide early. Fail-closed
  means Composer's reads start erroring — visible but annoying.
  Fail-open means redaction silently disabled — invisible and
  unsafe. Pick fail-closed.
- **`internal/audit` schema.** Cursor's transcript format
  (assuming one exists) needs reverse-engineering from the
  on-disk files. Time-box this — if the format is opaque or
  proprietary, drop strategy B and ship A only.

## Out of scope here

- Codex CLI — see [`04-codex-support.md`](04-codex-support.md).
- Adding more engines (trufflehog etc.) — orthogonal.
- Refactoring the agent-targeting structure (cmdhook vs cmdmcp
  vs future cmdwhatever). Wait until at least Cursor and Codex
  ship to decide if a per-agent subpackage is worth it.

## Versioning + release pacing

Cursor support is unlikely to be a single-session ship. Plan for
**two PRs**:

1. **PR 1 (audit-only):** Cursor transcript reader + audit
   coverage + README update saying "audit works on Cursor".
2. **PR 2 (MCP server):** the real-time redaction integration
   when Cursor MCP support is verified.

This lets the audit half ship in the launch window even if MCP
turns out to need more work.
