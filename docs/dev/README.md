# shhh — Development Phase Documentation

This directory contains the detailed phase plans referenced by the top-level `PRD.md` (§10). Each phase file is the contract for that phase: goal, why it exists, deliverables, ship criterion, explicit out-of-scope, and phase-specific risks. The PRD is the product spec; these docs are the implementation plan.

## The phases

| # | Phase | Duration | Status |
|---|-------|----------|--------|
| 0 | [Eval Suite & Minimal Redactor](./phase-0-eval-suite.md) | 4 weeks (private) | Not started |
| 1 | [Public Launch: Scan + Eval](./phase-1-scan-launch.md) | 3 weeks | Blocked on Phase 0 |
| 2 | [Architecture Decision](./phase-2-architecture-decision.md) | 1 week | Blocked on Phase 1 |
| 3 | [Claude Code Integration](./phase-3-claude-code.md) | 4 weeks | Blocked on Phase 2 |
| 4 | [Multi-tool Coverage](./phase-4-multi-tool.md) | 4 weeks | Blocked on Phase 3 |
| 5 | [TUI Installer & Polish](./phase-5-tui-polish.md) | 3 weeks | Blocked on Phase 4 |
| 6 | [Growth & Ecosystem](./phase-6-growth.md) | Ongoing | Blocked on Phase 5 |

**Total realistic timeline to Phase 5 ship: ~19 weeks of solo developer time**, not the "6 weeks" the original PRD draft implied. The original compression was fantasy; this is the honest plan.

## How to read these docs

Each phase document is structured identically:

1. **Goal** — one sentence.
2. **Why this phase exists** — the reasoning that led to scoping the phase this way, including what was cut and why.
3. **Deliverables** — checkbox list, grouped by subsystem. Every item must ship for the phase to be considered complete.
4. **Ship criterion** — the single falsifiable test that determines whether the phase is done.
5. **Explicitly out of scope** — the anti-deliverables. Work that would look like it belongs in the phase but must be deferred, with reasons.
6. **Phase-specific risks** — risks that only apply during this phase (general product risks live in PRD §11).
7. **Dependencies** — what must be true from earlier phases before this one can start.

## Governing principles (applies to every phase)

- **Eval-driven.** No architectural decision ships without a corresponding eval result. Speculative features are cut.
- **Config-injected.** Every integration drops config into standard locations. shhh never launches, wraps, or replaces another tool's binary. Uninstalling shhh leaves the agent running identically, minus protection.
- **Fail closed on safety, fail open on feature.** Protection failures refuse; feature failures degrade gracefully.
- **Public honesty over marketing polish.** Bad eval results are published alongside good ones. The credibility compounds; polish alone does not.
