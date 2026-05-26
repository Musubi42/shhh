# Cursor research — 2026-05-27

Scratch doc per brief 05 step 0. Captures the state of Cursor IDE
relative to shhh's integration model. Important: **brief 05's
premise is outdated** — it was written assuming Cursor had no
hook system and would need MCP. Cursor 1.7 (Oct 2025) shipped
native hooks; Cursor 2.4 (2026) expanded them. The integration
path is no longer MCP-shaped; it's the same hook-shape as Claude
Code and Codex with a few protocol differences.

## Q1 — User-configurable MCP servers

**Verdict: yes.** Cursor supports stdio + remote MCP servers at
`~/.cursor/mcp.json` (global) or `<repo>/.cursor/mcp.json`
(project). Ceiling: ~40 active tools across all MCP servers
before the agent silently drops some.

Source: [Cursor MCP docs](https://cursor.com/docs/context/mcp).

**Implications for shhh:** MCP is available but no longer the
preferred integration vector — see Q4.

## Q2 — Can `.cursor/rules` force the agent to call an MCP `read_file` instead of the built-in?

**Verdict: no, not reliably.** Cursor's MCP docs offer enable/
disable toggles per tool but no override / replacement for
built-ins. Community consensus (Cursor forum #99819) is that
rules nudge selection probabilistically but cannot guarantee the
agent skips the built-in `read_file`.

Source: [Cursor forum: enabling MCP tools in rules](https://forum.cursor.com/t/support-enabling-mcp-tools-in-cursor-rules/99819).

**Implications for shhh:** the original brief-05 linchpin (MCP +
rule to replace built-in read) fails. Even if we shipped an MCP
`read_file_redacted`, the agent could silently fall back to the
built-in and bypass redaction. This rules out strategy A as
originally specified.

## Q3 — Session/transcript storage on disk

**Verdict: yes, parseable.** macOS layout:

- `~/.cursor/projects/*/agent-transcripts/*.jsonl` — flat JSONL
  per project (preferred reader target, easy parse).
- `~/.cursor/chats/*/*/store.db` — per-session SQLite. `blobs`
  table holds messages.
- `~/Library/Application Support/Cursor/User/globalStorage/state.vscdb`
  — global SQLite, keys `composerData:*`, `bubbleId:*`, `agentKv`
  (~88k entries on long-running installs).
- Tool calls persist with `toolFormerData` fields (parameters,
  approval status, results) — usable for retrospective audit.

Linux: `~/.config/Cursor/User/globalStorage/state.vscdb`.
Windows: `%APPDATA%\Cursor\User\globalStorage\state.vscdb`.

Source: [vibe-replay: Cursor local storage deep-dive](https://vibe-replay.com/blog/cursor-local-storage/).

**Implications for shhh:** `shhh audit` on Cursor is feasible.
First-pass implementation reads the JSONL transcripts (no extra
deps); SQLite fallback added later if needed.

## Q4 — Pre-tool-call hook system

**Verdict: yes — and the docs explicitly list "redact secrets
before they reach the LLM" as a canonical use case.**

Cursor 1.7 introduced hooks (Oct 2025). 2.4 (2026) extended them:
- `beforeReadFile` — fires before file content reaches the model.
  **Allow/deny only; no content modification supported** (the
  docs are explicit: "does not include a field to return modified
  file content").
- `preToolUse` / `postToolUse` — generic tool interception.
  `preToolUse` supports `updated_input` to rewrite the call.
- `beforeShellExecution` — pre-shell-exec gating. Allow/deny/ask,
  no `updated_command` field per the docs.
- `beforeMCPExecution` / `afterMCPExecution` — MCP tool gating.
- `beforeSubmitPrompt`, `stop` — lifecycle events.

### Hook config shape (verbatim from [docs](https://cursor.com/docs/hooks)):

```json
{
  "version": 1,
  "hooks": {
    "beforeReadFile": [
      { "command": "./hooks/redact-secrets.sh", "timeout": 30 }
    ],
    "preToolUse": [
      { "command": "./hooks/validate-tool.sh", "matcher": "Shell|Read|Write" }
    ]
  }
}
```

Hook config lives at `~/.cursor/hooks.json` (global) or
`<repo>/.cursor/hooks.json` (project).

### Stdin payload conventions

snake_case throughout (`session_id` → `conversation_id` +
`generation_id` on Cursor, `tool_name`, `tool_input`,
`tool_use_id`, `cwd`, `hook_event_name`, `transcript_path`,
`model`, `workspace_roots`, `cursor_version`, `user_email`).

`preToolUse` payload:
```json
{
  "tool_name": "Shell",
  "tool_input": { "command": "npm install", "working_directory": "/project" },
  "tool_use_id": "abc123",
  "cwd": "/project",
  "model": "claude-sonnet-4-20250514",
  "agent_message": "Installing dependencies..."
}
```

`beforeReadFile` payload:
```json
{
  "file_path": "<absolute path>",
  "content": "<file contents>",
  "attachments": [{ "type": "file"|"rule", "file_path": "<absolute path>" }]
}
```

### Response envelope — DIFFERENT FROM CLAUDE CODE

This is the protocol-divergence detail that matters for the
implementation:

| Field | Claude Code | Cursor |
|---|---|---|
| envelope key | `hookSpecificOutput` | (none, flat) |
| permission flag | `permissionDecision: "allow"\|"deny"` | `permission: "allow"\|"deny"\|"ask"` |
| input rewrite | `updatedInput` | `updated_input` |
| agent-facing note | `additionalContext` | `agent_message` |
| user-facing note | — | `user_message` |
| event name field | `hookEventName` | (absent) |

Codex matched Claude Code's response shape exactly. Cursor does
not. shhh's response code cannot be shared between agents; we
need a Cursor-specific envelope. The same `hookInput` stdin
struct stays usable (snake_case fields overlap closely enough).

### preToolUse response (verbatim):
```json
{
  "permission": "allow" | "deny",
  "user_message": "<shown when denied>",
  "agent_message": "<sent to agent when denied>",
  "updated_input": { "command": "npm ci" }
}
```

### beforeReadFile response (verbatim):
```json
{
  "permission": "allow" | "deny",
  "user_message": "<shown when denied>"
}
```

Note: no `agent_message`, no `updated_input`, no
`updated_content`. **`beforeReadFile` cannot redact content** —
it can only block reads, optionally with a user-visible note.

### Exit-code semantics (verbatim):

- `0` — hook succeeded, use the JSON output.
- `2` — block the action (equivalent to `permission: "deny"`).
- Other — hook failed, action proceeds (**fail-open by default**).

Per brief 05's risk #2 ("fail-open means redaction silently
disabled — invisible and unsafe; pick fail-closed"), shhh must
either return valid JSON every time *or* deliberately use exit
code 2. The current `writeEmpty` fallback would land here as
"hook failed, fail-open" — same trade-off as on Claude Code,
which we've accepted there (non-blocking-best-effort by design).

Source: [Cursor hooks docs](https://cursor.com/docs/hooks),
[InfoQ on Cursor 1.7 hooks](https://www.infoq.com/news/2025/10/cursor-hooks/),
[GitButler deep-dive](https://blog.gitbutler.com/cursor-hooks-deep-dive),
[hamzafer/cursor-hooks examples](https://github.com/hamzafer/cursor-hooks).

**Implications for shhh:**

- Strategy A's mechanism is now native hooks, **not MCP**. Rewrite
  brief 05 around `hooks.json` before any implementation work.
- The same redaction core (`internal/redactor`) plugs in; the
  envelope code is new but small.
- `beforeReadFile`'s allow/deny-only contract means content
  redaction for Cursor must happen via `preToolUse:Read` —
  rewriting `tool_input.file_path` to a redacted cache path,
  exactly the Claude Code approach. This re-introduces the
  Read→Edit ledger risk (brief 01 / option D). Whether Cursor
  has the same ledger behaviour is unknown without a real
  smoke test.
- Bash interception is `preToolUse` with matcher `Shell` (not
  `Bash`); the wrap pattern stays identical.

## Q5 — Market position in 2026

**Verdict: still growing, second only to Copilot in workplace
adoption.** JetBrains Jan 2026 survey: Copilot 29%, Cursor 18%
(tied with Claude Code), Windsurf 8%. Cursor self-reports 7M MAU,
1M+ paying.

Source: [Uvik 2026 comparison](https://uvik.net/blog/claude-code-vs-cursor-vs-copilot-vs-codex-2026/).

**Implications for shhh:** the audience justifies a first-class
integration, not a deferred audit-only token.

## Bonus

- **Prior art:** Pipelock (security scanning via hooks), Endor
  Labs `cursor-hook-examples`, a Michael Feldstein
  `redact-secrets.sh` denying reads on files containing GitHub
  tokens. No project currently offers typed-placeholder redaction
  with reversible mapping. shhh's niche on Cursor is open.
  ([Pipelock](https://pipelab.org/learn/cursor-integration/),
  [endorlabs/cursor-hook-examples](https://github.com/endorlabs/cursor-hook-examples))
- **Latest stable:** Cursor **2.4** (2026), per the
  [Cursor changelog](https://cursor.com/changelog).

## Recommendation

**Pivot strategy A from MCP to native hooks.** Brief 05's MCP
plan was correct given what was known at writing time but is
superseded by Cursor 1.7's hooks shipment. Concretely:

1. `shhh hook cursor` — new dispatcher target reusing the
   existing stdin parsing. Response envelope is Cursor-specific
   (snake_case, no `hookSpecificOutput` wrapper).
2. Install target: `~/.cursor/hooks.json` (global) or
   `<repo>/.cursor/hooks.json` (project). Register on
   `preToolUse` with matcher `"Shell|Read"`.
3. Shell interception: wrap command through `shhh redact`, same
   pattern as Claude Code's Bash handler. Tool name is `Shell`
   on Cursor.
4. Read interception: rewrite `tool_input.file_path` to redacted
   cache, with agent_message narration (Cursor's equivalent of
   `additionalContext`) telling the agent to use `Shell` if it
   hits the same Read→Edit ledger issue.
5. **Defer to PR 2:** Cursor transcript reader for `shhh audit`
   (parse the JSONL files in `~/.cursor/projects/*/agent-transcripts/`).
6. **`beforeReadFile`:** not used in v1 — it's allow/deny-only.
   Could optionally add as a "block secret files entirely"
   power-user mode later, but not for the launch.

This is a near-Codex-scale shipment: dispatcher, runCursor,
codex_test.go-style coverage, installer extension, README +
known-limitations updates. Difference: response envelope code is
new (Codex reused Claude Code's verbatim).

## Open questions for implementation

- **Tool name spelling.** Confirm "Shell" vs "Bash" vs "shell" by
  running a real Cursor session against a probe hook before
  hardcoding the matcher.
- **Does Cursor have the Read→Edit ledger bug?** If `preToolUse`
  on Read rewrites `file_path` to a cache path, does Cursor's
  subsequent `preToolUse:Write` precondition-check on the
  original path fail? Smoke-test in a real session.
- **SessionEnd equivalent.** Cursor docs mention a `stop` event;
  confirm semantics match (wipe session cache when the
  conversation ends).
- **Project-scope vs global.** `<repo>/.cursor/hooks.json` exists
  per docs. Mirror the per-project install ergonomics already
  shipped for Claude Code + Codex.
