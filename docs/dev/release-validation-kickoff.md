# Release-validation flow — brainstorm kickoff

Self-contained doc for a fresh brainstorm session. Open it cold,
read top-to-bottom, then start the discussion. Output of the
brainstorm is **a design**, not code. Implementation is a
separate session.

Created 2026-05-27, after the shipment of briefs 01–05 (option D,
v0.3.0 release, viral README, Codex hook, Cursor hook). v0.4.0
is intentionally **not tagged yet** — that is the trigger we are
designing the gate for.

---

## TL;DR

We have a fast-growing product (3 agents wired in one week) and
a green `go test ./...`, but **no honest answer to "is this safe
to release?"** at the integration level. The unit tests prove
the Go functions work; they do not prove that, on a freshly
installed user's machine, a real Claude Code / Codex / Cursor
session will actually see placeholders instead of raw secrets.

Brief 02 acceptance already required a "3 machines" smoke test.
Brief 03 required a hero GIF that exercises the product. Briefs
04 and 05 each shipped a smoke-test note ("verified install JSON
shape on this machine") but neither **proved that the hook fires
in a real session** on its respective agent. That gap is what
keeps us from tagging v0.4.0.

The brainstorm needs to answer two coupled questions:

1. **What is the release-validation flow** — the runbook (or
   script) a developer runs before tagging a release, that
   produces a yes/no signal for "ship it"?
2. **What is the unified bench** — the single artifact that
   compares shhh OFF vs shhh ON across the three agents on the
   same fixture, producing the kind of scorecard the README
   pitch and the launch post will lean on?

These are not separate things. The **bench output IS the
validation evidence**. One artifact, two readers (the developer
gating the release, the visitor reading the README).

---

## Constraints to keep visible during the brainstorm

These are non-negotiable. If a design idea conflicts with one of
these, re-shape the idea, do not re-shape the rule. From
[`CLAUDE.md`](../CLAUDE.md):

- **Rule 1 — "Eval is a library test, not a product."** `go test
  ./...` is the validation command at the library level. Do not
  reintroduce a "product-agnostic benchmark harness," a "four-mode
  matrix," a "tier system," or a task runner as a first-class
  concept. Anything new must be in service of the forcing-function
  scenario, not parallel to it. The existing
  [`cmd/shhh-eval/`](../cmd/shhh-eval/main.go) +
  [`internal/eval/`](../internal/eval/) survive as Go tests of
  the redactor and **do not gate releases**.

- **Rule 2 — "No new phases, tiers, or multi-step internal
  roadmaps."** The flow we design should fit in one
  [`docs/release-checklist.md`](../docs/) (file does not exist
  yet; one candidate name) + one subcommand at most. Not a
  framework.

- **Rule 4 — "Beware elaboration bias."** Ask of every proposed
  step: *does this bring the v0.4.0 ship-gate signal closer?*
  Building "a great harness" is the trap. Building "a single
  script + one verify subcommand that produces a pass/fail
  the developer trusts" is the goal.

- **Rule 5 — Docker / proxy daemon / remote runner forbidden.**
  The flow is short-lived on-demand subcommands the developer
  explicitly invokes, plus a shell script. No background process,
  no long-lived service. `claude --print`-style automation that
  exits with the LLM call is fine; `shhh validate &` daemons
  are not.

- **Forcing function.** Every step must move
  `$ shhh install <agent>; <agent>; read .env` closer to working
  on a stranger's machine. A step that does not pass this check
  is scaffolding.

Also from [`docs/testing-playbook.md`](testing-playbook.md):

- **`go test ./...` is necessary, not sufficient.** Real-shell
  validation is mandatory for any change touching argv parsing
  or install/uninstall. The new flow extends this principle from
  "argv bugs" to "hook-fires-in-real-session bugs."

---

## Existing assets — annotated tour

Read these *before* the brainstorm. Each is a building block
the design will likely compose, NOT an existing solution.

### Already-built fixtures

- [`demo/leaktest/`](../demo/leaktest/) — a fake repo seeded
  with 7 fake-but-realistic secrets across `.env`,
  `src/config.py`, `src/db.js`, `credentials.json`. The values
  use real provider prefixes (`sk_live_`, `ghp_`, …) so the
  detector trips the same way it would on a real `.env`.

- [`demo/manifest.json`](../demo/manifest.json) — the ground
  truth. Lists every secret (file, type, expected raw value),
  every decoy (placeholder-like strings that MUST NOT be
  redacted, i.e. false-positive guards), and a marker per file
  so the verifier can tell which files the agent actually
  opened.

- [`testdata/fixtures/leaky-project/`](../testdata/fixtures/leaky-project/)
  — a sibling fixture used by `shhh scan` tests. Smaller; less
  curated than `demo/leaktest/`. May or may not be merged with
  `demo/leaktest/` as part of the brainstorm output.

- [`testdata/fixtures/hook-playground/README.md`](../testdata/fixtures/hook-playground/README.md)
  — eight numbered tests with **manual** prompts to paste into
  Claude Code. Test 2 is the canonical Read→Edit ledger repro
  (option D, see `docs/known-limitations.md` §1).

### Already-built runners

- [`demo/run.sh`](../demo/run.sh) — drives a REAL Claude Code
  session against `demo/leaktest/` twice (shhh OFF, shhh ON),
  captures both transcripts to a tmp dir, and invokes
  `verify.py`. Costs ~$0.03–0.10 per run (real API calls).
  **This is the closest thing we have to a release-validation
  flow today, but it only covers Claude Code.**

- [`demo/verify.py`](../demo/verify.py) — plain substring
  counting against the ground-truth manifest. Returns
  `0 secrets leaked + 0 decoys redacted = PASS`. **The
  verification model is sound; the question is whether to keep
  it in Python or port to a Go `shhh verify` subcommand.**

- [`scripts/demo.sh`](../scripts/demo.sh) — a different,
  shorter smoke runner. Inventory before the brainstorm whether
  this is redundant with `demo/run.sh` or covers a different
  scope.

### Already-built scoring + display

- [`web/index.html`](../web/index.html) — the public scorecard
  page. It loads diff fixtures from `web/data/` and presents
  "shhh OFF: 7/7 raw secrets reached the model · shhh ON: 0/7"
  cards. **This is the format the brainstorm's bench output
  should converge on.** Whatever shape the runbook produces, it
  must be trivially convertible to (and ideally feed) this
  page's data files.

### Already-built library-level eval (NOT a release gate)

- [`cmd/shhh-eval/`](../cmd/shhh-eval/main.go) +
  [`internal/eval/`](../internal/eval/) — Phase 0 benchmark
  harness. Lives. Builds. Tests. Per CLAUDE.md rule 1, **does
  not gate releases**. The brainstorm should treat this as
  "Go tests of the redactor that happen to have a CLI" — same
  legitimacy as `go test ./...`, no more.

### Already-shipped per-agent installers

- [`cmd/shhh/cmdinstall/cmdinstall.go`](../cmd/shhh/cmdinstall/cmdinstall.go)
  + [`settings.go`](../cmd/shhh/cmdinstall/settings.go) +
  [`cursor_settings.go`](../cmd/shhh/cmdinstall/cursor_settings.go)
  — `shhh install <claude-code|codex|cursor>` is solid. The
  brainstorm assumes install works and focuses one level up.

### Already-shipped audit reader (Claude Code only)

- [`internal/audit/`](../internal/audit/) — knows how to walk
  `~/.claude/projects/<encoded-cwd>/*.jsonl` and extract tool
  calls + content. **The Cursor and Codex equivalents are
  documented (see research docs below) but not implemented yet.
  The brainstorm needs to decide whether the validation flow
  needs an audit-reader per agent, or whether it inspects the
  freshly-written transcript directly.**

### Research docs (where the protocol facts live)

- [`docs/codex-research-2026-05-26.md`](codex-research-2026-05-26.md)
  — Codex protocol, session paths
  (`~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl(.zst)`),
  Bash-only PreToolUse caveat.

- [`docs/cursor-research-2026-05-27.md`](cursor-research-2026-05-27.md)
  — Cursor v1.7+ hook protocol, session paths
  (`~/.cursor/projects/*/agent-transcripts/*.jsonl`),
  GUI-only constraint.

- [`docs/design/read-edit-tracking.md`](design/read-edit-tracking.md)
  — the option D design record. Reading this before the
  brainstorm helps frame the "known limit, document it" pattern
  that the validation flow should respect.

- [`docs/known-limitations.md`](known-limitations.md) — the
  user-facing limits (§1 Claude Code ledger, §2 Codex apply_patch
  gap, §3 Cursor ledger unknown). The validation flow MUST
  verify each documented limit is still accurate; an upstream
  change that lifts a limit silently is a release-time surprise.

---

## Per-agent reality check

| Agent | Headless mode? | Transcript path | Auto E2E feasible? |
|---|---|---|---|
| Claude Code | `claude -p "<prompt>"` (non-interactive `--print` mode) | `~/.claude/projects/<encoded-cwd>/*.jsonl` | **Yes** — already proven by [`demo/run.sh`](../demo/run.sh) |
| Codex CLI | `codex exec "<prompt>"` (verify against current Codex docs) | `~/.codex/sessions/YYYY/MM/DD/rollout-*.jsonl(.zst)` | **Probably** — needs a smoke test in session zero of the brainstorm |
| Cursor | **No headless mode** — GUI only | `~/.cursor/projects/*/agent-transcripts/*.jsonl` | **No, manual** — needs a 30-second human step |

This asymmetry is the central design tension. The brainstorm
should not pretend Cursor automates; it should decide what
"validated Cursor" means when full automation is unavailable.

### Caveats the validation flow must NOT lose track of

- **Codex's PreToolUse only fires for Bash today**
  (`openai/codex#18491`). The flow must distinguish "we
  intercepted Bash and saw zero leaks" from "we intercepted
  every tool and saw zero leaks." The first is the actual
  v0.4.0 promise; the second would be a lie.

- **The Read→Edit ledger limit on Claude Code is documented**
  (option D). The flow must ASSERT that after a redacted Read,
  Edit fails as expected — if Edit *succeeded* on a redacted
  file in a future Claude Code release, the narration would be
  wrong and the limit would have lifted (good news, but
  release-blocking until docs catch up).

- **Cursor's ledger interaction is unverified.** The brainstorm
  must decide if v0.4.0 ships without verifying it (and the
  README §3 caveat stands), or if verifying it is part of the
  release gate (and v0.4.0 waits).

---

## Brainstorm questions

Drive the discussion. None of these have a right answer in this
doc — that is the brainstorm's job.

1. **What is the "ship gate" — the single pass/fail signal?**
   Brief 02 used `shhh version == 0.3.0` plus `shhh licenses`
   inspection on a fresh-machine install. For v0.4.0 we need a
   stronger signal because we are claiming three agents. Is it
   "zero raw secrets in any of three agent transcripts on the
   leaktest fixture"? Is it the same WITH a manual Cursor step?
   How tolerant are we of intermittent agent flakiness (real
   APIs, real network)?

2. **Manual vs scripted by agent.** Claude Code automates today.
   Codex *probably* automates. Cursor does not. Three plausible
   shapes:
   - **Hybrid runbook:** one shell script automates Claude Code
     + Codex, prompts the developer through the Cursor step,
     produces a single scorecard.
   - **Two-tier gate:** automated tier (Claude Code + Codex)
     blocks the release; manual tier (Cursor) is recorded as
     "verified by <date>" and is *informational*, not blocking.
   - **All-manual checklist:** one runbook lists the prompts and
     expected outputs for all three agents, dev runs each by
     hand. Slower but unambiguous.

3. **Where does the verifier live?** Today
   [`demo/verify.py`](../demo/verify.py) is Python. Three
   options:
   - **Keep Python:** zero dependency change. The verifier is
     150 LoC of substring counting; rewriting it gains nothing.
   - **Port to `shhh verify` subcommand:** Go binary, no Python
     dependency on the test machine. Costs a new subcommand
     surface but earns "one binary does everything."
   - **Inline in `shhh bench`:** make
     [`cmd/shhh/cmdbench/`](../cmd/shhh/cmdbench/) cover the
     transcript-level scorecard in addition to its current
     content-level diff. Risky — the existing `bench` is
     engine-comparison, not E2E.

4. **What is the fixture surface?** We have `demo/leaktest/`
   (real-shaped) and `testdata/fixtures/leaky-project/`
   (smaller). Do we converge on one canonical fixture? Add a
   second "happy path" fixture to verify shhh DOES NOT redact
   clean content (false-positive gate)? Hold a third "edge
   cases" fixture for the README scorecard?

5. **What does "unified bench" mean per agent?** The picked
   answer is "Scorecard E2E per agent": same fixture, drive each
   agent with the same prompt, score what reached the model.
   Open sub-questions:
   - Same prompt verbatim across agents, or per-agent prompts
     tailored to each agent's tool selection habits?
   - Run shhh OFF vs shhh ON per agent (6 sessions total) or
     just shhh ON with OFF inferred from prior runs?
   - How many runs per agent for statistical confidence — once,
     three times for variance, or until convergence?

6. **What about the documented limits?** The flow needs to
   verify that the limits in `docs/known-limitations.md` are
   still accurate. Concretely:
   - Claude Code: assert Edit on a redacted file fails (limit
     §1 still holds).
   - Codex: assert apply_patch is NOT covered (limit §2 still
     holds — if Codex shipped #18491's fix upstream, our README
     is wrong).
   - Cursor: ledger behavior — assert one way or the other.
   These are "limit-regression tests." Each is a one-line
   substring check on the transcript.

7. **Where does the cost ceiling sit?** Real API calls cost
   money. brief 02's `demo/run.sh` is ~$0.03–0.10 per pair.
   Three agents × OFF/ON × N runs adds up. Brainstorm should
   set a budget per release validation (~$1? $5?) and let that
   shape the run count.

8. **What is the output format?** The validation flow produces:
   - A scorecard that mirrors `web/index.html`'s shape (so it
     feeds the public scorecard cleanly).
   - A pass/fail boolean for the developer.
   - A change log when the result diverges from the previous
     baseline (a "limit lifted" signal, an unexpected leak).
   Pick the on-disk format (JSON, Markdown, both) and the
   convention for diffing across releases.

