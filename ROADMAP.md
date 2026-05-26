# ROADMAP

Working list of the next things to fix on shhh, ordered by how much
each one moves the forcing-function scenario in `CLAUDE.md` forward.
Derived from the 2026-04-15 dogfooding session where shhh intercepted
6 Reads on its own source tree — 0 real secrets, 5 false positives
on docs/tests/fixtures — and cascaded into 2 Read→Edit workarounds
that cost ~30% of the session.

Items 1 and 2 are the only ones that matter until they're done.
Everything below them is contingent on the friction surface shrinking
enough that a user on a monorepo is willing to keep the hook installed
for a full day.

**Status update (2026-05-25):** the Read→Edit cascade described in
item 1 has been substantially reduced by subsequent fixes — the
"every retry re-fails" cycle no longer reproduces consistently.
Residual rough edges remain (see notes below the prompt) but the
project is no longer blocked on this from a distribution standpoint.
Item 2 (detection FPs) is still open and is the next quality lever.

---

## Recent progress — `shhh bench` + gitleaks Step 1 (2026-05-26)

Detection-engine work for ROADMAP item #2 landed as two
interlocking pieces in one session: a pluggable backend layer
(so gitleaks can be opted into) and a measurement command (so
the migration is data-driven, not vibes-driven).

### Pluggable detector backends (`SHHH_DETECTOR` env flag)

Spike (`docs/gitleaks-spike.md`) confirmed gitleaks-as-library
is viable: 222 rules vs our ~30, MIT, clean Go API, +30ms perf,
+8 MB binary. Step 1 of the 4-step migration shipped in the
same session:

- `internal/detector/backend.go` — `Backend` interface (one
  method, `Detect([]byte) []Finding`). `*Detector` satisfies it
  via compile-time assertion.
- `internal/detector/gitleaks.go` — `GitleaksBackend` wraps
  gitleaks v8.30.1's `detect.Detector`. Label mapping for ~20
  known rule IDs → our placeholder vocabulary; heuristic
  fallback for the long tail.
- `internal/detector/factory.go` — `NewFromEnv()` reads
  `SHHH_DETECTOR=shhh-native|gitleaks`. the multi-engine wrapper runs in parallel,
  returns legacy findings (safety), and writes one-line diff
  events to stderr per unique content (`shhh detector-diff:
  legacy=N gitleaks=M only-shhh-native=[…] only-gitleaks=[…]`).
- `NewFromConfig(cfg)` (Phase 2) exposed so
  callers like bench can silence the production diff stream by
  passing `io.Discard`.
- `redactor.New` and `scanner.New` accept the `Backend`
  interface. Env-aware pass type-asserts an optional
  `envValueChecker` so gitleaks degrades gracefully when its
  vocabulary lacks the legacy "looser env threshold" feature.
- Hook (`cmd/shhh/cmdhook/sessionstore.go`) and `shhh scan`
  honour `SHHH_DETECTOR`. Audit / redact / eval stay legacy by
  construction for reproducibility.

### `shhh bench` — the calibration tool

`shhh bench <path>...` runs every selected backend over the
same corpus and emits both a terminal verdict and an HTML
dashboard. Built so the user can answer "should I flip the
default to gitleaks yet?" from observed data instead of
guesswork.

CLI:

```
shhh bench <path>...                          # all 3 engines
shhh bench --engines=shhh-native,gitleaks <path>   # subset
shhh bench --no-serve --no-html <path>        # CI / scripting
shhh bench --open <path>                      # auto-launch viewer
```

Per bench run, the output directory `~/.shhh/bench/<timestamp>/`
contains:

- `data.json` — full report (engines, agreement matrix, label
  hierarchy, per-finding rows). Single source of truth; `jq`
  consumes it without HTML scraping.
- `index.html` — thin shell (~23 KB) that fetches `data.json`
  on load and renders client-side. ~300 lines vanilla JS, no
  build step, no framework. Same hierarchy on screen as in the
  JSON.

