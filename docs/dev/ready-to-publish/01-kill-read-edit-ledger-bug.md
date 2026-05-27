# 01 — Kill (or honestly document) the Read→Edit ledger bug

## Context

The Claude Code hook redacts a file by rewriting `updatedInput.file_path`
in the `PreToolUse/Read` callback to point at a per-session cache copy
(e.g. `~/.shhh/sessions/<sid>/<hash>.env`). Claude Code's internal
"this file has been read" ledger then records the cache path. The
next time Claude tries `Edit` or `Write` against the original path,
the precondition check fails with:

```
File has not been read yet. Read it first before writing to it.
```

This was the original ROADMAP item #1. The status note in `ROADMAP.md`
says "largely resolved as of 2026-05-25" — the most painful cascade
("every retry re-fails") no longer reproduces consistently. But it
has not been **killed**. It still bit during the 2026-05-26 engine
refactor session: every test file containing fake-but-real-shaped
secrets (`factory_test.go`, `cmdbench_test.go`, `cmdhook_test.go`,
`project_scope_e2e_test.go`) had its `Edit` blocked the moment shhh's
own hook redacted the Read; the working session had to fall back to
Python-via-Bash for each one. That's a published-tool problem: if a
random Redditor hits it 5 minutes in, they bounce.

The full design context is in
[`docs/design/read-edit-tracking.md`](../design/read-edit-tracking.md).
Strategies A (PostToolUse content swap), B (additionalContext bridge),
C (symlink) were all evaluated and ruled out by the current Claude
Code hook API. That doc is the permanent record — read it first.

## Two acceptable outcomes

This brief is intentionally either-or. Pick one before writing code.

### Option K — actually kill it

The hook stops rewriting `file_path`. Instead, it injects the
redacted content via a different vector that Claude Code's ledger
accepts as a valid Read of the original path.

Candidates to evaluate (the design doc rules out the obvious ones,
but the API may have evolved since):

1. **`additionalContext` + decision: "block"** — block the real
   Read but supply the redacted content as `additionalContext`.
   Tradeoff: doubles context tokens for the original file and the
   redacted version may not flow back to the model the same way a
   Read result does. Verify Claude Code's behaviour on this path
   before assuming it works.
2. **`updatedInput.file_path` to the original path + a sidecar
   protocol** — leave `file_path` untouched and use a hook-level
   side channel (env var, named pipe, transient state file) to
   communicate "this content has been redacted". Likely impossible
   because the Read tool reads the file from disk and there is no
   in-flight redaction hook on the result. Treat as fallback if
   (1) and (3) both fail.
3. **PostToolUse content rewrite** — re-check the hook API: if
   PostToolUse on Read now supports a result-content field (it
   didn't before; the design doc records this), use it. Real fix
   if true.

For each candidate, write a 3-line `verdict.md` note in the design
doc before coding. Update `docs/design/read-edit-tracking.md`'s
"strategies considered" list so the next agent doesn't redo the
analysis.

**Acceptance for option K:**
```
$ shhh install claude-code
$ claude
claude> read .env                              # has STRIPE_LIVE_KEY=sk_live_…
claude> edit .env line 5 "STRIPE_LIVE_KEY=…"
# Edit succeeds on first try, no Bash fallback, no "File has not
# been read yet" error.
```

Re-run Test 2 in
[`testdata/fixtures/hook-playground/README.md`](../../testdata/fixtures/hook-playground/README.md)
and paste the new transcript under the existing RESULTS block.

### Option D — document it loudly

If the API has not budged and option K is still impossible, the
honest move is to surface the limitation up-front so a first-time
user knows what to expect.

Concretely:
- Add a "Known limitations" section near the top of `README.md`,
  one paragraph, links to the design doc, gives the workaround
  (use Bash with sed/python when Edit fails).
- The hook's redaction trailer already prints
  `IMPORTANT — how to modify this file: ... use the Bash tool`
  when it redacts. Verify this still fires; if not, restore it.
  Source: `cmd/shhh/cmdhook/read.go::narrateRedactions` (search
  for "use the Bash tool instead"). The text was ROADMAP item #4
  marked for deletion *after* option K — under option D it stays
  and you keep ROADMAP item #4 closed-as-wont-fix.
- File a `docs/known-limitations.md` with the full repro and the
  three strategies that don't work yet, linked from the README.
- Open a tracking GitHub issue with the exact `claude --version`
  range it affects, so when Anthropic ships a new hook API field
  the issue is auto-discoverable.

**Acceptance for option D:**
A skeptic clicking through the README, reading the limitations
section, and trying the demo on a redacted file *expects* the
Bash fallback before they see it. No surprise, no rage.

## Files to touch

For both options:
- `cmd/shhh/cmdhook/read.go` — the redirection logic
  (`handleRead`, `RedactedPath`, `narrateRedactions`).
- `docs/design/read-edit-tracking.md` — update strategies list.
- `testdata/fixtures/hook-playground/README.md` Test 2 — paste
  results.
- `README.md` — option D adds the section; option K removes any
  workaround disclaimers there.

For option K only:
- Any hook-narration tests in `cmd/shhh/cmdhook/cmdhook_test.go`
  that currently assert the "use Bash" string need to flip to
  asserting it does NOT appear (because the fix made the
  workaround unnecessary).
- Update `ROADMAP.md` item #4 to "CLOSED — no workaround text to
  compress".

## Decision rule

Spend the first hour on research only (verifying current Claude
Code hook API behaviour against options 1–3). If none of the three
work cleanly, ship option D in the second hour. The cost of D is
low (one doc + one README section); the cost of guessing at K and
shipping a half-working fix is paying down regression debt later.