---

## Scenario catalog — the things v0.4.0 MUST pass

Not the full design, just the minimum bar. Discuss + extend.

### Per-agent core scenarios

For each of `claude-code`, `codex`, `cursor`:

| # | Scenario | Pass criterion |
|---|---|---|
| 1 | Install hook on a fresh `~/.<agent>/` | settings/hooks file has shhh entries; existing settings preserved |
| 2 | Agent reads fixture `.env` (Read or `cat .env` via Shell) | 0 raw secrets in the resulting transcript |
| 3 | Agent reads source file with hardcoded secret (Read or `head src/config.py`) | 0 raw secrets in transcript |
| 4 | Agent reads `.env.example` (decoy file) | placeholder-like decoys NOT redacted (false-positive check) |
| 5 | Agent reads clean file | hook narration absent, file content unchanged |
| 6 | Uninstall hook | settings file returns to pre-install state (modulo non-shhh entries) |
| 7 | Re-install idempotent | no duplicate entries |

### Agent-specific scenarios

- **Claude Code only:**
  8a. After a redacted Read, attempt Edit on the same path →
  expected fail with "File has not been read yet" (option D
  limit-regression).
  9a. Bash `cat .env` after a previous Read → still redacted
  (session cache stays alive).

- **Codex only:**
  8b. `cat .env` via Codex's Bash → 0 raw secrets in transcript.
  9b. `apply_patch` editing a file with a secret → secret DOES
  reach the model (limit §2 still holds; this is a *required
  fail* — the test passes when it confirms the gap).

