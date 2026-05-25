# ROADMAP

Working list of the next things to fix on shhh, ordered by how much
each one moves the forcing-function scenario in `CLAUDE.md` forward.
Derived from the 2026-04-15 dogfooding session where shhh intercepted
6 Reads on its own source tree — 0 real secrets, 5 false positives
on docs/tests/fixtures — and cascaded into 2 Read→Edit workarounds
that cost ~30% of the session.

Items 1 and 2 are the only ones that matter until they're done.
Everything below them is contingent on the friction surface shrinking
enough that a user on a monorepo is willing to keep the hook installed
for a full day.

**Status update (2026-05-25):** the Read→Edit cascade described in
item 1 has been substantially reduced by subsequent fixes — the
"every retry re-fails" cycle no longer reproduces consistently.
Residual rough edges remain (see notes below the prompt) but the
project is no longer blocked on this from a distribution standpoint.
Item 2 (detection FPs) is still open and is the next quality lever.

---

## 1. Fix the Read→Edit ledger bug

**Status:** largely resolved as of 2026-05-25. Kept here for the
historical reproduction and prompt context — useful if a regression
ever shows up.

**Problem:** When the hook rewrites `updatedInput.file_path` to a cache
location, Claude Code's internal Read-ledger records the cache path,
not the original. The next Edit/Write on the original path fails with
`File has not been read yet` and there is no way to retry cleanly —
every retry re-enters the hook, which re-rewrites, which re-fails.

**Evidence:** Test 2 in
`testdata/fixtures/hook-playground/README.md` is the reproduction.
Entry 13 in `docs/implementation-log.md` documents how this cascaded
during the visibility-feature session (3 separate files could not be
edited; I had to patch them via `python3` from the Bash tool).

**Why it matters more than new features:** on a monorepo, the
"non-editable" surface grows monotonically over a session. After
~20 minutes of work, any file that ever held a detectable token is
dead to the Edit tool, regardless of whether the token was real.

**Prompt for the next session:**

> Fix the Read→Edit ledger bug in shhh's Claude Code hook.
>
> Read first:
>   1. `CLAUDE.md` — hard rules.
>   2. `testdata/fixtures/hook-playground/README.md` Test 2 — the
>      reproduction. The "RESULTS" block shows the cascade.
>   3. `docs/design/read-edit-tracking.md` — prior design notes on
>      this area; contains context on the Bash-fallback workaround
>      that was shipped instead of a real fix.
>   4. `cmd/shhh/cmdhook/read.go` — the PreToolUse handler that
>      rewrites `updatedInput.file_path`. This is where the bug is
>      introduced.
>   5. `docs/implementation-log.md` Entry 13 — describes how the
>      ledger bug cascaded during the visibility work, including the
>      python-via-Bash workaround.
>
> The fix needs to make `Edit` and `Write` work on their first try
> against the ORIGINAL file path after a redacted Read, with no
> retry loop and no user-visible error. Likely strategies to
> evaluate (not prescribe — investigate first):
>
>   - PostToolUse hook that fires a silent ledger entry for the
>     original path after Read completes
>   - Instead of rewriting `file_path`, inject the redacted content
>     via `additionalContext` and leave `file_path` pointing at the
>     original (costs more context tokens, but fixes the ledger)
>   - Symlink-based cache (original path → cache) rather than a
>     different path (may not work depending on how Claude Code's
>     ledger canonicalizes paths)
>
> Hard constraint: the fix must NOT disable redaction on files that
> fail to edit cleanly — silently letting secrets through is worse
> than a noisy workaround.
>
> Acceptance test: re-run Test 2 in hook-playground/README.md. It
> should succeed on the first Edit call, with no Bash fallback.
> Paste the new transcript under the Test 2 RESULTS section.

---

## 2. Replace the detection engine

**Status:** second blocker. Without this, item 1 fixes a symptom
while the root cause (over-redaction) keeps biting on new file types.

**Problem:** The current detection layer in `internal/detector/` and
`internal/rules/` is a mix of bespoke regex rules, a hand-tuned
Shannon-entropy gate (4.5 bits, 20+ char minimum, integrity-prefix
skip list), and an .env-aware pass. It over-fires on documentation
(hashes in logs, example keys in READMEs, placeholder strings in
test fixtures) and under-fires on the long tail of real secret shapes
that other projects have already catalogued.

**User's stance (2026-04-15):** the custom rules are bad; stop
evolving them in place. The right move is to depend on a vetted
upstream and put our effort on the hook surface (narration, session
map, ledger behavior) rather than on re-inventing gitleaks.