The dashboard hierarchy: **label → file → finding**. Sorted by
count desc at each level, so the noisiest contributors surface
first. Per-finding rows show line number, redacted snippet
(source line with the match replaced inline by `[LABEL:prefix…:hash]`),
and on/off engine pips (`L` filled when legacy fired, `L̶`
outlined faint when it didn't — same for gitleaks). Sticky
filter bar: engine chips, label search, file path search.
Common-prefix scan root computed so file paths display short
(`go.sum`, not `~/long/full/path/go.sum`).

Privacy invariant: raw match values never leave the backend.
`makePlaceholder()` builds `[LABEL:prefix…:hash]` from SHA1[:8]
of the value; the snippet's prefix gets scrubbed for any other
finding's value in the same file (covers go.sum-style lines
with two hashes). Locked by `TestMakePlaceholderRedacts` and
`TestLineComputerSnippetScrubsOtherValues`.

### Observed diagnostic on the shhh repo (the whole point)

Running `shhh bench . demo/leaktest` against this repo (49
files, 413 KB) produces the migration verdict in seconds:

| | legacy | gitleaks | both |
|---|---:|---:|---:|
| Findings | 607 | 78 | 561 |
| Time | 715 ms | 832 ms | 1.34 s |

Agreement: 57 shared · 550 only-shhh-native · 18 only-gitleaks.

Drill-down on the largest label tells the action item:
**HIGH_ENTROPY × 449, of which 442 (98.4%) live in `go.sum`.**
Tuning the shhh-native entropy gate to skip lockfiles would
eliminate ~70% of shhh-native's false-positive volume in one move.
Gitleaks fills the long tail: 18 `GENERIC_API_KEY` hits legacy
doesn't recognise. None of this required reading the code — it
came from the JSON. That's the win.

### What's still pending on the detection-engine migration

Step 1 unlocked Step 2: actually run running both engines (Phase 6 reintroduces a `union` pseudo-engine for bench) for a
week of real work, watch the diff stream, decide between
extending the label-mapping table vs keeping legacy as a
pre-filter. Bench is the offline analogue of that signal —
runnable on demand without waiting for organic usage.

---

## Recent progress — dryrun follow-up (2026-05-26)

Self-dogfood dryrun while still in the build session. Logged in
`docs/dryrun-2026-05-26.md`. Two CRITICAL bugs caught that all unit
tests had missed:

- **CLI flag-after-positional silently dropped.** `shhh audit .
  --no-serve` ignored `--no-serve` because Go's `flag.Parse` stops at
  the first non-flag argument. Identical bug in `shhh install
  claude-code /tmp/x --scope global` — silently turned the flag tokens
  into stray project paths and created `./--scope/.claude/` and
  `./global/.claude/` directories. Fixed in both commands via
  pre-splitting helpers (`splitFlagsAndPositionals` in cmdaudit,
  value-aware `splitArgsByFlags` in cmdinstall). Regression tests:
  `TestSplitFlagsAndPositionals`, `TestSplitArgsByFlags`.
- Hand-off to maintainer for the redaction-path verification (F4 in
  the dryrun doc): the dryrun agent runs *inside* a Claude Code
  session and can't reload its own hook config, so the
  install→read→placeholder loop has to be verified from a fresh
  `claude` session by the maintainer.

**Subsequent fixes (same day):**
- **F2** install path validation: `validateProjectPath` refuses
  paths starting with `-` (flag typos), ending in `.claude`
  (double-nesting), or containing `..`. Soft warning when the dir
  lacks `.git`/`package.json`/`go.mod`/`pyproject.toml`/`Cargo.toml`/etc.
  — non-blocking; catches "wrong directory" mistakes without
  refusing fresh-init repos.
- **F5** audit scope wording: announcement now reads
  `🛡️ shhh audit — scope ~/work/billing, scanning N projects (~M sessions)`
  so the count is visibly scope-restricted, not the total inventory.