- **Cursor only:**
  8c. `Shell` command reading `.env` → 0 raw secrets.
  9c. `Read` tool on `.env` → 0 raw secrets (verify the
  redacted cache redirect works on real Cursor).
  10c. After 9c, attempt Edit/Write on the original path → does
  it fail like Claude Code's option-D limit, or does Cursor
  handle the ledger differently? **This is the verdict the
  research doc deferred.**

### Cross-cutting scenarios

| # | Scenario | Pass criterion |
|---|---|---|
| X1 | `shhh audit` on all three agents' transcripts | reports leak counts per agent without crashing on Codex zstd / Cursor JSONL schemas |
| X2 | `make update-gitleaks-license` produces no diff | embedded LICENSE matches go.mod-pinned gitleaks version |
| X3 | `goreleaser release --clean --snapshot --skip=publish` | builds 6 archives + checksums.txt without warnings |
| X4 | Fresh `curl \| sh` on a clean machine | installs and resolves `shhh version` to the candidate tag |
| X5 | `shhh bench` engine comparison on `demo/leaktest/` | scorecard matches the value the README scorecard claims |

---

## Bench unified — three plausible shapes (tradeoffs only)

### Shape A — One script, three agents, one scorecard JSON

`scripts/release-validate.sh` runs each agent against
`demo/leaktest/` (shhh OFF + shhh ON), writes
`bench-results/<release-tag>/scorecard.json` with per-agent
sub-objects. `shhh verify` (new subcommand) reads the JSON and
prints the pass/fail. The same JSON feeds `web/data/` for the
public scorecard.

