# Implementation roadmap

This document is the logical order for Phase 0 implementation work. Each
step is chosen so that its deliverable teaches us something that improves
the next step. Doing the steps out of order wastes information.

The roadmap is a living document. Reorder it when an implementation
reveals that a later step is actually a prerequisite of an earlier one
(log the reordering in `implementation-log.md`).

## Reading the columns

- **#** — stable step number. Never renumbered, even on reorder.
- **Step** — short title.
- **Why now** — the single most important reason this step is in this
  position. If we can't articulate a "why now" in one sentence, the step
  is probably in the wrong place or not well-scoped.
- **Teaches** — what the next steps will benefit from knowing once this
  step lands. This is the compounding-learning justification.
- **Depends on** — earlier steps that must be complete.
- **Status** — `done` / `in progress` / `blocked` / `not started`.

## Steps

| # | Step | Why now | Teaches | Depends on | Status |
|---|------|---------|---------|-----------|--------|
| 0 | Phase 0 foundation (PRD rewrite, docs, core library, scan CLI, session map, redactor, scanner) | Everything downstream needs a working detector and a real Go module. | The full shape of the detection pipeline and the session-map API. | none | ✅ done |
| 1 | Eval harness scaffold + task 7 + task 8 | Validates the harness interface before we build ten tasks against it. Tasks 7 and 8 need no agent, so they are the cheapest smoke test. | The product-agnostic `Redactor` interface, the matrix report shape, and the fact that running task 8 immediately surfaces calibration bugs. | 0 | ✅ done |
| 2 | Detector calibration (token regex fix, charset-diversity gate, integrity-prefix skip, known-example allowlist) | Task 8 surfaced four false positives on the first run; fixing them before writing more rules or more tasks prevents compound error. | Which detector invariants matter (tokenization > threshold tuning) and how eval drives product fixes. | 1 | ✅ done |
| 3 | Scanner `.env` cross-reference (PRD §7.2 stage 3) | The gate function (`CheckEnvValue`) was already written but not wired. Leaving it dangling would mean custom secrets silently escape the scan. | How a two-pass scan composes with the detector without duplicating findings (span-covered check). | 0 | ✅ done |
| 4 | Makefile reproducibility contract | Phase 1 ship criterion is `make bench` reproducing numbers on a fresh machine; we need the contract in place before adding more tasks so every commit runs the full suite the same way. | The shape of the reproducibility surface (`build`, `test`, `vet`, `bench`). | 1, 2, 3 | ✅ done |
| 5 | Implementation log + roadmap docs | The user's explicit ask: future work must happen "en connaissance de cause." Without a running log, decisions from earlier sessions become invisible tribal knowledge. | The documentation format every subsequent step will add an entry to. | none, but retroactive entries need 0–4 | ✅ done |
| 5.5 | Runner `Expected`-by-mode semantics (inserted retroactively) | Discovered while landing task 1: a Tier-1 task whose *design* fails in `redact` mode cannot be modeled by a runner that treats every failure as a regression. Adding `Task.Expected(mode)` + four-glyph matrix (`✅ pass` / `✅ fail-ok` / `❌ regression` / `⚠️ surprise`) was mandatory, not optional. | That some steps are invisible until a later implementation forces them. The roadmap is a living doc — when this happens again, insert the step with a fractional number and log the reordering. | 1 | ✅ done |
| 6 | Shared primitives for task-internal agent simulation | Gives us a way to run agent-dependent tasks (1, 3, 5, 9) deterministically and cheaply, before investing in Docker and real Claude Code. **Re-scoped:** originally "mock agent harness." Entry 6 in the log explains why there is no standalone mock-agent component — each task encodes its own simulation, and the only shared piece is `CompensatoryTools`. | The agent-interaction boundary lives *inside* each task's `Run`, not in a separate type. Refactoring happens when a real runner lands. | 1 | ✅ done |
| 7 | Task 1 — JWT decode & claims inspection | The single task most likely to invalidate "placeholders preserve reasoning" for structured secrets. Implementing it forces us to decide: richer placeholders (scope creep) vs compensatory MCP tools (cleaner). | Whether the "compensatory tool" pattern actually works end-to-end, and what metadata the placeholder format needs to carry. | 6 | ✅ done |
| 8 | Compensatory tool `decode_jwt_safely` | Task 7 will fail in `redact-only` mode; implementing the tool makes `+compensatory` mode pass. This is our first real compensatory tool and it sets the pattern for the rest. | The interface compensatory tools expose to the harness, and the plumbing that lets them hit the session map. | 7 | ✅ done |
| 9 | Task 3 — Consistency across files (plaintext + base64 + docker-compose env) | After task 1 we know the session-map-by-value approach works for one value; task 3 tests cross-file identity and forces base64 normalization into the detector. | Whether the detector needs a canonicalization pass before map lookup, or if content-hash-on-raw-bytes is enough. | 6, 8 | not started |
| 10 | Compensatory tool `compare_secrets` | Needed iff task 3 fails in pure `redact+rehydrate` mode. Implementation is probably trivial (`session.Compare` already exists). | How thin a compensatory tool can be when the session map already has the right primitive. | 9 | not started |
| 11 | Task 5 — Grep for hardcoded secret | After task 3 we have cross-file identity; task 5 tests whether the agent can find *new* occurrences outside the files it has seen. This is the test for PostToolUse redaction consistency (not built yet) vs compensatory tool fallback. | Whether we need PostToolUse in Phase 0 at all or if `grep_hardcoded` alone is enough. This directly informs Phase 3 architecture. | 6, 8 | not started |
| 12 | Compensatory tool `grep_hardcoded` | Needed by task 5. Searches a directory for occurrences of the real value behind a placeholder ID. | Whether search-by-value can stay inside the session boundary or needs filesystem escalation. | 11 | not started |
| 13 | Gitleaks library integration | Now that detection signal is validated by eval tasks, widening coverage is a mechanical improvement. Doing this *before* tasks 1–5 would mean detecting more without knowing what matters. | How many of our hand-written rules can be retired, and how much runtime overhead gitleaks adds. | 2 | not started |
| 14 | Task 2 — Connection string diff (query-param granularity) | Tests placeholder granularity for URL query strings. Needs the wider detector from step 13 to avoid missing exotic URL formats. | Whether we redact query-string tokens or preserve them structurally. Forces an explicit policy. | 13 | not started |
| 15 | Task 9 — URL mismatch detection | Similar domain to task 2, piggy-backs on the same detector work. | Whether non-sensitive URLs survive redaction cleanly. | 13 | not started |
| 16 | Docker Claude Code runner | The heavy infrastructure lift. We do it *after* the deterministic tasks so the harness is already battle-tested with mocks; only the runner adapter is new. | How much of the mock-agent interface transfers to a real agent and what drifts. | 6 | not started |
| 17 | Task 6 — Prompt injection exfiltration (with positive control) | THE threat-model test. Requires the Docker runner and a model with a reproducible baseline leak. The positive control is non-negotiable. | Whether shhh actually defends against its primary stated threat. Result determines whether Phase 1 launch is viable. | 16 | not started |
| 18 | Task 4 — Write round-trip (silent data loss test) | Tests the Edit/Write refusal scaffolding. In Phase 0 this lives as a library-level check because there is no hook yet; the full test comes in Phase 3. | Whether the refuse-on-sensitive model has gaps we haven't thought of. | 16 | not started |
| 19 | Task 10 — Tool-use round-trip with rehydration | Validates the complete cycle (redact → rehydrate → execute → re-redact). Needs a real agent with tool use and a rehydration hook. | Whether rehydration composes with the real Claude Code tool_use format. | 16 | not started |
| 20 | Daemon + Unix socket stub | Preparation for Phase 3's shared session map between hook daemon and proxy daemon. Building it at the end of Phase 0 means the session map is already tested under load from the eval harness. | The socket protocol shape and whether it adds meaningful latency. | 6–19 | not started |
| 21 | Baseline results document | The Phase 1 launch artifact. Aggregates all eval results, metrics, known limitations, and the threat-model narrative. Writing it at the end means every number has been earned. | How the public-facing story lines up with the raw numbers. | 7–19 | not started |
| 22 | Split `shhh-eval` into standalone repo | Phase 1 prep. The eval harness lives inside the main shhh module during Phase 0 for convenience; before Phase 1 launch it moves to `musubi-sasu/shhh-eval` with a stable import path. | How the product-agnostic interface survives the split. | 21 | not started |

## Guiding principles

- **No speculative work.** A step ships only when the previous steps have
  revealed that it is needed, and the need is documented in the log.
- **Eval drives product.** When a task fails, the failure mode is the
  spec for the next fix.
- **Every step ends at a green checkpoint.** `make test` and `make bench`
  both pass before committing.
- **Document what you learn, not just what you did.** The log's `Learned`
  and `Decisions forced` fields are more valuable than the `Built` field
  because they are what future-you cannot reconstruct from git history.