- **F3+F4** new `shhh doctor` subcommand: health check that
  inspects the running binary vs $PATH, validates
  `~/.shhh/config.json`, walks `installed_paths` and flags entries
  whose referenced `settings.json` no longer contains a shhh hook
  (the F3 desync bug). `shhh doctor --fix` drops the stale entries
  from the config. Friendly tree-style output with ✓/⚠/✗ markers,
  ANSI auto-detect via `NO_COLOR` + isatty. Reminds users to
  restart Claude sessions after install/uninstall (F4 hand-off,
  surfaced as a check line rather than a separate command).

**F6 (done):** `enumerateClaudeProjects` now takes the scope paths
and skips per-project IO (transcript read + jsonl count) for
dash-names that can't possibly decode to a path in scope. The
correctness predicate is `DashNameCouldMatchScope` — dash and slash
are treated as the same separator class to handle the
hyphen/slash ambiguity (`/Users/me/open-source/shhh` and
`/Users/me/open/source/shhh` are both candidates for dash-name
`-Users-me-open-source-shhh`). False positives are caught by the
existing post-enumeration scope filter; false negatives would be
bugs and are covered by `TestDashNameCouldMatchScope` (9 cases).

**Measured impact:** on a 23-project Claude history, `shhh audit .`
(scope=1 project) was 17.8s before F6, 17.8s after — enumeration
is dominated by per-project session scanning, so the savings are
~50-200ms invisible to humans. The real perf story is
**`shhh audit` (no scope, 6:57s) vs `shhh audit .` (scope, 17.8s)**
— a 23× speed-up that's been in place since the earlier
`ScopePaths` work. The testing-playbook reminds new agents to run
the scoped form by default.

**Doc landed:** `docs/testing-playbook.md` (referenced from
`CLAUDE.md` reading order at step 2). Captures the dryrun's
operational lessons — stale binary detection, aliased `cp`/`rm`,
pipe-buffering with `head`, hook activation requiring session
restart, side-effect cleanup recipes, the "go test is necessary
not sufficient" rule, and a default test order for changes
touching the CLI surface.

---

## Recent progress — per-project install + diff renderer (2026-05-26)

Follow-up on the 2026-05-25 per-project MVP. All shipped, all on
`main`, `make test` green. See `docs/per-project-install-kickoff.md`
for the original design context.

**CLI shape — positional paths**
- `shhh install claude-code [paths...]` accepts positional project
  paths. `--scope project` is inferred when any path is given;
  `--scope global` is the default. Multi-path is supported in one
  invocation. `--cwd` kept as a back-compat alias. Passing
  `--scope global` together with a positional path is now an
  explicit error (intent ambiguous).
- Paths are normalized to absolute form via `filepath.Abs` so the
  install target is reproducible no matter where `shhh` is invoked.
- The HTML "Install shhh" CTAs no longer require `cd` — they emit
  the positional form (`shhh install claude-code <path>`) with
  `shellQuote()` for paths containing spaces.

**Interactive picker**
- New "Where to install shhh?" step between agent-select and audit
  scope. Project scope opens a `huh.MultiSelect` over the user's
  Claude history (`~/.claude/projects/`, OnDisk only), so the user
  picks from a concrete list rather than typing a cwd.
- Multi-select means one `shhh install` run can hook N repos.
- A `✍ Type a custom path...` sentinel at the top of the list
  triggers a free-form `huh.NewInput` loop for repos Claude has
  never been opened in (fresh clones, brand-new projects). Tilde
  expansion + dir-exists validation per entry.

**Path-decoding bug fix**
- `ListClaudeProjects` was using the naive `DecodeDashPath`, which
  corrupted any path containing literal hyphens (`open-source/shhh`
  → `open/source/shhh`). Switched to `ResolveProjectPath`, which
  prefers the loss-less `cwd` field stored in transcript JSONLs.
  Same fix as `internal/audit/run.go` got on 2026-05-25, just
  applied to the install picker's path source.