**Why the existing "transcribe gitleaks by hand" decision is stale:**
see `docs/implementation-log.md` Entry 10. That call was made when
the eval harness was framed as the product. Now the hook is the
product, and the tradeoff flips — adding a dependency is cheaper
than carrying the maintenance cost of a bespoke detector that keeps
producing friction on its own dogfood.

**Candidates to evaluate:**
- **gitleaks as a Go library**: same language, mature rule set,
  active maintenance. First pick unless something disqualifies it.
- **trufflehog** via shell-out: richer verification (live credential
  checking), heavier, not Go-native.
- **detect-secrets** (Python): known-good plugin model, but
  cross-language embedding is a tax.

**Prompt for the next session:**

> Replace shhh's detection layer with a vetted upstream engine.
>
> Read first:
>   1. `CLAUDE.md` — especially rule 4 (elaboration bias) and rule 6
>      (no speculative work from PRD claims).
>   2. `ROADMAP.md` item 2 — this entry.
>   3. `docs/implementation-log.md` Entry 10 — the "transcribe
>      gitleaks manually" decision. Understand why it was made so
>      you can understand why it's being reversed.
>   4. `internal/detector/detector.go`, `internal/rules/rules.go`,
>      `internal/redactor/redactor.go` — the current detection
>      surface. Understand the Finding struct shape because the new
>      engine's output must be adapted to it.
>   5. `PRD.md` §§5, 7 — the redact/rehydrate contract and the
>      detection pipeline spec. The engine swap must preserve these.
>
> Step 1 (research, don't code): compare gitleaks library,
>    trufflehog shell-out, and detect-secrets on:
>    - dogfood false-positive rate against the shhh repo itself
>      (run it on every tracked file, count over-redactions on
>      docs/tests/fixtures)
>    - coverage of common real-world secret types
>    - dependency weight and build impact
>    Report findings in a single markdown block. Stop before coding
>    so the user can pick.
>
> Step 2 (after approval): implement the adapter. The detector
>    package becomes a thin wrapper that maps the chosen engine's
>    output shape to `detector.Finding`. Keep the session-map and
>    placeholder layers untouched — they're not the problem.
>
> Step 3 (validation): re-run the dogfood false-positive sweep
>    with the new engine. The number must drop. Paste before/after
>    counts in the commit message.
>
> Hard constraint: the .env-aware pass
>    (`redactor.RedactEnvFile`) stays. Whatever upstream we pick
>    will not have .env context, and that's a real shhh feature
>    that can't regress.

---

## 3. Allowlist / bypass affordance for intentional fixture content

**Status:** unblocked once item 2 lands (or sooner, if item 2 slips).

**Problem:** Some files in any project contain intentional
secret-shaped content: test fixtures with `sk_live_...`, docs
showing example env vars, migration files with placeholder
connection strings. Over-redacting these is not a detection bug —
the content really looks like a secret — it's a policy bug. Shhh
needs a way for the developer to say "I know, this is fine."

**Shapes to evaluate:**
- In-file marker: `# shhh:allow` on the first line of a file
- Config file: `~/.shhh/allowlist.yaml` with glob patterns
  (`testdata/**`, `docs/**`, `**/*_test.go`)
- Auto-heuristic: if a file is under a conventional fixture/doc
  directory AND the secret is a known public example
  (AKIAIOSFODNN7EXAMPLE, etc.), skip
- Combination: config for project-wide, marker for one-offs

**Prompt for the next session:**

> Design and implement the allowlist/bypass affordance for shhh.
>
> Read first:
>   1. `CLAUDE.md` — hard rules 1 (no new phases), 2 (no tiers),
>      6 (no speculative PRD work).
>   2. `ROADMAP.md` item 3 — this entry.
>   3. `testdata/fixtures/hook-playground/` — the canonical example
>      of a directory that SHOULD be allowlisted in a real usage.
>   4. `cmd/shhh/cmdhook/read.go` — where the bypass check needs to
>      hook in (before calling `LoadRedactor`, ideally — if a path
>      is allowlisted we don't even load the session map).
>   5. `cmd/shhh/cmdhook/bash.go` — the Bash wrapper has no path
>      context at PreToolUse, so the allowlist there is harder.
>      Decide whether Bash gets a narrower allowlist shape (command
>      prefix, executable path, etc.) or nothing at all for v1.
>
> Step 1 (design, don't code): write a short proposal in
>    `docs/design/allowlist.md` covering:
>    - which shape(s) to implement and why
>    - precedence rules (in-file vs config vs auto)
>    - how the user discovers an allowlist-triggered skip (narration
>      should probably mention it in a one-liner so there's no
>      silent bypass)
>    - failure modes: what if the allowlist file is malformed,
>      unreadable, contains broken globs
>    Stop and wait for approval.
>
> Step 2 (implementation): ship the approved design as one commit.
>
> Hard constraint: default behavior without an allowlist must be
>    IDENTICAL to today. The allowlist is opt-in. A user who doesn't
>    configure one must see the same redactions as before.

---

## 4. Compress the narration; make "IMPORTANT — how to modify this file"
   conditional

**Status:** blocked on item 1. If the Read→Edit ledger bug is fixed,
the 8-line "use Bash instead" block in the narration becomes
obsolete and can be deleted. Doing this before the ledger fix is
backwards.

**Prompt for the next session:**

> After the Read→Edit ledger bug is fixed, sweep
> `cmd/shhh/cmdhook/read.go::narrateRedactions` and
> `cmd/shhh/cmdredact/cmdredact.go::buildBashNarration` for text
> that described the workaround. Anything of the form "Edit and
> Write will fail, use Bash instead" should be deleted. Keep the
> per-finding listing and the "shhh protected N secrets" opener;
> those remain useful.
>
> Read first:
>   1. `cmd/shhh/cmdhook/read.go` — the current narration function.
>   2. `cmd/shhh/cmdhook/cmdhook_test.go` — the narration tests.
>      The test that asserts "use Bash instead" appears in the
>      narration needs to flip to asserting that it DOES NOT appear.
>
> Also consider: on a long session, the narration repeats the same
> framing block N times. Investigate whether Claude Code's hook API
> exposes a "has this session already seen narration X" signal. If
> not, ship the first iteration unchanged and note in the commit
> message that session-aware narration compression is deferred.

---

## 5. Smarter session scoping for the redaction cache

**Status:** nice-to-have. Only matters once items 1 and 2 are done
and shhh is actually running on a real monorepo long enough for
the cache to matter.

**Problem:** Today, every redacted Read creates a file under
`~/.shhh/sessions/<id>/cache/`. On a long monorepo session that
touches hundreds of files, the cache directory grows linearly and
is wiped on SessionEnd. No TTL, no eviction, no dedupe across
sessions that hit the same file.

**Prompt for the next session:**

> Audit the session cache behavior in `cmd/shhh/cmdhook/sessionstore.go`
> and `cmd/shhh/cmdhook/hashname.go` (and wherever the cache path is
> computed) for a real monorepo workload.
>
> Read first:
>   1. `cmd/shhh/cmdhook/sessionstore.go`
>   2. `cmd/shhh/cmdhook/hashname.go`
>   3. `cmd/shhh/cmdhook/read.go` — where `RedactedPath` is called.
>
> Questions to answer, in order:
>   - How large does the cache get on a real session (find a
>     monorepo to test against, measure).
>   - Are there files that get redacted once and never again in a
>     session? If yes, the cache is just taking space.
>   - Is there a safe TTL or LRU eviction that doesn't break the
>     Read-ledger contract?
>
> Do NOT ship a cache change until items 1 and 2 are done — the
> friction surface is dominated by them, not by cache size.
> This entry exists mainly so the eventual audit has a home.

---

## Out of scope for this roadmap (on purpose)

- **MCP integration.** CLAUDE.md rule 5 is explicit: not in scope.
- **Docker runner / proxy daemon / remote runner.** Same.
- **Eval harness expansion.** It is a library test, not a product
  (CLAUDE.md rule 1).
- **PRD-driven features not listed above.** CLAUDE.md rule 6: only
  implement what the current milestone requires.

---

## Forcing-function check for this roadmap

Every item above passes the CLAUDE.md §"Forcing function" check
against the working demo:

> `$ shhh install claude-code; claude; claude> read .env`
> `  (Claude sees [STRIPE_LIVE_KEY:sk_live_...], not the raw key.)`

- Item 1 (ledger bug) is the difference between "the demo works for
  one Read" and "the demo works for a real session".
- Item 2 (detection engine) is the difference between "the demo
  works on the `.env` in the fixture" and "the demo doesn't also
  mangle the README that explains the demo".
- Items 3–5 only make sense in a world where 1 and 2 have shipped.

If a future session is tempted to pick up an item below the one
that's actually blocking, it has failed the check.
