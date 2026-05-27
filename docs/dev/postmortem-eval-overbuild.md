# Postmortem: the eval-harness over-build (Phase 0 drift)

**Filed:** 2026-04-13, after step 15, before touching step 16.
**Status:** canonical. Read before touching anything in this repo.

## TL;DR

shhh was supposed to be a thin hook that drops into coding agents (Claude
Code, Codex, Cursor) and redacts secrets before they reach the LLM.
Between roadmap step 0 and step 15, we built a 39-rule detector, a
session map, a redactor, a scanner CLI, and a seven-task eval harness
with a four-mode matrix — **none of which runs inside a coding agent.**
At step 16 the plan was to containerize Claude Code inside Docker so the
eval harness could measure it. That was the moment the mis-scoping
became visible. This document is the diagnosis, so we don't repeat it.

## What the product was supposed to be

PRD §1: a tool that intercepts secrets at the coding-agent boundary
(hook, plugin, skill, MCP — whatever each agent supports), replaces them
with typed placeholders, and lets the agent keep reasoning. Zero
friction. Installs in 30 seconds. Uninstalls cleanly. The positioning
statement calls it "multi-layered protection (native hooks + universal
proxy + MCP) that installs in 30 seconds and works with every major
coding agent."

Nothing in the above sentence says "eval harness."

## What we actually built

**Useful, aligned with PRD** (keep):
- `internal/detector` — 39 regex rules, entropy pipeline, allowlist.
- `internal/session` — session-scoped placeholder map, salted determinism.
- `internal/redactor` — redact/rehydrate round-trip, structural URL
  placeholders.
- `internal/rules` — built-in rule set + gitleaks-derived additions.
- `internal/scanner` — file walker with sensitive-path detection and
  .env cross-reference.
- `cmd/shhh scan` and `cmd/shhh redact` — the CLI entrypoints that
  actually exist.

**Overbuilt, drifted, or misframed** (demote or drop):
- `internal/eval/` — a product-agnostic `Redactor` interface, an
  `Adapter`, a task framework, a four-mode matrix, a runner with
  "Expected-by-mode" semantics, four glyph states, exit-code policy.
  Treated as its own product. Was supposed to be unit tests.
- Seven eval tasks (t01, t02, t03, t05, t07, t08, t09) with rubrics,
  compensatory-tool simulations, tempdir fixtures, and per-task design
  narratives in the implementation log.
- `CompensatoryTools` as "first-class product surface" — implemented as
  a Go bundle that no real agent ever calls, validating MCP tools for
  an MCP server that doesn't exist.
- A 22-step roadmap in `implementation-roadmap.md`. Steps 0–15 are
  eval work. Step 16 is "Docker Claude Code runner." Steps 17–19 are
  "tasks that need a real agent." Steps 20–22 are daemon + baseline
  doc + repo split. The actual product (a hook in Claude Code) is
  nowhere on this roadmap by name.
- An implementation log of ~1100 lines across 11 entries documenting
  "what we learned."
- A step-16 plan that would have containerized Claude Code, cached LLM
  responses on disk, and invented rubric-tolerance rules — to validate
  a product that does not exist.

**Not built at all** (but the PRD says this *is* the product):
- A Claude Code hook.
- A Codex integration.
- A Cursor integration.
- An `install` command.
- A demo that anyone can run against a real coding agent.

## Where the absurdity became visible

Step 16 planning. The assistant produced a three-path tradeoff document
covering Docker vs host-only, response caching, API-key handling,
rubric tolerance, prompt-injection positive-control threat models, and
a recommendation for a one-week spike. The user's reply, in substance:

> *"I don't understand any of this. The idea was very simple. Why are
> you talking to me about Docker and running Claude with `claude -p`
> when the goal was a hook that swaps secrets for references?"*

That was the forcing function the roadmap never supplied. One human
sentence, in frustration, did what eleven log entries couldn't.

## Why it happened

1. **Principle 7 inverted.** PRD §2 principle 7 says "Eval-driven, not
   architecture-driven." The *intent* is: validate every product claim
   with reproducible numbers before shipping. The *interpretation* was:
   build the eval first, fully, in isolation, before writing the
   product. Same words, opposite meaning. Nothing in the system
   noticed the inversion.

2. **No forcing function tied to the product.** Every step ended at a
   "green checkpoint" (`make test`, `make bench`). Green on the unit
   tests meant the *library* worked. Green on the eval meant the
   *simulated* agent saw consistent placeholders. Neither ever meant
   "a real coding agent on a real machine redacts a real secret
   before it hits the network." That end-state was never named as
   the definition of progress.

3. **A linear step dependency that wasn't real.** The roadmap chained:
   task 6 → task 5 → task 3 → task 1 → "mock agent harness" →
   "calibration" → "foundation." Each link is locally defensible. The
   total chain happens to fully postpone the product. Nothing in the
   chain is load-bearing: a real hook in a real agent would have given
   real signal for every downstream eval question. The eval-before-
   product ordering was a choice presented as a constraint.