**Diff renderer**
- The before/after JSON dump on install/uninstall (60+ lines) is
  replaced by a compact semantic diff:
  ```
    + PreToolUse  matcher=Read  →  ~/.local/bin/shhh hook claude-code
    + PreToolUse  matcher=Bash  →  ~/.local/bin/shhh hook claude-code
    + SessionEnd  *             →  ~/.local/bin/shhh hook claude-code

    3 hooks added · 7 existing settings preserved
  ```
- `ensureHook` now returns `bool` so callers can collect exactly
  the entries that changed. ANSI colors auto-detect via `isatty` +
  `NO_COLOR` (no-color.org convention). Sub-2-second scannability
  is the goal — users see precisely what shhh touched, nothing
  more.

**Tests**
- `TestProjectScopeInstallAuditUninstallCycle` — end-to-end:
  install per-project → audit sees `[HOOKED ✓]` → uninstall →
  audit demotes to `[NOT HOOKED]`. Also asserts sibling projects
  outside the install root stay un-protected.
- `TestPlanExecuteMultiProject` — single `Plan` with N project
  paths installs into all of them in one Execute call.
- `TestParseInstallFlags` — seven sub-cases covering positional /
  flag / `--cwd` alias / scope-inference / incompatible-combo
  errors.
- `TestRenderChangeAddNoColor`, `…RemoveSingular`,
  `…QuotedCommand` — diff renderer formatting, including the
  no-color path and quoted-path display.

**Decisions captured**
- `.claude/` is **never** removed on uninstall, even when empty.
  Claude Code may use the directory for unrelated state; an empty
  dir is harmless; partial cleanup adds complexity for no user
  benefit. Documented in `cmdinstall.go::uninstallClaudeCode` and
  the kickoff doc's "Decisions captured" section.

---

## Recent progress — `shhh audit` polish (2026-05-25)

Big session of forensic-audit work, driven by the v0.1 release
dry-run. All shipped, all on `main`, `make test` green. See
`docs/audit-api.md` for the full agent-facing reference and
`docs/release-dryrun.md` for what triggered each change.

**Bug fixes**
- `[PROTECTED ✓]` was lying when `~/.shhh/config.json::installed_paths` drifted from the actual `settings.json` state. Fixed by (a) `shhh uninstall` now updates config.json, (b) the audit defensively re-reads each referenced settings.json and only trusts it if `shhh hook` is genuinely present (`internal/audit/run.go::settingsHasShhhHook`).
- Path normalization was a naïve `strings.ReplaceAll("-", "/")` that mangled `open-source` into `open/source`. Replaced by `ResolveProjectPath` which prefers the loss-less `cwd` field from inside transcripts and falls back to dash-decode only when no transcript is readable.
- `0/23 projects` counter stuck at zero during scan — `ProgressProjectFinished` only fired in the post-scan loop. Added `OnProjectDone` callback on `TranscriptSource`, emits `ProgressProjectScanned` per project as transcripts complete; counter ticks in real time.
- Ctrl-C was captured by `signal.Notify` but no goroutine read it during the long scan, so it got swallowed and users had to kill the terminal. Watcher goroutine spawned before `auditpkg.Run` calls `cancel()` on first signal; second signal `os.Exit(130)`.
- HTML report hid projects with no findings (only 8 of 23 visible). Now renders all of them with a `<details>` foldable group for the no-finding ones.
- Live counter showed `events scanned` (per-line, opaque to users). Replaced by per-`.jsonl` `sessions scanned` matching the header's `(≈N sessions)` figure.
- `ignored_paths` filtered the audit's `projects` slice but didn't propagate to the sources, which kept reading every `.jsonl` on disk. Step 2c in `Run()` now translates the surviving project set into a `selectedProjects` allow-list for the sources.

