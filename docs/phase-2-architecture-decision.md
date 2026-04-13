# Phase 2 — Architecture Decision

**Duration:** 1 week
**Visibility:** Public (decisions are published)
**Prerequisite phase:** Phase 1 shipped with baseline eval numbers live

## Goal

Translate the Phase 0 eval results into a committed architecture for Phases 3+. Small phase, high leverage: one week of analysis and writing that determines what gets built next.

## Why this phase exists

Between Phase 0 and Phase 3, there is a decision point that the original PRD silently skipped. The eval will have produced 40 cells of data. Some will be surprising. Most of those surprises invalidate assumptions the PRD currently makes. Going straight from "eval shipped" to "let's build the Claude Code hook" risks building the wrong hook for the wrong problem.

Phase 2 is explicitly the *"sit down and read your own numbers"* phase. Its deliverable is a short document with numbered architectural decisions, each citing the eval result that justifies it.

## Deliverables

### `ARCHITECTURE_DECISIONS.md` in the main repo

A numbered list of ADRs (Architecture Decision Records). Each ADR has:
- **ID and title** (`ADR-001: Commit to tool_use rehydration as core engineering work`)
- **Context** — the eval result or critique that forced the decision
- **Decision** — what we chose
- **Consequences** — what this means for Phase 3+
- **Alternatives considered** — briefly, so the reasoning is traceable

### The decisions to make

**ADR: Rehydration commit.** Does the eval show that `redact+rehydrate` meaningfully beats `redact-only` on Tier 1 tasks? Expected: yes, significantly. If so, commit to rehydration as core engineering work for Phase 3 and 4. If the gap is small, narrow the product to "read-only redaction" and document it explicitly in the README and threat model.

**ADR: Compensatory tool roadmap.** Which MCP tools actually unblock failing eval tasks, and in what order? Each compensatory tool ships only if an eval task demonstrates it is necessary. Speculative tools from the PRD (`explain_secret` if no task needed it, for example) are dropped until a task justifies them.

**ADR: Tier 1 failing tasks.** For each Tier 1 task that still fails in full mode: pick one of (a) engineering work to fix it in Phase 3 or 4, with a specific plan; (b) scope-out with documentation in the threat model; (c) accept as a known limitation in the compatibility matrix. The decision must be explicit — a failing Tier 1 task cannot be left unaddressed.

**ADR: Primary integration choice.** Phase 3 ships one integration first. The default is Claude Code (largest hook API, strongest story), but if the eval reveals that Cursor or Aider integration would better validate something, the choice is revisited. The decision cites the eval result.

**ADR: Threat-model pivot risk.** Revisit PRD §11 "Anthropic/OpenAI build native protection" against the eval numbers. Does shhh's multi-tool value hold if one vendor ships native redaction? Does the session-map-shared-across-tools property show up as an eval advantage, or is it speculative? The pivot plan needs a real answer, not "users win anyway."

**ADR: False-positive calibration.** If task 8 (public-example corpus) shows false positives above the 5% target, decide: (a) tighten the detection rules (with a Phase 3 budget) or (b) raise the default entropy threshold (with a potential false-negative tradeoff). The decision is logged with the current numbers.

**ADR: Session-map scope.** The in-memory session map is specified as shared between hook daemon, proxy daemon, and MCP server via Unix socket. Does the eval support this, or is the simpler "one daemon per integration" approach sufficient for the expected use cases? The shared-map approach has more attack surface; the per-integration approach has cross-tool consistency gaps. The decision is logged.

## Ship criterion

`ARCHITECTURE_DECISIONS.md` is published in the public `shhh` repo with at minimum the seven ADRs above, each citing a specific eval result. The document is short (one page per ADR is fine), honest about tradeoffs, and links to the eval data.

A reader of the ADRs should be able to understand *why* Phase 3 will build what it will build, without needing to read Phase 3's plan itself.

## Explicitly out of scope for Phase 2

- Writing any code. Phase 2 is reading, thinking, and writing documentation.
- Designing the detailed API of compensatory tools. That's Phase 4.
- Settling implementation details like "which Go library for X." That's Phase 3.
- Revisiting the eval task list. New tasks come in Phase 0 amendments, not Phase 2.
- Marketing or community engagement beyond publishing the ADRs.

## Phase-specific risks

| Risk | Severity | Response |
|------|----------|----------|
| Analysis paralysis — the phase extends to 3 weeks | High | Hard time-box to 1 week. At end of week, ship whatever ADRs are ready and move on. Unresolved decisions are logged as "ADR deferred, pending Phase N." |
| An eval result is so bad it invalidates the entire product | Low but existential | Accept it and pivot publicly. Credibility is the asset; hiding bad news destroys it. |
| The decisions feel anticlimactic after Phase 1's launch | Medium | That's fine. Phase 2 is infrastructure for the team (even if the team is one person). It's okay for it to be quiet. |

## Dependencies

- Phase 1 shipped.
- `shhh-eval` baseline published and stable.
- At least two weeks have passed since launch so some community feedback has surfaced (bug reports, unexpected use cases). This feedback is input to the ADRs.
