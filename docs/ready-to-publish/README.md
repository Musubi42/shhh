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

## Order

| # | Brief | Why now |
|---|---|---|
| 01 | [`01-kill-read-edit-ledger-bug.md`](01-kill-read-edit-ledger-bug.md) | Last visible friction during real sessions. Either kill it or document it loudly — but don't ship a Reddit post with this as a 5-min repro. |
| 02 | [`02-cut-release-and-homebrew.md`](02-cut-release-and-homebrew.md) | The engine refactor has 5 commits past `v0.2.0`. Tag, release, optionally publish a Homebrew tap. Bullet-proof `curl \| sh` before anyone clicks it. |
| 03 | [`03-viral-readme.md`](03-viral-readme.md) | First impression on GitHub. Visitor needs to grasp the value in 5 seconds and install in 30. Today's README is informative but lacks the punch. |
| 04 | [`04-codex-support.md`](04-codex-support.md) | OpenAI Codex CLI. The hook protocol may or may not exist; the brief asks the right question before any code lands. |
| 05 | [`05-cursor-support.md`](05-cursor-support.md) | Cursor (IDE). Likely needs a different integration shape (no native PreToolUse equivalent at time of writing). |
| 06 | [`06-launch-post.md`](06-launch-post.md) | Reddit + Twitter + Hacker News. Drafts, screenshots checklist, A/B angles. Run this last — when 01–03 are done it lights up. |

## Not in scope here

These belong elsewhere or have been done:

- Engine architecture redesign — shipped, see [`../engine-architecture.md`](../engine-architecture.md).
- `.shhhtrust` (hook bypass) — separate design pass, not a launch
  blocker. The detector skip-list half already ships.
- Smarter session cache scoping, narration compression — `ROADMAP.md`
  items 4 and 5. Polish, not virality.