**New features**
- Interactive picker (`huh.MultiSelect`) on `shhh audit` by default. All projects shown with session counts (including `[folder gone]`), pre-checked except those in `ignored_paths`. Unchecking persists. `--no-select` bypasses for CI; auto-bypass in non-TTY.
- Live scroll log with in-place upgrades: entries appear as `⟳ transcripts scanned`, upgrade in place to `✓ ... [HOOKED ✓] 🚨 N leaked` as the post-scan loop finalizes them. Block stays on screen after `ProgressDone` (it used to be cleared before users could read it).
- ETA in the footer once ≥30 sessions are processed: `(elapsed / sessionsDone) × sessionsRemaining`. "almost done" below 30s remaining.
- Live `🚨 N leaked` counter in the footer, ticking as the aggregator sees new `(placeholder, project)` pairs.
- `shhh audit ignore <path>` / `unignore <path>` / `ignored` subcommands as scriptable equivalents of the picker.
- HTML overview now has a top-level "⚠ N projects not hooked" install-CTA block above the project list, with a copy-paste global command and up to 3 per-project examples.

**Wording sweep**
- `PROTECTED` → `HOOKED`, `UNPROTECTED` → `NOT HOOKED`, `ARCHIVED` → `FOLDER GONE`. The old labels read as historical claims when they actually meant "right now" / "directory deleted". New labels are honest.
- `Currently at risk` is conditional on status: `🔒 Will be redacted on next read` on hooked projects (proof, not alarm); `At risk on next session` on not-hooked.
- Delta deltas read `+31 newly detected` instead of `+31 new`, to avoid implying 31 new leaks happened in the delta window.
- Removed the misleading `9 days` header ("history span" was actually the earliest-detected-leak timestamp).

**CLI restructure**
- Removed per-project breakdown from the CLI summary entirely. CLI now shows header + 4-line summary + delta + rotation block + install CTA + URL. Per-project detail lives in the HTML report.

**Per-project install — MVP shipped**
- `shhh install claude-code --scope project [--cwd <path>]` and matching uninstall, plus `.claude/` dir auto-creation on missing. Config `scope` re-derived from `installed_paths` on every load.
- Pending follow-ups documented in `docs/per-project-install-kickoff.md`: interactive picker enhancement, per-project HTML mini-CTA, global-with-local-override edge case, automated test coverage.

---

## 1. Fix the Read→Edit ledger bug

**Status:** largely resolved as of 2026-05-25. Kept here for the
historical reproduction and prompt context — useful if a regression
ever shows up.

**Problem:** When the hook rewrites `updatedInput.file_path` to a cache
location, Claude Code's internal Read-ledger records the cache path,
not the original. The next Edit/Write on the original path fails with
`File has not been read yet` and there is no way to retry cleanly —
every retry re-enters the hook, which re-rewrites, which re-fails.

**Evidence:** Test 2 in
`testdata/fixtures/hook-playground/README.md` is the reproduction.
Entry 13 in `docs/implementation-log.md` documents how this cascaded
during the visibility-feature session (3 separate files could not be
edited; I had to patch them via `python3` from the Bash tool).

**Why it matters more than new features:** on a monorepo, the
"non-editable" surface grows monotonically over a session. After
~20 minutes of work, any file that ever held a detectable token is
dead to the Edit tool, regardless of whether the token was real.

**Prompt for the next session:**

> Fix the Read→Edit ledger bug in shhh's Claude Code hook.
>
> Read first:
>   1. `CLAUDE.md` — hard rules.
>   2. `testdata/fixtures/hook-playground/README.md` Test 2 — the
>      reproduction. The "RESULTS" block shows the cascade.
>   3. `docs/design/read-edit-tracking.md` — prior design notes on
>      this area; contains context on the Bash-fallback workaround
>      that was shipped instead of a real fix.
>   4. `cmd/shhh/cmdhook/read.go` — the PreToolUse handler that
>      rewrites `updatedInput.file_path`. This is where the bug is
>      introduced.
>   5. `docs/implementation-log.md` Entry 13 — describes how the
>      ledger bug cascaded during the visibility work, including the
>      python-via-Bash workaround.
>
> The fix needs to make `Edit` and `Write` work on their first try
> against the ORIGINAL file path after a redacted Read, with no
> retry loop and no user-visible error. Likely strategies to
> evaluate (not prescribe — investigate first):
>
>   - PostToolUse hook that fires a silent ledger entry for the
>     original path after Read completes
>   - Instead of rewriting `file_path`, inject the redacted content
>     via `additionalContext` and leave `file_path` pointing at the
>     original (costs more context tokens, but fixes the ledger)
>   - Symlink-based cache (original path → cache) rather than a
>     different path (may not work depending on how Claude Code's
>     ledger canonicalizes paths)
>
> Hard constraint: the fix must NOT disable redaction on files that
> fail to edit cleanly — silently letting secrets through is worse
> than a noisy workaround.
>
> Acceptance test: re-run Test 2 in hook-playground/README.md. It
> should succeed on the first Edit call, with no Bash fallback.
> Paste the new transcript under the Test 2 RESULTS section.

