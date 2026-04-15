# docs/mvp — feature specs beyond milestone 1

This folder contains the specs for the features that follow the milestone 1
MVP (the Claude Code hook). It exists because the project has shifted from
a linear "phased" plan — which is what the prior 22-step roadmap turned
into, and which the postmortem diagnoses — to a **silo'd feature model**:

> Each feature is independently shippable. Each feature has its own
> forcing-function demo. Features can land in any order after the anchor
> (NPX installer), and the project ships value at each feature boundary.

This is intentionally *not* the same thing as a phased roadmap. Key
differences:

- A phased roadmap says *"you must complete step N before step N+1 is
  meaningful."* A feature silo says *"this works on its own; ship it."*
- A phased roadmap accumulates scaffolding because each step assumes a
  larger structure. A feature silo forces each spec to carry its own
  forcing function and its own exit criteria.
- A phased roadmap drifts because nothing in it is individually
  demo-able until the whole is done. A feature silo is a sequence of
  demos.

## Reading order

One feature is the **anchor**: everything else depends on it existing.

1. [`feature-npx-installer.md`](./feature-npx-installer.md) — **anchor.**
   Without distribution, no user can install shhh, which means no other
   feature has any users. The NPX installer also owns the *configuration
   surface* for the whole product: install scope (global / per-project),
   obfuscation level, which agents to hook. Every other feature's user
   configuration lands here.

After the anchor ships, the remaining features are **independent**:

- [`feature-home-audit.md`](./feature-home-audit.md) — `shhh scan --home`.
  Cross-machine secret audit. Reuses the existing `internal/scanner`
  walker over all project roots under `$HOME`. Doesn't touch any agent
  config, purely reports. The "scan is the marketing" feature from
  PRD §1 principle 4.
- [`feature-codex-hook.md`](./feature-codex-hook.md) — `shhh install
  codex`. Second coding agent supported. Reuses the cmdhook dispatcher
  we built for Claude Code.
- [`feature-cursor-hook.md`](./feature-cursor-hook.md) — `shhh install
  cursor`. Third coding agent, likely via MCP.

These three are not ordered relative to each other. Pick whichever the
next session has energy for.

## What's explicitly NOT in this folder

- **A linear roadmap with numbered phases.** If you find yourself
  wanting to add "MVP step 1, 2, 3" numbering, stop. Re-read the
  postmortem.
- **Implementation details for features we haven't discussed.** Each
  spec exists so it can be refined *with the user* before code is
  written. If a spec has a section full of decisions the user never
  made, it has overbuilt.
- **Docs for milestone 1.** That one shipped. Its history is
  `docs/implementation-roadmap.md` (milestone 1) and the commit
  history. Nothing in this folder retro-documents what already works.

## Refinement protocol

Each feature spec is a **draft for discussion**, not a commitment. The
sections labeled **Open questions** are the points where the user's
judgment is needed before implementation starts. The expected flow:

1. I write the spec from what I know (code that already exists, PRD
   content, and prior conversation).
2. We walk the Open questions together. User decides. I update the
   spec in place.
3. Only when a spec has zero open questions does it become a work
   item. Until then it's conversation scaffolding.

A spec with unresolved open questions is not ready to implement — the
CLAUDE.md rule holds: *"When in doubt, ask the user."*

## What every feature spec contains

```
## Forcing function
  The one-sentence demo that proves this feature works.
  If the scenario doesn't run end-to-end on a real machine,
  the feature has not shipped.

## What already exists in the codebase
  Library pieces this feature reuses. Links to packages.
  Usually most of the work — the postmortem warning about
  rebuilding things that already work applies here too.

## What's new
  The thin layer of new code this feature actually adds.
  Should be a short list, not an architecture diagram.

## Open questions
  Things the user must decide before implementation starts.
  Each question should be phrased so "yes/no" or "A/B/C"
  answers are possible.

## Out of scope for this feature
  What this feature deliberately does NOT touch, to keep
  the silo boundary honest.
```

## Honest forcing-function check for the folder itself

Does writing these docs bring the demo closer? **Only if each spec ends
with a decision we can act on.** Docs that describe features in the
abstract but never resolve into concrete implementation work are the
exact failure mode the postmortem warns about. The user and the
assistant both have to watch for this. If a spec sits unchanged for
more than one session without either being refined toward
implementation or deleted, it is over-scoping dressed up as planning.