- **Pro:** smallest surface, mirrors `demo/run.sh`'s success.
- **Con:** Cursor manual step lives inside the shell script as a
  pause + prompt, which is awkward.
- **Con:** real API costs at $0.03–0.10 × 3 agents × 2 modes = up
  to ~$0.60 per validation. Reasonable but adds up across PR
  reviews.

### Shape B — Two-tier runbook

Markdown doc `docs/release-validation.md` walks the developer
through each step. Tier 1 (`make validate-auto`) automates
Claude Code + Codex and exits with a status. Tier 2 (manual)
is a checkbox list for Cursor. The developer fills in the manual
results in the same JSON the auto tier writes, then runs `shhh
verify` to compute pass/fail.

- **Pro:** honest about Cursor's GUI constraint; doesn't pretend
  to automate what cannot be automated.
- **Pro:** the runbook IS the public smoke-test doc the README
  links to. Multi-purpose.
- **Con:** more moving parts; "did the dev actually fill in the
  Cursor section truthfully?" is on the honor system.

### Shape C — Library-rooted bench

Embed the validation in `cmd/shhh/cmdbench/`. `shhh bench
--release-mode --agent <name>` drives the agent and emits the
scorecard. Reuses the existing bench surface; no new
subcommand.

- **Pro:** smallest user-visible surface.
- **Con:** drags `cmdbench` from "compare engines on content"
  into "drive external agents and grep transcripts" — a much
  bigger ask. Smells like elaboration bias against CLAUDE.md
  rule 1.
