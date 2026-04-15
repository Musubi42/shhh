# Read→Edit tracking: why Edit fails on redacted files

## The problem

When `shhh` intercepts a `PreToolUse/Read` call, it rewrites
`updatedInput.file_path` from the original path (e.g.
`~/project/.env`) to a per-session cache path containing a redacted
copy (e.g. `~/.shhh/sessions/<sid>/<hash>.env`).

Claude Code's internal "has this file been read?" ledger records the
**rewritten** path, because that is the path the `Read` tool actually
executed against. The next time Claude tries to `Edit` or `Write` the
*original* path, Claude Code fails the precondition:

```
Error: File has not been read yet. Read it first before writing to it.
```

This is reproducible. See `testdata/fixtures/hook-playground/README.md`
Test 2 for the exact repro; Test 3 (reading a clean file with no
redactions) is the control and proves the break is specifically the
`file_path` rewrite, not some broader hook interference.

## Why we cannot fix this in the hook API as it stands

We asked the Claude Code hooks documentation whether any hook field
can bridge the ledger gap. Three strategies were considered and all
three are impossible with the current API. This section is the
permanent record of that investigation so nobody has to redo it.

### Strategy A — mutate the tool result in `PostToolUse/Read`

**Idea:** let the real `Read` execute against the real file, then in a
`PostToolUse` hook, replace the textual content the LLM sees while
leaving `file_path` untouched. The ledger would record the real path,
Edit would work, and we would not need the cache file at all.

**Verdict: impossible.** `PostToolUse` for built-in tools (Read, Edit,
Write, Bash) only exposes `additionalContext` (advisory text appended
to the tool result) and `decision: "block"`. There is no field to
replace the content body. The only content-replacement field,
`updatedMCPToolOutput`, applies **exclusively to MCP tools**. `Read`
is built-in.

This is the strategy that would be "the right fix" if Claude Code
ever exposes content replacement for built-in tool outputs. We should
file feedback asking for this.

### Strategy B — short-circuit with a synthetic result in `PreToolUse`

**Idea:** in `PreToolUse/Read`, block the real tool execution and
return a synthetic tool result containing the redacted content, as if
the Read had run. Claude Code records the real path in its ledger
because that is the path the LLM requested.

**Verdict: impossible.** `PreToolUse` supports `permissionDecision:
"deny"` to block execution, but there is no accompanying field to
return a synthetic result in place of the blocked call. Denying stops
the Read; it does not replace it. No `toolResultOverride`,
`syntheticOutput`, or equivalent exists in the documented
`hookSpecificOutput` schema.

### Strategy C — inject "file was read" state into the ledger directly

**Idea:** keep rewriting `file_path`, but emit a separate hook signal
telling Claude Code "also mark the original path as read".

**Verdict: impossible.** The Read-ledger is internal to Claude Code
and not exposed to any hook. Not `SessionStart`, not
`UserPromptSubmit`, not `PreToolUse`, not `PostToolUse`. No
documented field mutates it.

### The MCP escape hatch (Strategy D, not chosen)

There is one theoretical workaround the docs do hint at: expose a
local MCP server `shhh-read` with its own `read_file` tool. MCP tool
output *can* be replaced by `PostToolUse` via `updatedMCPToolOutput`.
The flow would be:

1. Claude calls `shhh-read.read_file(path)` instead of the built-in
   `Read`.
2. The MCP tool reads and redacts the file.
3. `PostToolUse/shhh-read.read_file` mutates the output one more time
   if needed.
4. The built-in Read ledger is never touched, so Edit keeps working
   on the original path — as long as Claude previously called the
   built-in `Read` on it at some earlier point.

**Why we are not doing this.** Three reasons, in order of weight:

