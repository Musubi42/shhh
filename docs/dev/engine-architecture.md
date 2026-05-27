# Engine architecture (post-2026-05-26 redesign)

Design document. Captures the decisions reached during the bench
review of `~/.shhh/bench/2026-05-26T11-38-50Z/` and frames the
work that flows from them. Not a roadmap, not an implementation
log — a single source of truth for the engine shape before code
lands.

## 1. Context

- shhh's value is **redaction for LLM agents**, not detection-as-a-product.
- gitleaks is MIT, maintained by a real team, ships ~222 rules, has
  a curated global allowlist (lockfiles, vendor, binaries, fonts).
  Linking it is cheaper, more correct, and more honest than
  re-implementing it.
- shhh keeps a first-party engine for capabilities gitleaks does
  not offer by design: env cross-reference (value defined in
  `.env` found hardcoded elsewhere) and structural URL preservation
  (`postgres://user:pwd@host/db` → keep host/db visible).

## 2. Decisions

### 2.1 Naming

| Concept                       | Name in user-facing surface     | Notes |
|-------------------------------|----------------------------------|-------|
| Third-party detection engine  | `gitleaks`                       | preserves upstream name |
| shhh's first-party engine     | `shhh-native`                    | replaces the old internal name `legacy` |
| Generic term in CLI/docs      | "engine"                         | not "scanner", not "detector" |
| Internal Go interface         | `Backend` (unchanged)            | implementation detail |
| Future engines pattern        | `<vendor>` or `shhh-<flavor>`    | e.g. `trufflehog`, `shhh-secrets-v2` |

### 2.2 Two distinct ignore concepts

These MUST stay separate. Mixing them is a security hole.

**(A) `.shhhignore` — detector skip-list.** Files matching are not
scanned. Reduces false positives. Safe. Industry equivalent:
`.gitleaksignore`, `.semgrepignore`. THIS DOCUMENT scopes feature (A).

**(B) `.shhhtrust` — hook bypass [FUTURE].** Files matching pass to
the LLM unredacted. Dangerous. Requires its own design pass: loud
warnings, possible TTL, explicit logging on every use. NOT in
scope here. Mentioned only to nail the distinction.

The two features must never share a file, a flag, or a syntax. A
user who copy-pastes a Stack Overflow snippet for one must not
accidentally enable the other.

### 2.3 `.shhhignore` mechanics

**Syntax**: gitignore-style. Anchored globs, `**` for recursive
match, `!` for negation (re-include). A Go implementation of
gitignore matching is reused; we do not invent syntax.

**Precedence** (top to bottom; later layers can override via `!`):

1. gitleaks built-in allowlist — embedded via the linked
   `github.com/zricethezav/gitleaks/v8` module. Includes lockfiles
   (`go.sum`, `package-lock.json`, `pnpm-lock.yaml`, …), vendor
   directories, binaries, fonts, images, etc.
2. `~/.shhh/.shhhignore` — user-global additions.
3. `<project>/.shhhignore` — per-project additions.

**Merge policy**: additive. Each layer is concatenated; a later
`!pattern` un-ignores something an earlier layer ignored. This is
exactly how `.gitignore` composes with `~/.config/git/ignore` —
familiar to anyone who's used git.

To fully override the default for one path, the user writes
`!path` in their layer. To "wipe slate" for the project layer
they can chain `!**` and re-add their own rules, but this is
discouraged; concat-with-last-wins is the intended model.

**Defaults are not materialised**. The gitleaks allowlist is NOT
copied into `~/.shhh/.shhhignore` at install time. Two reasons:
- Upgrade safety: if we wrote 50 lines of defaults to disk, every
  gitleaks bump becomes a manual merge for the user.
- Honesty: the defaults belong to gitleaks; we should not pretend
  they're ours.

Instead, `shhh ignore list` is the canonical inspector. It prints:
- The full active rule set, resolved (defaults + user-global + project).
- For each gitleaks-sourced rule, the source: a versioned GitHub
  link to `https://github.com/gitleaks/gitleaks/blob/v<X.Y.Z>/config/gitleaks.toml`
  where `<X.Y.Z>` is the pinned version from `go.mod`. The link
  updates automatically when shhh bumps its gitleaks dependency.

### 2.4 Engine selection at install

**Interactive (`shhh install` with no positional args)**:

```
Detection engines (at least one required):

  [x] gitleaks      222 rules, maintained by github.com/gitleaks (MIT)
                    Default ignore set: lockfiles, vendor, binaries.

  [ ] shhh-native   env cross-reference + URL-structural redaction.
                    Complements gitleaks; adds capabilities it lacks.

  [space] toggle  [enter] continue
```

The selection screen ALWAYS shows. Even if the user has selected
the default before, re-selection on re-install is fast (one ENTER).

**Non-interactive**:

```
shhh install claude-code --engines gitleaks
shhh install claude-code --engines gitleaks,shhh-native
shhh install claude-code --engines shhh-native
```

CSV value, plural flag name (`--engines`). At least one engine is
required; the parser rejects an empty list with a clear error.

**Persistence**: `~/.shhh/config.json` gains an `engines` field:

```json
{
  "version": 1,
  "scope": "global",
  "agents": ["claude-code"],
  "installed_paths": [...],
  "engines": ["gitleaks"]
}
```