- **Con:** harder to express the Cursor manual step inside a
  Go binary; ends up shelling out to a doc anyway.

The brainstorm should rate these on (a) constraint adherence,
(b) blast radius if a step fails, (c) cost in real API dollars
per validation. My personal lean is **A or B, not C** — but
that is a default to be challenged, not a decision.

---

## Suggested brainstorm flow

For the fresh session, in order. Each step is 15–45 minutes.

### Step 1 — Ground the conversation in real artifacts (45 min)

Before any design talk, run these commands and look at the
outputs. The numbers calibrate every later opinion.

```sh
# 1.1 — See the existing Claude Code-only flow work.
make build
cd demo && ./run.sh     # ~5 min, ~$0.10
# Read the printed scorecard. Note what evidence verify.py uses.

# 1.2 — Find each agent's transcript path on this machine.
ls ~/.claude/projects/         # encoded-cwd dirs of past sessions
ls ~/.codex/sessions/           # YYYY/MM/DD/ tree
ls ~/.cursor/projects/          # if the dir exists, per-project transcripts

# 1.3 — Run shhh audit on each one. Confirm Claude Code works,
# observe what fails on Codex / Cursor.
bin/shhh audit
# Codex zstd + Cursor JSONL readers are not implemented yet;
# `audit` may report nothing or crash. Note the gap honestly.

# 1.4 — Test that Claude Code's --print mode works for our
# purposes.
echo 'read the file /etc/hostname' | claude --print --model sonnet
# Confirm a transcript JSONL appears under ~/.claude/projects/.

# 1.5 — Test Codex's exec mode.
codex exec "read the file /etc/hostname"
# Confirm a transcript appears under ~/.codex/sessions/.

# 1.6 — Open Cursor manually. Use Composer to ask "read .env".
# Confirm a transcript appears under ~/.cursor/projects/.
# Time how long the human step takes from app launch to verifiable
# transcript on disk. (Budget it for shape B.)
```

The output of Step 1 is **a calibrated mental model** of what
each agent looks like at the transcript level. Without this, the
design conversation drifts into theory.

### Step 2 — Pick the verifier shape (30 min)

Discuss brainstorm question 3. The choice constrains everything
downstream. Lean on Step 1's findings: if `shhh audit` already
handles the Codex/Cursor transcript schemas honestly, the
verifier could live as a flag on it. If not, a new `shhh verify`
subcommand is cleaner.

Output: one sentence "the verifier is `<X>` and reads `<Y>`."

### Step 3 — Pick the runner shape (30 min)

Discuss brainstorm question 2 + the three shapes (A / B / C
above). The Cursor automation question is the linchpin —
**resolve it before debating runner mechanics.**