1. **It does not actually solve the problem.** The `Edit` tool still
   requires the *built-in* `Read` to have been called on the target
   path. A successful `shhh-read.read_file` call does not populate
   Claude Code's Read-ledger for the built-in `Edit`. So either Edit
   still fails (unchanged from today) or Claude reads the file twice
   (once via MCP for the redacted view, once via built-in `Read` for
   the ledger entry) — and the second read defeats the whole point
   because it would fetch the raw file past `shhh`.
2. **CLAUDE.md excludes MCP from scope.** "an MCP compensatory-tool
   server as a first-class surface" is explicitly listed as out of
   scope. Reintroducing MCP would need a much stronger justification
   than a workaround for a ledger limitation.
3. **It is choice architecture against us.** It relies on Claude
   picking `shhh-read` over the built-in `Read` every time. We have
   no way to force that. A single missed call and the user's secret
   leaves the machine.

So MCP is documented here as a known alternative but not pursued.
Revisit if (a) Anthropic exposes `Read` ledger mutation, or (b) the
MCP ecosystem grows a way to *redirect* built-in calls rather than
add parallel ones.

## What we actually do: explicit Bash fallback

Since we cannot make `Edit` work, we do the next best thing: tell
Claude, in the `additionalContext` of the rewritten Read, that `Edit`
and `Write` will fail on this file and that it should use `Bash`
(`sed -i`, `tee`, `printf >>`, `python -c`, etc.) instead. The Bash
wrapper already runs through `shhh redact --session`, so output stays
safe.

Implemented in `cmd/shhh/cmdhook/read.go` `narrateRedactions`.

### Why this is acceptable rather than merely a workaround

- **It turns a silent failure into a predictable one.** Before the
  narration change, Claude would try `Edit` two or three times, see
  two or three "File has not been read yet" errors, and finally fall
  back to `Bash` on its own. Now it skips the failed attempts.
- **It is coherent with how `shhh` already treats Bash.** Bash is
  already a first-class interception surface (`cmd/shhh/cmdhook/bash.go`),
  so recommending Bash as the write path does not widen the trust
  boundary.
- **It degrades gracefully.** Clean files (no redactions) take no
  narration and suffer no restriction — `Edit` on them works
  normally. Only files that actually contained a secret get the
  "Edit is unavailable" guidance.
- **It is reversible.** If Anthropic ships Strategy A tomorrow, we
  delete `narrateRedactions`' warning block and we are done. The
  rest of the hook is unchanged.

### Known downsides

- Claude might still *try* `Edit` first out of habit before reading
  the narration carefully. The narration uses the word "IMPORTANT"
  and explicit tool names to reduce this, but it is ultimately a
  model-behavior issue we cannot force.
- Bash edits are syntactically noisier than `Edit`: a `sed -i` on a
  large file is harder to review than a three-line diff. Users who
  read Claude's actions in the TUI will see uglier commands.
- This only covers `Read` → `Edit`. `Read` → `Write` has the same
  problem and the narration mentions both, but we have not yet
  reproduced a `Write` failure case in the fixture.

## Feedback to Anthropic

File via `/feedback`:

> The hooks API lacks a way to replace content returned by built-in
> tools (Read, Bash). Our use case is a local pre-LLM secret
> redactor: we need `Read` to execute against the real file path
> (so Edit can follow) but return redacted text to the model. Today
> we rewrite `updatedInput.file_path` to a cache copy, which breaks
> Claude Code's Read→Edit tracking. Either (a) a
> `PostToolUse.updatedOutput` field for built-in tools, symmetric
> with `updatedMCPToolOutput`, or (b) a
> `PreToolUse.markFileAsRead(path)` side-effect would unblock this
> cleanly. Repro: any hook that rewrites `Read.file_path`, then try
> to `Edit` the original path.

## References

- `cmd/shhh/cmdhook/read.go` — the hook and `narrateRedactions`.
- `testdata/fixtures/hook-playground/README.md` — tests 2 and 3.
- Research session on hooks API, 2026-04-15 — three strategies
  ruled out, Bash-fallback chosen.