The hook, `shhh scan`, `shhh audit`, and `shhh bench` all read
`Config.Engines` as the source of truth.

**Debug override**: `SHHH_DETECTOR` env var continues to work. It
overrides the config for the duration of one process. Used for
A/B comparison during development and for users who want to
toggle engines without re-installing. Documented as a debug knob,
not as a primary API.

### 2.5 Multi-engine semantics

When N engines are active, all run on each content chunk. Their
findings are merged with **union by (start, end) span**:

- If two engines flag the same exact span, keep one finding
  (prefer the engine listed first in `Config.Engines`, so user
  ordering controls label-priority).
- Otherwise, keep both findings.

This favours redaction over recall completeness: when in doubt,
redact. The label-priority rule makes the user's intent visible
without dropping coverage.

### 2.6 Attribution and licensing display

`shhh install` final output (after success):

```
shhh installed. Engines active: gitleaks

  • gitleaks v8.30.1 (MIT, https://github.com/gitleaks/gitleaks)
    Default ignore rules:
    https://github.com/gitleaks/gitleaks/blob/v8.30.1/config/gitleaks.toml

  • To add the shhh-native engine: shhh install --engines gitleaks,shhh-native
  • To inspect active ignore rules:  shhh ignore list
  • Full third-party notices:        shhh licenses

Restart any running `claude` sessions for the hook to take effect.
```

`shhh licenses` is a new subcommand that prints the full text of
the gitleaks MIT license (and any future third-party). Covers
both MIT compliance (preserved notice in distribution) and user
trust (no hidden dependencies).

### 2.7 Performance

Bench (49 files, 2026-05-26):
- gitleaks: 832 ms total, ~30 ms/file
- shhh-native: 715 ms total, ~14 ms/file
- both: 1340 ms total, ~34 ms/file

For CLI usage (one-shot `shhh scan`), negligible. For hook usage
(per-tool-call), unmeasured against real Claude sessions.
Mitigations available if needed:
- Content-hash cache of detection results (the `bothBackend`
  already hashes for diff dedup; the same cache can short-circuit
  detection).
- Parallel goroutine execution when N engines active (already
  the case in `bothBackend`; the new multi-engine wiring inherits).

Decision: measure post-implementation, optimise only if a real
session feels slow.

### 2.8 Backwards compatibility

None. shhh is pre-release; sole user. No deprecation cycle, no
fallback shims. The rename `shhh-native` → `shhh-native` and removal of
running both engines (Phase 6 reintroduces a `union` pseudo-engine for bench) happen in one clean refactor.

## 3. Implementation order (informational)

A suggested order. Each step is testable in isolation; do not
batch them into a megacommit.

1. Rename `shhh-native` → `shhh-native` everywhere (constants, mode
   strings, docs, comments, bench output, gitleaks-spike.md).
2. Add `Engines []string` to `Config`; load/save round-trip
   tested. Default value `["gitleaks"]` when unset.
3. Wire `Config.Engines` into `detector.NewFromConfig()` (new
   constructor). Keep `NewFromEnv()` as a thin debug override.
4. Implement `.shhhignore` reader: gitignore lib (`go-gitignore`
   or similar), layered loader (gitleaks defaults via
   `gitleaksconfig.Config`, then `~/.shhh/.shhhignore`, then
   `<project>/.shhhignore`).
5. Wire the merged ignore matcher into `scanner.New`.
6. Add `shhh ignore list` subcommand: print resolved rules with
   per-rule source attribution + the versioned GitHub link.
7. Wire interactive engine selection in
   `cmdinstall/interactive.go` (checkbox UI; refuse empty
   selection).
8. Update install output text per §2.6.
9. Add `shhh licenses` subcommand.
10. Update `cmdbench` to take `--engines` instead of mode strings.
11. Remove `ModeBoth` and its `bothBackend`; replace with a
    generic multi-engine runner driven by `Config.Engines`.
12. Update `CLAUDE.md`, `README.md`, `docs/gitleaks-spike.md` to
    reflect the new shape.

## 4. Out of scope for this iteration

- `.shhhtrust` (feature B, hook bypass) — separate design pass.
- TTL / ephemeral ignore entries.
- Additional third-party engines (trufflehog, detect-secrets).
  Pattern allows them; integration awaits proven need.
- Per-engine ignore (e.g., "ignore go.sum for gitleaks only, not
  for shhh-native"). Single shared `.shhhignore` keeps the mental
  model small. Revisit only if a real case demands per-engine
  scoping.

## 5. Open questions

- **Naming of `.shhhignore`** vs alternatives like `.shhh/ignore`
  or `.shhhrc`. Current pick: `.shhhignore`. Mirrors `.gitignore`,
  `.eslintignore`, `.semgrepignore`. Final unless someone objects.
- **`shhh ignore` subcommand surface beyond `list`**. Probable
  needs over time: `shhh ignore add <pattern>`, `shhh ignore
  check <path>` (does this path get ignored, and by which rule?).
  Not blocking the first cut.
- **CI distribution of `.shhhignore`**. Project-scoped
  `.shhhignore` should be committed to the repo (it's not a
  secret); install output reminds the user, mirroring the
  current per-project `settings.json` reminder.
