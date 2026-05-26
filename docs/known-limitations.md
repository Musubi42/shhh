# Known limitations

Honest list of things shhh does NOT do today. Read this before
filing a bug — what looks like a defect may be a documented limit
of the Claude Code hook API that shhh has chosen to live with
rather than paper over silently.

## 1. `Edit` / `Write` fail on a file that shhh just redacted

### What you see

```text
claude> read .env
  (shhh redacts the file, Claude sees placeholders.)

claude> edit .env line 5 "STRIPE_LIVE_KEY=…"
Error: File has not been read yet. Read it first before writing to it.
```

The error comes from Claude Code, not shhh.

### Why this happens

When shhh intercepts the `PreToolUse/Read` call, it rewrites
`updatedInput.file_path` from the original path (e.g.
`~/project/.env`) to a per-session redacted copy
(`~/.shhh/sessions/<sid>/<hash>.env`). Claude Code's internal
"has this file been read?" ledger records the rewritten path —
because that is the path the `Read` tool actually executed
against. The next `Edit` / `Write` on the original path fails
its precondition check.

### Why shhh does not fix it

Three hook-API strategies were evaluated and ruled out:

| Strategy | Why it cannot work today |
|---|---|
| Replace tool result content in `PostToolUse/Read` | `updatedMCPToolOutput` only applies to MCP tools, not built-ins like `Read`. |
| Return a synthetic result from `PreToolUse/Read` | `PreToolUse` has `permissionDecision: "deny"` but no field to substitute a result for a blocked call. |
| Inject "file was read" state into the ledger | The Read-ledger is internal to Claude Code; no hook field mutates it. |

A fourth strategy — exposing a parallel `shhh-read` MCP tool —
does not actually solve the problem because the built-in `Edit`
still requires the built-in `Read` to have been called on the
target path. See
[`docs/design/read-edit-tracking.md`](design/read-edit-tracking.md)
for the full investigation and references.

### What shhh does instead

The hook's `additionalContext` payload tells Claude, in plain
language, that `Edit` and `Write` will fail on the just-read file
for the rest of the session, and points it at the `Bash` tool
(`sed -i`, `tee`, `printf >>`, `python -c`, …) as the working
alternative. `Bash` output is also redacted by shhh
(`cmd/shhh/cmdhook/bash.go`), so writes through that path stay
safe.

In practice this means: on a file that contained a real secret,
Claude will reach for `Bash` directly instead of failing on
`Edit` first. Clean files (no findings, no rewrite) are
unaffected and `Edit` works on them normally.

### When this is expected to lift

If Anthropic ships either of these on the hook API:

1. `PostToolUse.updatedOutput` (or symmetric to
   `updatedMCPToolOutput`) for built-in tools, OR
2. `PreToolUse.markFileAsRead(path)` as a documented
   side-effect,

shhh will drop the Bash narration and `Edit` / `Write` will work
on the first try. Feedback to Anthropic is logged in
`docs/design/read-edit-tracking.md` under "Feedback to
Anthropic".

### Affected versions

Last verified on Claude Code `2.1.150` (2026-05-26). Every prior
Claude Code release exposing the current hook-API surface is
affected too — there is no version below which this works. Updates
that change the hook API are tracked in the GitHub tracking issue
(see `docs/ready-to-publish/01-tracking-issue-draft.md` for the
body until the issue is opened).

## 2. Codex: only `Bash` is intercepted today

### What you see

When you run `shhh install codex`, the post-install footer prints:

```
Codex coverage note: shhh intercepts Bash today (cat .env, rg, etc.). Codex's
apply_patch and read_file tools do not yet fire PreToolUse upstream — track
https://github.com/openai/codex/issues/18491.
```

That is the limit: shhh sees Codex shell commands (`cat .env`,
`rg`, `head`, `sed -i`), but it does **not** see `apply_patch`
edits or the internal `read_file` / `grep` tools.

### Why this happens

Codex CLI v0.117 ships a `PreToolUse` hook system structurally
identical to Claude Code's (same JSON payload, same response
shape — see
[`docs/codex-research-2026-05-26.md`](codex-research-2026-05-26.md)).
But as of 2026-05, `PreToolUse` fires reliably only when
`tool_name == "Bash"`. `apply_patch`, `read_file`, and `grep` are
tracked upstream in
[openai/codex#18491](https://github.com/openai/codex/issues/18491)
but not yet shipped.

### Practical impact

- ✓ "Codex, please read my `.env`" — Codex will run `cat .env`,
  the Bash hook fires, shhh redacts. **Covered.**
- ✓ "Codex, please grep for `STRIPE_LIVE_KEY` across the repo" —
  same path (Codex uses `rg`). **Covered.**
- ✗ "Codex, please edit `config.yml` line 12" — Codex calls
  `apply_patch` directly, which does not fire `PreToolUse`. The
  pre-edit content (which Codex must read into memory to compute
  the patch) reaches the model unredacted. **Not covered.**

### When this lifts

Automatically, when Anthropic-style multi-tool hook coverage
lands upstream — `cmd/shhh/cmdhook/codex.go`'s dispatcher will
just need an `apply_patch` / `read_file` handler added to the
switch. Track the upstream issue.

### Workaround until upstream ships

Prefer prompting Codex to use shell tools for file inspection
("read X with `cat`", "patch Y with `sed -i`"). Codex's
shell-via-Bash mode catches everything shhh needs to see.

---

## Reporting limitations not listed here

Open an issue against
[Musubi42/shhh](https://github.com/Musubi42/shhh/issues) with:

- the exact `claude --version` you ran,
- the redacted output (the hook's trailer is enough),
- the next tool call that broke.

shhh prefers honest documentation of a limit over a quiet
workaround that leaks under pressure.
