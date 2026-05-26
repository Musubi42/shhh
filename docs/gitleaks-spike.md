# Gitleaks integration spike — 2026-05-26

1-hour investigation to decide if/how shhh should replace its
bespoke detector with gitleaks. Triggered by ROADMAP item #2 and
the user's 2026-04-15 stance ("custom rules are bad; depend on
vetted upstream"). Output: a recommendation, not code.

## TL;DR

**Recommend Option A (library integration), feature-flagged, with
our transformation layer kept as-is.** Bloat and perf are
acceptable, coverage gain is real (222 rules vs ~30), the API is
clean, and the spike binary actually built and ran in < 5 minutes.

The one caveat that needs follow-up before committing: gitleaks
**missed** `GITHUB_PAT` and `POSTGRES_CONNSTRING` on our
`leaky-project/.env` fixture where shhh's bespoke env-aware pass
caught them. Investigate before flipping the default.

## Measurements

### API surface

```go
import "github.com/zricethezav/gitleaks/v8/detect"

d, err := detect.NewDetectorDefaultConfig()
findings := d.DetectString(content)
// or DetectBytes, DetectReader, Detect(Fragment) for richer cases
```

Finding shape (the fields we'd actually consume):

| Field | Use |
|---|---|
| `RuleID` (string) | Maps to our placeholder label scheme |
| `Match` (string) | The matched text (for the redaction transformation) |
| `Secret` (string) | The captured secret if a sub-group was used |
| `StartLine`/`EndLine` | For line-level reporting in `shhh scan` |
| `Entropy` (float32) | Already computed; we can drop our Shannon code |
| `Fingerprint` | Stable per-secret hash; replaces our `sha1[:3]` suffix |

### Numbers

| Metric | Today (shhh) | With gitleaks | Δ |
|---|---|---|---|
| Default rules | ~30 | **222** | +192 (7.4×) |
| Binary size (standalone) | 15 MB | ~11 MB (just gitleaks) | — |
| Binary size (projected merged) | 15 MB | ~17-18 MB | +2-3 MB |
| Scan time, 971-line composite | 40 ms | 70 ms | +30 ms (1.75×) |
| License | MIT | MIT | compatible |
| Last release | — | v8.30.1 (Mar 2026) | "feature complete" |

The perf delta (30 ms / read) is comfortably under any hook
threshold humans notice (≥100 ms).

### Coverage comparison

Same fixture, `testdata/fixtures/leaky-project/.env`:

| Secret type | shhh | gitleaks |
|---|---|---|
| STRIPE_LIVE_KEY (`sk_live_…`) | ✓ | ✓ |
| OPENAI_PROJECT_KEY (`sk-proj-…`) | ✓ (specific rule) | ✓ (generic-api-key) |
| AWS_ACCESS_KEY (`AKIA…`) | ✓ | ✓ |
| **GITHUB_PAT** (`ghp_…`) | ✓ | **✗ MISSED** |
| **POSTGRES_CONNSTRING** (`postgresql://…`) | ✓ | **✗ MISSED** |
| JWT_TOKEN (`eyJ…`) | ✓ | ✓ |
| ENV_CUSTOM_SECRET (high-entropy passphrase) | ✓ (entropy gate) | ✓ (generic-api-key) |

Second fixture `hook-playground/.env`: gitleaks caught the github-pat
this time AND surfaced `anthropic-api-key` as a typed rule (we have a
specific rule for it too). Still missed POSTGRES_CONNSTRING.

False-positive sanity check on `eval-corpus/public-examples/README.md`
(documentation containing example-shaped strings): **both = 0
findings**. No regression on the FP axis.

### Why gitleaks misses what it misses

The two MISSED cases on leaky-project/.env are both rules where
gitleaks expects context that env files don't provide:
- `github-pat`: the rule may require an `assignment` context (token after `=`) that the .env line `GITHUB_TOKEN=ghp_…` does match — yet it didn't fire. Worth poking at the rule's regex.
- `postgres`: gitleaks' generic-url rule may have stricter shape requirements than ours.

This isn't a fatal coverage gap (the second fixture caught them),
but it suggests **gitleaks alone is not a strict superset** of our
current detector. The migration must keep our env-aware pass as
either a pre-filter or a co-runner.

## What stays ours regardless

1. **Structural redaction.** Gitleaks tells us "this match is a
   postgres URL." We still need to transform
   `postgresql://admin:s3cret@host:5432/db` →
   `postgresql://[USER]:[PASSWORD]@host:5432/db` so the agent can
   reason about the database structure without seeing credentials.
   This is core shhh value, not detection.
2. **Typed placeholder format** `[LABEL:prefix:hash]` and the
   per-session map for rehydration. Gitleaks doesn't care; this is
   shhh's output contract.
3. **Allowlist / bypass** (ROADMAP #3, not yet built). Gitleaks has
   its own allowlist concept (regex + path) which we could honour,
   but the user-facing surface (`shhh allow <pattern> in <path>`)
   is ours to design.
4. **The hook plumbing** (cmdhook, install, audit, redact). None
   of that changes.

What gitleaks REPLACES:

- `internal/detector/` (Shannon entropy, charset gates, integrity-prefix skip list)
- Most of `internal/rules/rules.go` (the regex catalog)
- The bespoke .env-aware pass (probably; needs investigation per "why gitleaks misses" above)

## Recommended migration shape

Multi-step, multi-session. Each step is independently shippable.

### Step 1 — Add gitleaks as a parallel detector behind a flag

```
SHHH_DETECTOR=legacy   # default; current behavior
SHHH_DETECTOR=gitleaks # use gitleaks only
SHHH_DETECTOR=both     # run both, log diffs to stderr, prefer legacy findings
```

Add a `Detector` interface in `internal/detector/`. Two
implementations: `legacyDetector` (existing code) and
`gitleaksDetector` (new). The hook reads `SHHH_DETECTOR` from env
at startup.

### Step 2 — Calibrate

Run running both engines (Phase 6 reintroduces a `union` pseudo-engine for bench) for a few real sessions on the
maintainer's machine. The diff log shows:
- New true positives gitleaks catches (good, switch over)
- Things only legacy catches (figure out why, port the rule or
  keep legacy logic as a pre-filter)
- New false positives gitleaks triggers (compose the allowlist or
  patch our rule mapping)

Stop calibrating when the diff log is empty for a week of normal
use.

### Step 3 — Flip the default

`SHHH_DETECTOR=gitleaks` becomes default. `shhh-native` and the multi-engine wrapper
remain as escape hatches for one release cycle.

### Step 4 — Delete legacy

Remove `internal/detector/` entropy code and most of
`internal/rules/rules.go`. Keep the structural-transformation
layer (Postgres, etc.) and the allowlist.

## Risks

- **gitleaks rule semantics ≠ ours.** Their `generic-api-key`
  fires on anything that looks like an API key; we have separate
  `OPENAI_PROJECT_KEY`, `ANTHROPIC_KEY`, etc. Our placeholder
  labels are part of the agent's narration ("Claude saw a Stripe
  key, not an opaque high-entropy blob"). A mapping table from
  gitleaks `RuleID` → shhh label is required. Maintenance
  overhead: one entry per new gitleaks rule, mostly automated by
  pattern-matching the rule name.
- **Dependency lock-in.** Even though gitleaks is MIT and
  "feature complete," we'd be coupled to their rule shape and
  release cadence. If they break API in v9, we have to migrate.
  Mitigation: pin to v8.30.1 explicitly, treat their next major
  as an opt-in upgrade.
- **Binary size grows by 2-3 MB.** Acceptable today; flag for the
  user.

## What I did NOT investigate

Out of scope for this 1h spike, real work but not blocking the
A/C decision:
- Exact RuleID → shhh label mapping (manual table; ~30 entries
  high-coverage, fall back to generic for the long tail)
- Allowlist semantics overlap (gitleaks has theirs, we'd want
  ours)
- Tests: how to keep our test fixtures meaningful when gitleaks
  drives detection (probably: keep them as integration tests,
  drop the "expected entropy" assertions)

## Decision needed

If you want to proceed: I open Step 1 (Detector interface +
gitleaks-backed impl + env flag) as the next task. ~1 session of
work, no user-visible change. Step 2 (calibration) is dogfooding
+ diff-log review, no code. Step 3-4 follow once we trust the
diff log.

If you want to defer: ROADMAP #3 (allowlist) is the smaller,
more-immediate-impact alternative. Allowlist also benefits gitleaks
later (its allowlist semantics overlap with ours, easier to design
once and reuse).

---

## Step 1 — Landed 2026-05-26

The Detector interface, gitleaks backend, and `SHHH_DETECTOR`
env flag shipped in the same session as this spike doc. Files
added: `internal/detector/backend.go`, `internal/detector/gitleaks.go`,
`internal/detector/factory.go`, `internal/detector/factory_test.go`.
`redactor.New` and `scanner.New` upgraded to accept the interface.

### How to use it

```sh
shhh scan <path>                       # legacy (default, unchanged)
SHHH_DETECTOR=gitleaks shhh scan <p>   # gitleaks only
SHHH_DETECTOR=both shhh scan <p>       # shhh-native as source of truth + diff log
```

The hook (`cmd/shhh/cmdhook/sessionstore.go`) also reads
`SHHH_DETECTOR`. Set it in your shell rc to dogfood the new
backend across all Claude Code sessions for a week:

```sh
export SHHH_DETECTOR=both   # in ~/.zshrc
```

### What the diff log looks like

Stderr line per unique file content where the two backends
disagree:

```
shhh detector-diff: legacy=6 gitleaks=5 only-shhh-native=[GITHUB_PAT OPENAI_PROJECT_KEY POSTGRES_CONNSTRING] only-gitleaks=[GENERIC_API_KEY GENERIC_API_KEY]
```

Read as: legacy caught 6, gitleaks caught 5. Three labels only
legacy fired on. Two `GENERIC_API_KEY` hits only gitleaks fired
on (these are real secrets gitleaks classifies coarsely; legacy
catches them with more specific labels like `OPENAI_PROJECT_KEY`).

### Updated numbers (post-implementation)

| Metric | Spike projection | Actual |
|---|---|---|
| Binary size | 17-18 MB | **23 MB** (+8 MB) |
| New transitive deps | a few | golang.org/x/exp, gopkg.in/ini.v1, gopkg.in/yaml.v3, github.com/stretchr/testify, others |
| Test time | — | +0.3s (gitleaks rule compile on startup) |

Binary size grew more than projected. Tolerable for a CLI; flag
for an eventual `make build-slim` variant if user-facing size
becomes a concern.

### Confirmed regressions to fix before flipping default

On the canonical leaky-project/.env fixture, gitleaks alone misses
three labels legacy catches:

| Label | Why gitleaks misses |
|---|---|
| `GITHUB_PAT` (`ghp_…`) | Gitleaks' `github-pat` rule didn't fire on the bare KEY=VALUE form here; the second fixture (hook-playground) caught it. Investigate context requirements. |
| `POSTGRES_CONNSTRING` | No equivalent rule in gitleaks defaults; would need a custom config. |
| `OPENAI_PROJECT_KEY` | Gitleaks fires `generic-api-key`, which is correct as a hit but loses our specific label. The mapping table in `gitleaks.go` can be extended once we know the gitleaks RuleID it uses (might be `generic-api-key` requiring sub-pattern context). |

None of these block Step 1 (which is "make the swap possible").
They're work for Step 2 (calibration) and Step 3 (flip default).

### Next concrete step

Run running both engines (Phase 6 reintroduces a `union` pseudo-engine for bench) for a week of real usage, eyeball the
diff log, then decide:
- If the only-gitleaks set is mostly `GENERIC_API_KEY` for things
  legacy labels specifically: extend the label mapping table.
- If only-shhh-native keeps growing: keep shhh-native as a pre-filter,
  layer gitleaks on top instead of replacing.
- If parity is reached: flip default to gitleaks, deprecate
  legacy entropy code.
