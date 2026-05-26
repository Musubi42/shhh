# CLAUDE.md — shhh operating instructions

This file is loaded into every Claude Code session that runs in this
repo. Read it before touching anything. It is short on purpose.

## What shhh is

A thin hook that drops into coding agents (Claude Code, Codex, Cursor)
and redacts secrets before they reach the LLM, replacing them with
typed placeholders so the agent can keep reasoning. PRD §1 is the
source of truth for the product. Everything else in this file flows
from that.

## The forcing function

Progress on shhh is defined by one scenario, and only that scenario:

```
$ shhh install claude-code
$ claude
claude> read .env
  (Claude sees [STRIPE_LIVE_KEY:sk_live_...], not the raw key.
   The raw key never left the developer's machine.)
```

Every session must end with a one-sentence honest answer to:
**"is that scenario closer to working today than it was yesterday?"**

If the honest answer is *"no, but we tightened the detector / added a
rule / improved a test / landed a roadmap step,"* today's work was
scaffolding and the next session needs to correct course. This is not
a soft guideline. A prior 10-commit streak failed this check and
produced a working library with no product. A human had to intervene.
See `docs/postmortem-eval-overbuild.md`.

## Pre-release status

shhh has zero external users. Raphaël is the sole developer and
sole user. There is no installed base to migrate, no API contract
to honour, no deprecation cycle to manage.

Practical consequences:
- Breaking changes are not just allowed, they're often the right
  call. Renaming things, removing options, flipping defaults — do
  it cleanly in one commit, not behind a compat shim.
- Do NOT write code paths "for users who might have the old
  setup." There are none.
- Do NOT add deprecation warnings, migration helpers, or
  schema-version negotiation for our own config. When the design
  changes, the format changes with it.
- This is NOT permission to skip testing. Real-shell validation
  (see `docs/testing-playbook.md`) still applies. "Pre-release"
  means we can change the design, not that we can ship broken
  binaries.

## Hard rules

1. **Eval is a library test, not a product.** `go test ./...` is the
   validation command. If a question cannot be answered by a Go
   test, answer it by running the hook against a real coding agent —
   not by building more test infrastructure. Do not reintroduce a
   "product-agnostic benchmark harness," a four-mode matrix, a
   tier system, or a task runner as a first-class concept.

2. **No new phases, tiers, or multi-step internal roadmaps.** The
   roadmap lives in `docs/implementation-roadmap.md` as a short list
   of user-shippable milestones. Do not extend it without a
   corresponding user-visible deliverable. "Foundations" are not
   deliverables.

3. **Before writing code, check the target artifact exists.** The
   product is a hook binary wired into `~/.claude/settings.json`
   (and equivalents for Codex, Cursor). If that does not exist yet,
   building it is the next thing. Not more rules, not more tests,
   not more docs.

4. **Beware elaboration bias.** When the next locally-obvious action
   is "extend the scaffolding," stop and ask: *does this bring the
   demo closer?* If you cannot answer yes in one sentence, the
   action is wrong even if it compiles, tests pass, and it looks
   like progress. Prior sessions failed this check repeatedly.

5. **Docker is not in scope.** Neither is a proxy daemon, a Unix
   socket, an MCP compensatory-tool server as a first-class surface,
   a response-caching LLM harness, or any form of "remote runner."
   These may become real features later, justified by observed
   real-agent need. Until then they do not exist.

   What this rule bans: anything that **runs in the background
   without the user having just asked for it**. What this rule
   does NOT ban: short-lived, on-demand subcommands the user
   explicitly invokes (e.g. a future `shhh serve` that opens a
   local viewer on `localhost`, serves the redaction log, and dies
   when the user closes it). The distinction is "background
   process the user forgot about" vs "subcommand the user just
   typed." The first is forbidden; the second is fine when it
   serves the forcing-function scenario.

6. **No speculative work from PRD claims.** The PRD describes a
   finished product. Implement only what the current milestone
   requires, observed against real agents. If the PRD describes
   something the current milestone does not need, it is not
   something to build.

## Reading order for a fresh session

1. `docs/postmortem-eval-overbuild.md` — why the prior roadmap got
   scrapped. Skip this once and you will repeat it.
2. `docs/testing-playbook.md` — how to test changes end-to-end
   without losing 30 minutes to stale binaries, aliased `cp`, or
   buffered pipes. Read once before your first `make build`.
3. `docs/implementation-roadmap.md` — the current milestone list.
4. `PRD.md` §§1, 2, 5, 6, 8 — the product vision. Skip §10 (phases)
   and §11 (open questions) unless doing growth work; both are
   historical.
5. `internal/detector`, `internal/session`, `internal/redactor`,
   `internal/rules`, `internal/scanner` — the core library. It
   works. It does not need redesign. It is ready to be the body of
   a hook.
6. `cmd/shhh/` — the CLI. Currently has `scan` and `redact`
   subcommands. It needs `install`, `uninstall`, and `hook`
   subcommands to become the product.

## What to treat as history, not as direction

- `docs/implementation-log.md` entries 1–11: they document ~1100
  lines of eval-harness work. They contain real calibration lessons
  (tokenization, charset-diversity gate, integrity-prefix skip,
  structural URL redaction, the gitleaks transcription decision)
  and those lessons transfer to the library. **Do not use them to
  drive forward work.** Use them only to avoid past mistakes.
- `internal/eval/` files: they compile, they test, they stay. Do not
  frame them as a "benchmark suite," a "harness," or a "tier
  system." They are Go tests for the redactor. That is the whole
  framing allowed.
- `cmd/shhh-eval/`: keeps building for now. Re-decide after
  milestone 1. If it has no user, it goes.

## GitHub push protection — a known repo specificity

shhh is a secret-redaction tool: its fixtures under `testdata/`,
`eval-corpus/` and `demo/leaktest/` deliberately contain
secret-shaped values. They are all **fake**. This trips GitHub
push protection, and there is one non-obvious fact to remember:

- The repo-level toggle (`secret_scanning_push_protection`) is
  **ineffective** on this public repo — it can read `disabled` and
  the push is still blocked. Do not waste time toggling it.
- The only thing that works is **per-secret allowlisting** via the
  `unblock-secret/...` URLs printed in the `git push` error. The
  repo owner opens each, marks it "used in tests", and allows it.
  This is a one-time action per new fixture secret.
- `.github/secret_scanning.yml` (`paths-ignore`) only silences
  scanning *alerts*; it does not affect push protection.
- A history rewrite cannot fix this: fixtures must keep real
  provider prefixes (`sk_live_`, `ghp_`, …) so shhh emits typed
  placeholders — the same prefixes GitHub detects. The two
  patterns overlap by design.

When adding a new fixture secret, expect to allowlist it once.

## Maintenance reminder — gitleaks LICENSE

`cmd/shhh/cmdlicenses/gitleaks-LICENSE.txt` is embedded in the
binary and shown by `shhh licenses`. It must always match the
gitleaks version pinned in `go.mod`. When you bump that version,
run `make update-gitleaks-license` and commit the refreshed file
in the same commit as the go.mod bump. Skipping this ships a
stale MIT notice and silently misattributes upstream changes.

## When in doubt

If a decision is not obviously pulling toward the forcing-function
scenario above, stop and ask the user. The user's frustration is a
better signal than ten green test runs.