Output: one sentence "the runner is a `<shell script | Go
subcommand | hybrid>` invoked by `<command>`."

### Step 4 — Catalogue the limit-regression tests (30 min)

Walk through brainstorm question 6. For each documented limit
in `docs/known-limitations.md`, write down the exact transcript
substring or behavior that would assert the limit still holds.
This is small and tractable; do not over-think it.

Output: 3 one-liners that the verifier checks.

### Step 5 — Sketch the scorecard JSON schema (30 min)

Reconcile with [`web/index.html`](../web/index.html)'s loader so
the same JSON feeds both the developer's pass/fail and the
public page. Two readers, one file.

Output: a JSON skeleton with field names, types, and one example
populated value.

### Step 6 — Write down the brief for the implementation session (15 min)

The output of this brainstorm is **a new
`docs/release-validation-design.md`** that the next session uses
as its kickoff brief. The implementation session does not redo
the design; it just builds what this doc specifies.

Output: a file path + a 5-line summary of what goes inside.

---

## Anti-patterns to spot during the brainstorm

If you catch yourself doing any of these, stop and re-anchor on
CLAUDE.md.

- **Building "a great harness."** The flow is one script + one
  verifier. If the brainstorm is naming subdirectories
  (`internal/validate/`, `internal/bench/`, `internal/runner/`),
  it has drifted.

- **Reintroducing the eval matrix.** The existing
  [`internal/eval/`](../internal/eval/) Cells / Modes / Tasks
  structure is for library tests. **It is not a release-gate
  surface.** Do not extend it to cover the new flow; build the
  new flow next to it.

- **Trusting the model's narration.** The verifier must grep
  the transcript file, not parse the LLM's "I redacted N
  secrets" prose. The model can lie; the bytes cannot.

- **Per-agent forks of the verifier.** If the scorecard logic
  diverges per agent (because Codex zstd is annoying, because
  Cursor JSONL has a different shape), the design has failed at
  one of its levels — likely the audit-reader level. Push the
  per-agent specificity *down* into the transcript readers and
  keep the verifier agent-agnostic.

- **Backwards-compat shims.** Per CLAUDE.md pre-release status,
  if the new flow contradicts how `demo/run.sh` works today, the
  right move is to **replace `demo/run.sh`** with the new flow's
  Claude Code path. Not to add a parallel `demo/run2.sh`.

- **Building for "future agents."** If the brainstorm wants to
  pre-design extensibility for Windsurf, Aider, Continue.dev:
  stop. Each new agent is a brief of its own. The flow needs to
  handle three agents well, not seven hypothetically.

- **Spending real API dollars during the brainstorm itself.**
  Step 1 burns ~$0.10–0.30. Everything after is design talk;
  don't loop on real runs to "test" the design. Test it in the
  implementation session.

---

## Expected output of the brainstorm

The session produces **one new file:**
`docs/release-validation-design.md` (or whatever name fits;
`-design.md` parallels the existing
`docs/engine-architecture.md` naming convention).

That doc must contain:

1. **The chosen verifier shape** (Python kept / `shhh verify`
   built / inline in `shhh bench`) with rationale.
2. **The chosen runner shape** (A / B / C above or a hybrid)
   with rationale and the Cursor automation verdict.
3. **The scorecard JSON schema** with field names and types.
4. **The scenario catalogue locked in** — the list from this
   doc, edited based on Step 1's findings.
5. **The limit-regression assertions** (one per documented
   limit).
6. **The cost ceiling per validation** in real API dollars.
7. **Files-to-touch** for the implementation session.
8. **A timeline estimate** — single session implementable, or
   two PRs (e.g. tier 1 first, manual Cursor tier later).

That doc becomes the kickoff for **a separate implementation
session** that builds the flow. We do not implement in the
brainstorm.

---

## Open question parking lot

These are too specific to resolve here but worth surfacing so
they do not get lost in the brainstorm:

- Should `bench-results/<tag>/` be in `.gitignore` or
  committed? (Probably .gitignored — release artifacts, not
  source.)
- Does the validation flow run in CI, or strictly locally
  before tagging? CI implies API credentials in secrets,
  which is its own can of worms.
- The marker-strings in `demo/manifest.json` are
  fixture-readable plaintext (`LEAKTESTMARKER_ENV`). Do those
  count as "content the model saw" in a strict reading of
  the scorecard? (Probably not — they exist precisely to
  prove the model saw the file, so they SHOULD be in the
  transcript.)
- When the Cursor smoke test is manual, what is the artifact
  the developer commits to prove they ran it? A screenshot? A
  pasted transcript path? The honor system?

These belong on the brainstorm's whiteboard, not at the top of
the agenda.

---

## tl;dr for the brainstorm operator

1. Read this file top to bottom (15 min).
2. Run Step 1's commands (45 min).
3. Work questions 1–8 in the order suggested in steps 2–5.
4. Write `docs/release-validation-design.md` as the artifact.
5. Schedule a separate session to implement it.

If at any point during the brainstorm the conversation feels
like "designing a harness," pause and re-read the
**Constraints** section.