---

## 2. Replace the detection engine

**Status:** Step 1 of 4 shipped 2026-05-26 — gitleaks backend
lives behind `SHHH_DETECTOR=shhh-native|gitleaks`, default
unchanged. `shhh bench` is the measurement tool that drives the
remaining 3 steps. See the consolidated "Recent progress —
`shhh bench` + gitleaks Step 1" entry above.

**Remaining work:**
- Step 2 (calibration): dogfood with running both engines (Phase 6 reintroduces a `union` pseudo-engine for bench),
  watch the diff stream and `shhh bench` output, decide between
  extending the gitleaks → shhh label-mapping table vs keeping
  shhh-native as a pre-filter for what gitleaks misses
  (POSTGRES_CONNSTRING, env-aware GITHUB_PAT, etc.).
- Step 3 (flip default): switch `SHHH_DETECTOR` default from
  `shhh-native` to `gitleaks` once the diff stream is stable and the
  bench dashboard shows no regression on the canonical corpora.
- Step 4 (cleanup): delete dead bespoke rules in
  `internal/rules/` once nothing depends on them.

**Concrete first action item from the 2026-05-26 bench run:**
`HIGH_ENTROPY × 449` is dominated by `go.sum × 442` (98.4%).
Tuning the shhh-native entropy gate to skip well-known lockfile
extensions (`.sum`, `*-lock.json`, `*.lock`) before Step 3
would eliminate most of shhh-native's false-positive volume in the
"both" diff stream, making the gitleaks-only future cleaner.

---

**Original framing kept for context** (predates the Step 1
landing — historical):

**Status:** second blocker. Without this, item 1 fixes a symptom
while the root cause (over-redaction) keeps biting on new file types.

**Problem:** The current detection layer in `internal/detector/` and
`internal/rules/` is a mix of bespoke regex rules, a hand-tuned
Shannon-entropy gate (4.5 bits, 20+ char minimum, integrity-prefix
skip list), and an .env-aware pass. It over-fires on documentation
(hashes in logs, example keys in READMEs, placeholder strings in
test fixtures) and under-fires on the long tail of real secret shapes
that other projects have already catalogued.

**User's stance (2026-04-15):** the custom rules are bad; stop
evolving them in place. The right move is to depend on a vetted
upstream and put our effort on the hook surface (narration, session
map, ledger behavior) rather than on re-inventing gitleaks.

**Why the existing "transcribe gitleaks by hand" decision is stale:**
see `docs/implementation-log.md` Entry 10. That call was made when
the eval harness was framed as the product. Now the hook is the
product, and the tradeoff flips — adding a dependency is cheaper
than carrying the maintenance cost of a bespoke detector that keeps
producing friction on its own dogfood.

**Candidates to evaluate:**
- **gitleaks as a Go library**: same language, mature rule set,
  active maintenance. First pick unless something disqualifies it.
- **trufflehog** via shell-out: richer verification (live credential
  checking), heavier, not Go-native.
- **detect-secrets** (Python): known-good plugin model, but
  cross-language embedding is a tax.

**Prompt for the next session:**