4. **Commits that felt productive masked the stasis.** Each log entry
   taught us something real (tokenization vs entropy, rehydrate vs
   tool_use args, structural redaction for URLs). Those learnings are
   real. But they were taught on simulations, not on agents. The
   visible momentum kept the loop fed.

5. **AI agents elaborate any scaffolding they're given.** Prior Claude
   sessions kept extending the harness because extending the harness
   was the closest local action matching "next roadmap step." No prior
   session stopped to ask: does this extension bring us closer to the
   product or further? The roadmap said step N+1 was next, so step
   N+1 got built. A human had to notice the direction. The user did,
   at step 16.

## Warning signs that should have triggered the stop earlier

- **Entry 6** (task 1 + compensatory tools) contains this admission:
  *"The 'mock agent harness' from roadmap step 6 was a mirage. When
  I wrote task 1 the right shape turned out to be: each task encodes
  its own simulation, and the only shared abstraction is
  CompensatoryTools (which is not an agent at all)."* That was the
  moment we discovered the foundational step the whole roadmap
  depended on did not exist as a coherent thing. The correct
  conclusion was: the roadmap is wrong. The actual conclusion was:
  rescope step 6 and keep going.
- **Entry 5** (Makefile) treats *"a skeptic clones and runs
  `make bench`"* as the Phase 1 ship criterion. That criterion
  verifies the library passes its tests. It does not verify any
  claim from PRD §1. A skeptic checking the PRD wants to see a
  hook running, not a matrix rendering.
- **Every "Next" field** in log entries 1–11 points to the next
  roadmap step. Never once to "install the hook and watch it work
  in Claude Code on this machine." There was no line in the log
  that would have revealed the product didn't exist.
- **`shhh --help`**, at any point, would have shown: the binary has
  `scan` and `redact` subcommands. It does not have `install` or
  `hook`. The PRD's §8 installation flows describe features that
  simply are not in the code. Running one command exposed the gap.
  Nobody ran it.

## What's kept, what's demoted, what's thrown out

### Kept, unchanged, as the core library

- `internal/detector`, `internal/session`, `internal/redactor`,
  `internal/rules`, `internal/scanner`.
- `cmd/shhh scan`, `cmd/shhh redact`.
- `testdata/fixtures/leaky-project/`.
- `eval-corpus/public-examples/` (as fixture data for the detector
  tests, not as a "benchmark corpus").

### Demoted to library tests

- `internal/eval/*` stays on disk but is no longer framed as a
  "product-agnostic benchmark harness." The files run under
  `go test ./internal/eval/...` as regression checks for the
  redactor. The four-mode matrix, the `Expected(mode)` semantics,
  and the task/tier classification stop being public concepts.
  No rename, no refactor in this pass — the goal is to stop
  treating eval as a product, not to delete working tests.
- `cmd/shhh-eval/` stays compilable for now but drops out of
  `make bench` as a first-class target. Re-decision after milestone 1.

### Thrown out

- The 22-step `implementation-roadmap.md`. Replaced.
- The step-16 Docker Claude Code runner plan, in full.
- Roadmap steps 17–19 as currently framed (task 6 prompt-injection
  positive control, task 4 write round-trip, task 10 tool-use
  round-trip). These come back as *manual tests against a real
  agent* once the hook ships, not as automated eval tasks.
- Roadmap step 20 (daemon + Unix socket stub). The daemon appears
  only if a real hook implementation reveals a shared-state
  requirement. Speculative today.
- Roadmap step 21 (baseline results doc as Phase 1 launch artifact).
  The launch artifact is a working hook in a coding agent, not a
  document.
- Roadmap step 22 (split `shhh-eval` into standalone repo). There
  is no `shhh-eval` product to split.
- "CompensatoryTools as first-class product surface" framing.
  Compensatory tools may come back as real MCP tools later,
  justified by observed need during real agent testing.

## Lessons (short form, for CLAUDE.md cross-reference)

1. **The product is a hook in a coding agent.** Before anything else,
   run `shhh install claude-code` and watch a secret not reach the
   network. That scenario is the definition of done for v0.1.
2. **Every session ends with the same question:** is the demo closer
   to working? If today's work did not bring it closer, today's work
   was scaffolding and the next session needs to correct course.
3. **Eval is a library test, not a product.** `go test ./...` is the
   validation command. Anything that cannot be answered by a Go test
   is a product question and should be answered by running the hook
   against a real agent.
4. **No "phases."** Phases were a postponement device. Replace with
   named milestones, each with a user-shippable artifact.
5. **AI agents will elaborate any scaffolding they're given.** The
   only reliable counter is a tightly-worded `CLAUDE.md` and regular
   re-grounding in the user's actual goal. See that file.

## What happens next

A new session starts from `docs/implementation-roadmap.md` milestone 1
(Claude Code hook). This postmortem is the only artifact that
describes the drift. The implementation log entries 1–11 are retained
as history — they contain real calibration learnings that will
transfer — but they are no longer the primary reading. See the new
`CLAUDE.md` for reading order.
