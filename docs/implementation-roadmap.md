# Implementation roadmap

shhh is a thin hook that drops into coding agents and redacts secrets
before they reach the LLM. This roadmap is a list of user-shippable
milestones — nothing more. No phases, no tiers, no eval framework as a
product.

**Read first:** `docs/postmortem-eval-overbuild.md`. It explains why
this document is 100 lines and not 1000, and why the prior 22-step
eval-first roadmap got scrapped.

## Forcing function

Every milestone ships when this scenario works end-to-end on a real
machine:

```
$ shhh install <agent>
$ <agent>
  > read .env
  (<agent> sees a placeholder, the real secret never leaves the
   developer's machine.)
```

If a milestone's work does not make that scenario more real, it does
not belong to the milestone.

## Milestone 1 — Claude Code hook (v0.1 MVP)

**Shippable artifact:** `shhh install claude-code` works end-to-end on
the author's laptop. A Claude Code session reading a `.env` file sees
placeholders, not raw secrets.

Work items:

- [ ] `shhh hook claude-code` subcommand. Reads the Claude Code hook
      JSON payload on stdin, runs the `PreToolUse`/`PostToolUse`
      content through the existing `internal/redactor`, writes the
      modified payload on stdout. Exit non-zero only on genuine
      errors; redaction is always best-effort-non-blocking.
- [ ] `shhh install claude-code`. Writes a hook entry into
      `~/.claude/settings.json`. Idempotent. Prints the diff it made
      and where it made it.
- [ ] `shhh uninstall claude-code`. Removes the entry cleanly.
      Leaves surrounding config untouched.
- [ ] `make demo`. Creates a fixture `.env` with a Stripe key, runs
      Claude Code against it (non-interactively if possible), asserts
      the raw key does not appear in the transcript and a placeholder
      does.
- [ ] Manual verification on the author's real Claude Code
      installation. Screenshot of the before/after for the README.

Exit criteria: the scenario above runs on the author's laptop without
any hand-holding, and uninstalling returns Claude Code to its prior
behavior.

## Milestone 2 — README and first ship

**Shippable artifact:** `v0.1.0` tag, public GitHub repo, README that
tells the story on its own.

Work items:

- [ ] README that opens with the `shhh scan` output screenshot
      (PRD principle 4: scan is the marketing).
- [ ] One-line install instruction.
- [ ] Before/after screenshot of a real Claude Code session.
- [ ] Tag `v0.1.0`. Public announcement is a separate call — the
      artifact is the tag.

## Milestone 3 — Codex hook

**Shippable artifact:** `shhh install codex` works the same way.

Work items:

- [ ] Identify Codex's extension point (plugin, SDK middleware, or
      wrapper — to be discovered by reading Codex's docs at
      implementation time, not speculated now).
- [ ] `shhh hook codex` + `shhh install codex` + uninstall.
- [ ] Manual verification against a real Codex session.

## Milestone 4 — Cursor integration

**Shippable artifact:** `shhh install cursor` works.

Work items:

- [ ] Cursor's MCP support is the likely integration point. Verify
      at implementation time.
- [ ] `shhh hook cursor` + `shhh install cursor` + uninstall.
- [ ] Manual verification.

## Milestone 5+ — Growth (post v0.2)

**Not scoped now.** Candidates, in no particular order:

- Session determinism across multiple tool calls within one agent
  session (the genuinely hard bit — probably needs a small daemon,
  but only after milestones 1–4 land and real usage reveals the
  need).
- Community rule additions, team configs, CI mode.
- MCP compensatory tools (`compare_secrets`, `decode_jwt_safely`)
  if and only if real agent usage reveals a specific case the hook
  alone can't handle.
- Telemetry on real-world detection rates.

None of these get built speculatively.

## Explicitly out of scope for v0.1

- Any form of eval framework as a product or separate repo.
- Docker runners for validation.
- A proxy daemon or Unix socket.
- "Product-agnostic Redactor interface."
- Prompt-injection threat-model tasks as automated benchmarks (they
  become manual tests once the hook ships).
- `shhh-eval` as a standalone tool.

See the postmortem for why each of these was once on the old
roadmap and is now deleted.

## What the core library already gives us (don't rebuild)

These components exist, have tests, and are ready to be the body of
milestone 1's hook subcommand. Do not redesign them; wire them up.

- `internal/rules` — 39 anchored detection rules (15 hand-written +
  24 transcribed from gitleaks v8.30.1's default config). Includes
  the known-placeholder allowlist.
- `internal/detector` — pattern + Shannon-entropy pipeline with
  charset-diversity gate and integrity-prefix skip.
- `internal/session` — salted, session-scoped placeholder map with
  fail-closed rehydration and structural-placeholder support.
- `internal/redactor` — redact/rehydrate round-trip with
  structural URL placeholders for postgres/mongo (PRD §5 example).
- `internal/scanner` — file walker with `.env` cross-reference for
  custom secrets that don't match a pattern rule.

The `internal/eval/` package also exists and its tests run under
`go test`. It is not framed as a product. See CLAUDE.md for the
exact framing allowed.

## History

Entries 1–11 of `implementation-log.md` document the prior 22-step
roadmap (Phase 0 through Phase 6 — all eval work). They produced a
working library and a coherent but product-less eval harness.
`docs/postmortem-eval-overbuild.md` has the full retrospective.
Entry 12 of the log marks the pivot to this document.
