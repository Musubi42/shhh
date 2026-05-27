# Claude Code — shhh integration

How shhh inserts itself into Claude Code, what it intercepts, what
it does not, and what the user sees in each case. This doc stays
abstract — for code, see `cmd/shhh/cmdhook/`.

Reading audience: someone deciding whether shhh covers their use
case in Claude Code, or trying to understand what just happened
when shhh did something in their session.

## What shhh does, in one paragraph

shhh installs hooks into Claude Code's `~/.claude/settings.json`.
When Claude Code is about to read a file, run a shell command, or
(in future) accept a user prompt, the hook fires. shhh runs a
detection engine over the payload, replaces secrets with typed
placeholders like `[STRIPE_LIVE_KEY:sk_live_...]`, and Claude Code
proceeds with the redacted version. The raw secret never reaches
the LLM. Uninstall removes the hooks; Claude Code runs identically
without shhh, minus the protection.

## Commands available

These are CLI commands the user runs in their shell, not slash
commands inside Claude Code.

| Command                          | What it does |
|----------------------------------|--------------|
| `shhh install claude-code`       | Writes hook entries into `~/.claude/settings.json` (global) or `.claude/settings.json` (per-project). |
| `shhh uninstall claude-code`     | Removes the hook entries. Leaves Claude Code working, unprotected. |
| `shhh audit`                     | Shows what shhh has redacted across recent sessions. Read-only. |
| `shhh doctor`                    | Verifies the install: settings file is wired, hook binary is reachable, engine works. |

Planned (not yet shipped):

| Planned command         | What it will do |
|-------------------------|-----------------|
| `/shhh-allow NAME`      | Slash command inside Claude Code. Marks a placeholder name (e.g. `ANTHROPIC_API_KEY`) as allowed for the current session — shhh will let raw values of that kind through instead of redacting. See "Bypass UX" below. |
| `!!` prefix on a prompt | Inline one-shot bypass: prefix a user prompt with `!! ` and shhh skips redaction for that single submit. |

## Interception flows

### 1. Claude reads a file (`.env`, `config.yml`, anything)

- **Hook used:** `PreToolUse` on the `Read` tool.
- **API capability used:** `updatedInput` — the hook rewrites the
  `file_path` Claude Code is about to read.
- **What happens:** shhh runs the engine over the file contents. If
  findings exist, shhh writes a redacted copy to
  `~/.shhh/sessions/<session_id>/<hash>.<basename>` and points
  `Read` at that copy. The original file is untouched on disk.