> Replace shhh's detection layer with a vetted upstream engine.
>
> Read first:
>   1. `CLAUDE.md` — especially rule 4 (elaboration bias) and rule 6
>      (no speculative work from PRD claims).
>   2. `ROADMAP.md` item 2 — this entry.
>   3. `docs/implementation-log.md` Entry 10 — the "transcribe
>      gitleaks manually" decision. Understand why it was made so
>      you can understand why it's being reversed.
>   4. `internal/detector/detector.go`, `internal/rules/rules.go`,
>      `internal/redactor/redactor.go` — the current detection
>      surface. Understand the Finding struct shape because the new
>      engine's output must be adapted to it.
>   5. `PRD.md` §§5, 7 — the redact/rehydrate contract and the
>      detection pipeline spec. The engine swap must preserve these.
>
> Step 1 (research, don't code): compare gitleaks library,
>    trufflehog shell-out, and detect-secrets on:
>    - dogfood false-positive rate against the shhh repo itself
>      (run it on every tracked file, count over-redactions on
>      docs/tests/fixtures)
>    - coverage of common real-world secret types
>    - dependency weight and build impact
>    Report findings in a single markdown block. Stop before coding
>    so the user can pick.
>
> Step 2 (after approval): implement the adapter. The detector
>    package becomes a thin wrapper that maps the chosen engine's
>    output shape to `detector.Finding`. Keep the session-map and
>    placeholder layers untouched — they're not the problem.
>
> Step 3 (validation): re-run the dogfood false-positive sweep
>    with the new engine. The number must drop. Paste before/after
>    counts in the commit message.
>
> Hard constraint: the .env-aware pass
>    (`redactor.RedactEnvFile`) stays. Whatever upstream we pick
>    will not have .env context, and that's a real shhh feature
>    that can't regress.

---

## 3. Allowlist / bypass affordance for intentional fixture content

**Status:** unblocked once item 2 lands (or sooner, if item 2 slips).

**Problem:** Some files in any project contain intentional
secret-shaped content: test fixtures with `sk_live_...`, docs
showing example env vars, migration files with placeholder
connection strings. Over-redacting these is not a detection bug —
the content really looks like a secret — it's a policy bug. Shhh
needs a way for the developer to say "I know, this is fine."

**Shapes to evaluate:**
- In-file marker: `# shhh:allow` on the first line of a file
- Config file: `~/.shhh/allowlist.yaml` with glob patterns
  (`testdata/**`, `docs/**`, `**/*_test.go`)
- Auto-heuristic: if a file is under a conventional fixture/doc
  directory AND the secret is a known public example
  (AKIAIOSFODNN7EXAMPLE, etc.), skip
- Combination: config for project-wide, marker for one-offs

**Prompt for the next session:**

> Design and implement the allowlist/bypass affordance for shhh.
>
> Read first:
>   1. `CLAUDE.md` — hard rules 1 (no new phases), 2 (no tiers),
>      6 (no speculative PRD work).
>   2. `ROADMAP.md` item 3 — this entry.
>   3. `testdata/fixtures/hook-playground/` — the canonical example
>      of a directory that SHOULD be allowlisted in a real usage.
>   4. `cmd/shhh/cmdhook/read.go` — where the bypass check needs to
>      hook in (before calling `LoadRedactor`, ideally — if a path
>      is allowlisted we don't even load the session map).
>   5. `cmd/shhh/cmdhook/bash.go` — the Bash wrapper has no path
>      context at PreToolUse, so the allowlist there is harder.
>      Decide whether Bash gets a narrower allowlist shape (command
>      prefix, executable path, etc.) or nothing at all for v1.
>
> Step 1 (design, don't code): write a short proposal in
>    `docs/design/allowlist.md` covering:
>    - which shape(s) to implement and why
>    - precedence rules (in-file vs config vs auto)
>    - how the user discovers an allowlist-triggered skip (narration
>      should probably mention it in a one-liner so there's no
>      silent bypass)
>    - failure modes: what if the allowlist file is malformed,
>      unreadable, contains broken globs
>    Stop and wait for approval.
>
> Step 2 (implementation): ship the approved design as one commit.
>
> Hard constraint: default behavior without an allowlist must be
>    IDENTICAL to today. The allowlist is opt-in. A user who doesn't
>    configure one must see the same redactions as before.

---

## 4. Compress the narration; make "IMPORTANT — how to modify this file"
   conditional

**Status:** blocked on item 1. If the Read→Edit ledger bug is fixed,
the 8-line "use Bash instead" block in the narration becomes
obsolete and can be deleted. Doing this before the ledger fix is
backwards.

**Prompt for the next session:**

> After the Read→Edit ledger bug is fixed, sweep
> `cmd/shhh/cmdhook/read.go::narrateRedactions` and
> `cmd/shhh/cmdredact/cmdredact.go::buildBashNarration` for text
> that described the workaround. Anything of the form "Edit and
> Write will fail, use Bash instead" should be deleted. Keep the
> per-finding listing and the "shhh protected N secrets" opener;
> those remain useful.
>
> Read first:
>   1. `cmd/shhh/cmdhook/read.go` — the current narration function.
>   2. `cmd/shhh/cmdhook/cmdhook_test.go` — the narration tests.
>      The test that asserts "use Bash instead" appears in the
>      narration needs to flip to asserting that it DOES NOT appear.
>
> Also consider: on a long session, the narration repeats the same
> framing block N times. Investigate whether Claude Code's hook API
> exposes a "has this session already seen narration X" signal. If
> not, ship the first iteration unchanged and note in the commit
> message that session-aware narration compression is deferred.

---

## 5. Smarter session scoping for the redaction cache

**Status:** nice-to-have. Only matters once items 1 and 2 are done
and shhh is actually running on a real monorepo long enough for
the cache to matter.

**Problem:** Today, every redacted Read creates a file under
`~/.shhh/sessions/<id>/cache/`. On a long monorepo session that
touches hundreds of files, the cache directory grows linearly and
is wiped on SessionEnd. No TTL, no eviction, no dedupe across
sessions that hit the same file.

**Prompt for the next session:**

> Audit the session cache behavior in `cmd/shhh/cmdhook/sessionstore.go`
> and `cmd/shhh/cmdhook/hashname.go` (and wherever the cache path is
> computed) for a real monorepo workload.
>
> Read first:
>   1. `cmd/shhh/cmdhook/sessionstore.go`
>   2. `cmd/shhh/cmdhook/hashname.go`
>   3. `cmd/shhh/cmdhook/read.go` — where `RedactedPath` is called.
>
> Questions to answer, in order:
>   - How large does the cache get on a real session (find a
>     monorepo to test against, measure).
>   - Are there files that get redacted once and never again in a
>     session? If yes, the cache is just taking space.
>   - Is there a safe TTL or LRU eviction that doesn't break the
>     Read-ledger contract?
>
> Do NOT ship a cache change until items 1 and 2 are done — the
> friction surface is dominated by them, not by cache size.
> This entry exists mainly so the eventual audit has a home.

---

## Out of scope for this roadmap (on purpose)

- **MCP integration.** CLAUDE.md rule 5 is explicit: not in scope.
- **Docker runner / proxy daemon / remote runner.** Same.
- **Eval harness expansion.** It is a library test, not a product
  (CLAUDE.md rule 1).
- **PRD-driven features not listed above.** CLAUDE.md rule 6: only
  implement what the current milestone requires.

---

## Forcing-function check for this roadmap

Every item above passes the CLAUDE.md §"Forcing function" check
against the working demo:

> `$ shhh install claude-code; claude; claude> read .env`
> `  (Claude sees [STRIPE_LIVE_KEY:sk_live_...], not the raw key.)`

- Item 1 (ledger bug) is the difference between "the demo works for
  one Read" and "the demo works for a real session".
- Item 2 (detection engine) is the difference between "the demo
  works on the `.env` in the fixture" and "the demo doesn't also
  mangle the README that explains the demo".
- Items 3–5 only make sense in a world where 1 and 2 have shipped.

If a future session is tempted to pick up an item below the one
that's actually blocking, it has failed the check.
