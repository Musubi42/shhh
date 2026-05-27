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

- [x] `shhh hook claude-code` subcommand. Reads the Claude Code hook
      JSON payload on stdin, runs the `PreToolUse`/`PostToolUse`
      content through the existing `internal/redactor`, writes the
      modified payload on stdout. Exit non-zero only on genuine
      errors; redaction is always best-effort-non-blocking.
      (`cmd/shhh/cmdhook/`)
- [x] `shhh install claude-code`. Writes a hook entry into
      `~/.claude/settings.json`. Idempotent. Prints the diff it made
      and where it made it. (`cmd/shhh/cmdinstall/`)
- [x] `shhh uninstall claude-code`. Removes the entry cleanly.
      Leaves surrounding config untouched.
      (`cmdinstall.RunUninstall`)
- [x] `make demo`. Creates a fixture `.env` with a Stripe key, fires
      a synthetic PreToolUse/Read at the hook, asserts the raw key
      is replaced with a `[STRIPE_LIVE_KEY:...]` placeholder.
      (`scripts/demo.sh`)
- [x] Manual verification on the author's real Claude Code
      installation. Hook is live in `~/.claude/settings.json` and
      observed redacting real tool output in sessions.

Status: **shipped.** Screenshot of before/after for the README is
the only loose end — folded into M2's README work.

## Milestone 2 — Distribution one-liners (v0.1 ship)

**Shippable artifact:** `v0.1.0` tag + a public README where any
developer, on any common OS, can install shhh with one copy-paste
command. Modeled on the Claude Code install page (tabs per
package manager, one-liner per OS).

Work items:

- [ ] `goreleaser` config producing static binaries for
      `linux/{amd64,arm64}`, `darwin/{amd64,arm64}`,
      `windows/{amd64,arm64}` on every git tag.
- [ ] GitHub releases wired (binaries + checksums + SBOM).
- [ ] `install.sh` (curl-pipe) for macOS/Linux/WSL:
      `curl -fsSL https://shhh.sh/install.sh | bash` (or the
      equivalent raw-GitHub URL until a domain exists).
- [ ] `install.ps1` for Windows PowerShell.
- [ ] Homebrew tap `musubi42/shhh` (own tap first; submit to
      `homebrew-core` only once usage justifies it).
- [ ] README opens with the `shhh scan` screenshot (PRD principle 4),
      then a "Step 1: Install shhh" section with **Native install
      (recommended) / Homebrew / `go install`** tabs and per-OS
      one-liners. Before/after Claude Code screenshot follows.
- [ ] Tag `v0.1.0`. Public announcement is a separate call — the
      artifact is the tag and the install page.

Explicitly deferred to growth (not in v0.1): npm wrapper, native
Claude Code plugin on the marketplace, WinGet package. Each ships
only when a real user asks for it.

## Milestone 3 — Proof of redaction

**Shippable artifact:** a developer who installs shhh can answer the
question "what did shhh just redact?" without reading source code or
parsing raw logs. Confidence in the tool requires evidence.

Work items:

- [ ] `shhh log` writes one JSON line per redaction to
      `~/.shhh/log.jsonl`. Fields: timestamp, agent, rule id,
      placeholder emitted, sha256 of the original secret (never the
      secret itself), source path if known. The hook writes this
      synchronously from its existing process — no daemon.
- [ ] `shhh log --last N` / `shhh log --since <duration>` pretty-print
      the tail of the file.
- [ ] `shhh baseline init` snapshots the current per-rule redaction
      counts (or known placeholders) into `.shhh-baseline.json` at
      the repo root.
- [ ] `shhh baseline diff` exits non-zero if new redaction types
      appear vs the baseline — drop-in for pre-commit / CI.
- [ ] README section "Proof it works" with a 5-line example output.

This milestone lands **before** Codex and Cursor: trust on one host
matters more than coverage across three.

## Milestone 4 — Codex hook

**Shippable artifact:** `shhh install codex` works the same way.

Work items:

- [ ] Identify Codex's extension point (plugin, SDK middleware, or
      wrapper — to be discovered by reading Codex's docs at
      implementation time, not speculated now).
- [ ] `shhh hook codex` + `shhh install codex` + uninstall.
- [ ] Manual verification against a real Codex session.

## Milestone 5 — Cursor integration

**Shippable artifact:** `shhh install cursor` works.

Work items:

- [ ] Cursor's MCP support is the likely integration point. Verify
      at implementation time.
- [ ] `shhh hook cursor` + `shhh install cursor` + uninstall.
- [ ] Manual verification.

## Milestone 6+ — Growth (post v0.2)

**Not scoped now.** Candidates, in no particular order, each gated
on a real user asking for it:

- `shhh verify <placeholder>` — pings the secret's provider to
  confirm the redacted key is live (trufflehog-style). Powerful
  trust signal, but sends the *real* secret to the provider — must
  always be an explicit user action, never automatic. **Validate
  demand with the community before building.**
- `shhh serve` — short-lived local viewer on `localhost` showing
  the redaction log graphically. Permitted under CLAUDE.md hard
  rule 5 because it is user-invoked and dies on close. Validate
  demand first.
- npm wrapper that downloads the binary at `postinstall` — to win
  JS monorepos (lefthook hedge).
- Native Claude Code plugin published on the official marketplace
  (shells out to the same `shhh` binary).
- Session determinism across multiple tool calls within one agent
  session (the genuinely hard bit — probably needs a small daemon,
  but only after milestones 1–5 land and real usage reveals the
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
