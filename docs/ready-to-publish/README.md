# docs/ready-to-publish/

Briefs ordered by what unblocks "shhh has a real public moment".
Each file is a self-contained session-ready document — open it,
read it, ship it, then move to the next one. Style mirrors
`docs/engine-architecture.md`: context, decisions, concrete file
paths, acceptance check at the bottom.

**Do not** treat these as a multi-week phased plan. They are
sequenced because some unblock others (you cannot launch before
you can ship a binary, you cannot post a Reddit thread before the
README has a GIF) but each is meant to be done end-to-end in one
focused session.

Most briefs ship in one session. **Brief 06 is the exception:**
it is a brainstorm-kickoff that produces a design doc, which in
turn drives an implementation session. The pattern is
research-first, then design, then build — three sessions rather
than one. It exists because releasing v0.4.0 with three agents
wired needs a stronger ship-gate than `go test ./...` can give,
and the only way to design that gate honestly is to first sit
down and brainstorm what "validated release" means for shhh.

Current state (2026-05-27): briefs 01–05 are shipped. v0.3.0 is
public via `curl | sh`. Codex and Cursor are wired but not yet
in a tagged release — tagging waits on brief 06's outcome.

## Order

| # | Status | Brief | Why now |
|---|---|---|---|
| 01 | shipped 2026-05-26 | [`01-kill-read-edit-ledger-bug.md`](01-kill-read-edit-ledger-bug.md) | Last visible friction during real sessions. Either kill it or document it loudly — but don't ship a Reddit post with this as a 5-min repro. |
| 02 | shipped 2026-05-26 (`v0.3.0`) | [`02-cut-release-and-homebrew.md`](02-cut-release-and-homebrew.md) | The engine refactor has 5 commits past `v0.2.0`. Tag, release, optionally publish a Homebrew tap. Bullet-proof `curl \| sh` before anyone clicks it. |
| 03 | shipped 2026-05-26 | [`03-viral-readme.md`](03-viral-readme.md) | First impression on GitHub. Visitor needs to grasp the value in 5 seconds and install in 30. Today's README is informative but lacks the punch. |
| 04 | shipped 2026-05-26 | [`04-codex-support.md`](04-codex-support.md) | OpenAI Codex CLI. The hook protocol may or may not exist; the brief asks the right question before any code lands. |
| 05 | shipped 2026-05-27 | [`05-cursor-support.md`](05-cursor-support.md) | Cursor (IDE). Likely needs a different integration shape — see brief; in practice Cursor shipped native hooks in v1.7 so the pivot was MCP → hooks. |
| 06 | design pending | [`../release-validation-kickoff.md`](../release-validation-kickoff.md) | Release-validation flow + unified per-agent bench. With 3 agents wired, `go test ./...` no longer answers "is this safe to release?" Brainstorm-first (kickoff doc); design then implementation are separate sessions. Gates `v0.4.0`. |
| 07 | pending, blocked on 06 + assets | [`06-launch-post.md`](06-launch-post.md) | Reddit + Twitter + Hacker News. Drafts, screenshots checklist, A/B angles. Run this last — when 01–06 are done it lights up. (File still numbered `06-` for git history; will be renumbered or its content folded into a new `07-` once the launch lands.) |

## Not in scope here

These belong elsewhere or have been done:

- Engine architecture redesign — shipped, see [`../engine-architecture.md`](../engine-architecture.md).
- `.shhhtrust` (hook bypass) — separate design pass, not a launch
  blocker. The detector skip-list half already ships.
- Smarter session cache scoping, narration compression — `ROADMAP.md`
  items 4 and 5. Polish, not virality.