- **What Claude sees:** the redacted content, with placeholders.
- **What the user sees:** nothing different in the moment. The
  next assistant turn typically mentions the placeholders ("I
  noticed `[STRIPE_LIVE_KEY:sk_live_...]` in your env file"),
  which is how the user learns shhh acted.
- **Engine note:** see `docs/engines/` for which engine runs.

### 2. Claude runs a shell command (`cat .env`, `aws sts ...`, anything via `Bash`)

- **Hook used:** `PreToolUse` on the `Bash` tool.
- **API capability used:** `updatedInput` rewrites the `command`
  string before execution.
- **What happens:** shhh scans the command's argv. If the command
  itself contains a secret (`curl -H "Authorization: Bearer
  sk_live_..."`), the argv is rewritten to use the placeholder.
  Output of the command is **not** post-processed today (no
  `PostToolUse` handler yet) — but the most common leak path is
  `cat .env`, which is covered by the Read flow above.
- **Known gap:** if a Bash command produces output containing a
  secret that did *not* come from a file shhh redacts (e.g. an
  API response printed to stdout), Claude sees the raw value.
  This is on the roadmap; tracked in `docs/dev/known-limitations.md`.

### 3. The user paste a secret directly into their prompt

This is a **planned flow**, not yet implemented. Worth documenting
now because the design constraints are unusual and worth knowing
upfront.

- **Hook used (planned):** `UserPromptSubmit`.
- **API capability available:** `decision: "block"` (refuse the
  prompt) or `additionalContext` (add a side-channel string to
  the LLM). Critically, **Claude Code's hook API does not let
  the hook rewrite the user's prompt text.** There is no
  `updatedPrompt` field. Verified against the official hooks
  documentation.
- **Consequence:** the "silently redact and pass through" model
  used for tool outputs is technically impossible here. Either
  shhh lets the raw secret reach the LLM, or shhh blocks the
  prompt and asks the user to act.
- **Chosen behaviour:** block. The error message names the
  placeholder shhh detected (e.g. "shhh detected
  `[ANTHROPIC_API_KEY:sk-ant-...]` in your prompt at line 3").
  The user's prompt stays in their input buffer; they edit and
  re-submit, or use one of the bypasses below.
- **Why block rather than "warn and pass":** "warn and pass"
  defeats the entire promise. The secret is already at the LLM
  by the time the warning lands. shhh's contract is "the secret
  does not leave your machine"; block is the only option that
  honours it for this surface.

## Bypass UX (planned, for the UserPromptSubmit flow)

Two complementary mechanisms, plus the planned-ahead allow list:

### Inline: `!!` prefix

A user prompt starting with `!! ` (double-bang, space) skips
detection entirely for that single submission. shhh strips the
prefix before the prompt is sent to Claude.

- One keystroke, no slash command, no session disruption.
- Strict start-of-prompt match — `!!` appearing mid-text (e.g. in
  a JavaScript snippet a user is pasting) does not trigger
  bypass.
- The space after `!!` is required, so `!!foo` (a JS expression
  someone copy-pasted) does not match.

### Slash command: `/shhh-allow NAME`

When shhh has blocked a prompt and the user wants to mark a
specific placeholder name as acceptable for the rest of the
session:

1. shhh's block message tells the user the placeholder name it
   detected.
2. The user types `/shhh-allow ANTHROPIC_API_KEY` in Claude Code.
3. The slash command writes that name to a session-scoped allow
   file.
4. The user presses ↑ to recall their previous prompt and
   re-submits. shhh's hook reads the allow file, sees the name
   is allowed, lets the prompt through with the secret intact.

The allow is **session-scoped**, with two cleanup paths:

- **Cleared on `Stop` / `SessionEnd` hooks** — when Claude Code
  ends the session, the allow file goes with it.
- **GC'd after 24 hours** — even if the session never emits a
  clean stop signal, allow files older than 24h are deleted on
  the next hook invocation.

The slash command's confirmation message must say this explicitly
("ANTHROPIC_API_KEY allowed for this session (max 24h)") so the
user is not surprised when it stops applying.

### Planned ahead: `shhh allow <NAME>` (CLI)

For sessions where the user knows in advance they will paste
fakes (demo recordings, test fixtures), a CLI command writes a
longer-lived allow entry. Same names work — they map to
placeholder types from the engine. Details TBD; the slash command
covers the in-session need first.

## How `allow` and `redact` cohabitate

These are two knobs on the same flow, not competing features:

- **`redact`** is what shhh *does* with a finding (rewrite to
  placeholder, or pass through). It runs on every hook
  invocation.
- **`allow`** decides, per placeholder name, whether redaction
  is skipped for that name. It is read from the session allow
  file before `redact` runs.

**`allow` is asymmetric**: it gates only the user-prompt flow
(UserPromptSubmit), **not** the tool-output flows (PreToolUse on
Read/Bash). If a user runs `/shhh-allow ANTHROPIC_API_KEY`, their
next paste containing an Anthropic key passes through to the LLM
raw — but files Claude reads continue to be redacted normally.

The rationale: a user who pastes a secret knows what they pasted;
a user who asks Claude to read a file does not necessarily know
what is inside. Keeping tool-output redaction always-on is the
safe default. If a future use case demands "expose this name in
files too," it gets its own opt-in (and its own command), not a
silent broadening of `/shhh-allow`.

## Engine selection

`shhh install claude-code` lets the user pick the engine
(`gitleaks` or `shhh-native`); shhh-native is the default. The
choice does not affect which hooks fire — it only affects what
counts as a finding. See `docs/engines/gitleaks.md` and
`docs/engines/shhh-native.md`.

## Known limits (current state, 2026-05)

- **`PostToolUse` is not handled.** Output from Bash commands is
  not redacted. A `curl` that prints a secret in its response
  body will reach the LLM unredacted.
- **`UserPromptSubmit` is not handled.** User-pasted secrets
  currently reach the LLM. The design above is the planned
  remediation.
- **Image and binary pastes are out of scope.** No OCR.
- **Edit/Write on a redacted file fails** because Claude Code's
  internal "has this been read?" ledger keys on the rewritten
  path. Workaround documented in `docs/dev/known-limitations.md`
  §1.

The current state honours the forcing-function scenario for
agent-initiated file reads (`claude> read .env`). It does not yet
honour it for user-initiated paste-into-prompt — that gap is the
next milestone.
