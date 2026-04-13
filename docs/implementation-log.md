# Implementation log

Running journal of what we built, what it taught us, and what decisions
it forced. Each entry corresponds to a step in `implementation-roadmap.md`.

**Format per entry:**
- **Goal** — one sentence.
- **Built** — bullets of what shipped.
- **Learned** — what surprised us or what the implementation revealed that
  a reader of the diff could not reconstruct.
- **Decisions forced** — architectural calls made because of this step.
  Link to ADRs when that directory exists.
- **Next** — the step that this entry unblocked.

Entries are newest-first. Roadmap step numbers are stable; if we reorder
the roadmap, we log the reorder as its own entry.

---

## Entry 6 — Task 1 JWT decode, compensatory tools bundle, Expected-by-mode (roadmap steps 6+7+8, partly 10)

**Goal:** land the first compensatory tool (`decode_jwt_safely`), the
first Tier-1 agent-dependent task (JWT decode), and whatever harness
primitives the task needs to run end-to-end in all four modes.

**Built:**
- `internal/eval/jwt.go` — `BuildJWT(claims)` and `DecodeJWTPayload(jwt)`
  with a `JWTClaims` struct (sub, iss, aud, exp, iat, scopes). Signature
  verification is intentionally absent: the point of the compensatory
  tool is claims inspection, not authentication.
- `internal/eval/jwt_test.go` — round-trip and malformed-input tests.
- `internal/eval/compensatory.go` — `CompensatoryTools` bundle bound to
  a `Redactor` + `SessionID`. Exposes `DecodeJWTSafely(placeholder)` and
  `CompareSecrets(a, b)`. Both fail closed on unknown placeholders.
- `internal/eval/tasks/t01_jwt_decode.go` — task 1 implementation. It
  constructs a fixture JWT at runtime (deterministic claims), redacts
  the `.env` content per mode, simulates agent extraction via
  `extractEnvValue`, tries direct JWT decoding, and falls back to
  `DecodeJWTSafely` in compensatory mode. A rubric helper checks that
  returned claims match the expected sub/exp/scopes.
- `internal/eval/task.go` — new `Expected` type and `Task.Expected(mode)
  Expected` method. Three possible cell outcomes now: design-held,
  regression, surprise pass.
- `internal/eval/runner.go` — `Cell.Matches()`, `Cell.IsRegression()`,
  `Cell.IsSurprisePass()`, `HasRegressions([]Cell)`. `Matrix()` renders
  with four glyphs: `✅ pass`, `✅ fail-ok`, `❌ regression`, `⚠️ surprise`.
  Mismatches section replaces the old failures section.
- `cmd/shhh-eval/main.go` — exit code 1 only on regressions. Surprise
  passes are surfaced in the matrix but do not fail CI.
- All pre-existing tasks (t07, t08) gain `Expected()` methods returning
  `ExpectedPass` for their single supported mode.

**Learned:**

1. **The "mock agent harness" from roadmap step 6 was a mirage.** I
   expected to build a reusable mock-agent abstraction. When I wrote
   task 1 the right shape turned out to be: each task encodes its own
   simulated-agent logic directly in `Run`, and the only shared
   abstraction is the `CompensatoryTools` bundle (which is not an agent
   at all — it is the MCP tool surface). The "agent" in a Phase-0 task
   is just Go code that mimics what a reasoning agent would do:
   extract a value, try to decode, fall back to compensatory tools.
   When a real Claude Code runner lands in step 16, the task will
   either swap its `Run` body to drive the runner, or we will factor
   the shared prompt/rubric shape out then — after seeing the real
   thing. Roadmap step 6 is re-scoped to "shared primitives for
   task-internal agent simulation" and mostly consists of
   `CompensatoryTools`.

2. **Task 1 failing in `redact` and `redact+rehydrate` mode is the
   design, not a bug.** I ran the eval and saw exit 1 with four cells
   of red. First instinct: "the runner treats failures as failures."
   Correct reading: the runner was missing a concept. Some tasks are
   *supposed* to fail in some modes because the mode is deliberately
   weaker than what the task needs. Treating every failure as a
   regression erases the whole point of a Tier-1 task, which is to
   demonstrate what pure redaction cannot do. Adding `Expected(mode)`
   made the matrix narrate the story: baseline passes, redact fails
   as designed, redact+rehydrate fails as designed (because rehydration
   only touches `tool_use` args, not the content the model reasons
   about), compensatory mode passes via the tool. That is the intended
   story and the matrix now tells it in one screen.

3. **The rehydration fail in `redact+rehydrate` on task 1 validates
   the critique prediction literally.** The original critique said:
   *"rehydration in tool_use args doesn't help when the agent is
   reasoning about the value in the model context — the value is
   inside an opaque base64 blob."* Task 1 now empirically confirms
   this. When Phase 3 ships the real proxy and runs task 1 against
   real Claude Code, we expect the same result: rehydration changes
   nothing for a claims-inspection task because there is no tool call
   to rehydrate. The compensatory tool is the *only* fix, and that is
   why compensatory tools are a first-class product surface in the
   PRD, not an afterthought.

4. **`CompareSecrets` is trivial on top of `ResolvePlaceholder`** — two
   lookups and an equality check. This is a hint that most compensatory
   tools will be thin, because the session map already carries the
   necessary structure. The compensatory surface is valuable less as
   novel logic and more as a discoverable API: the agent needs to know
   the tool exists, even if its implementation is three lines.

5. **`ResolvePlaceholder` via rehydration-of-a-single-string is a
   kludge.** `ShhhAdapter.ResolvePlaceholder` calls `r.Rehydrate([]byte(placeholder))`
   and compares input to output to detect "was substituted." This
   works because the placeholder regex matches the full string, so
   rehydrate can replace it. But it is obviously the wrong primitive
   for a production lookup — it pays a regex walk to do what should be
   a single map fetch. When the daemon's session map gets a direct
   lookup method (Phase 3), the adapter rewrites to use it. Logged as
   a Phase-0 tech debt item.

**Decisions forced:**

- **Eval exit code distinguishes regressions from surprise passes.**
  Only regressions fail `make bench`. Surprise passes are warnings the
  matrix prints but do not fail CI. Rationale: a surprise pass might
  mean the product quietly became better (great!) or it might mean the
  task stopped testing what we thought (also important!) — neither
  outcome deserves a red build until a human looks at the mismatch.
  This is encoded in `HasRegressions()` which is what `cmd/shhh-eval`
  checks before exiting.
- **`Expected(mode)` is per-task-per-mode, not a global policy.** I
  considered a simpler "all cells expected to pass" policy with an
  annotation like `ExpectedFailModes`. Per-mode method is more
  uniform: every task answers "what do you expect in this mode?" and
  no task has to opt into a non-default behavior.
- **Fixture content for task 1 is built in Go, not stored on disk.**
  The JWT is generated at runtime from a fixed payload map. Keeps the
  task self-contained and makes the "this is what we assert"
  relationship explicit in the task source. Tasks that need a file
  tree (task 4, 5) will use on-disk fixtures; the two patterns
  coexist.
- **Task simulation lives inside `Run`, not in a separate
  `AgentAdapter`.** When we wire a real agent runner (roadmap 16), we
  will either refactor `Run` to delegate to the runner or extract a
  thin protocol — the exact shape will depend on what the runner
  needs. Not worth guessing now.

**Next:** the natural next step is roadmap step 9 (task 3: consistency
across files) which exercises `CompareSecrets` as its compensatory
tool. That task will reveal whether base64 normalization needs to
happen at detection time or at comparison time, and whether the
session map's content-hash approach suffices or needs a canonical
form.

---

## Entry 5 — Makefile reproducibility contract (roadmap step 4)

**Commit:** `38f833d`

**Goal:** give a first-time reader a single command that reproduces every
number in the repo.

**Built:**
- `Makefile` with targets `build`, `test`, `vet`, `bench`, `scan`,
  `fixture-scan`, `clean`, `help`.
- `./bin/` artifact directory, gitignored.
- `bench` depends on `build`, so a fresh clone runs the suite with `make bench`.

**Learned:**
- The Phase 1 ship criterion ("a skeptic clones and runs `make bench`")
  is cheap to honor if you set up the target at step 4, not step 20.
  Deferring it would have let every commit use ad-hoc `go run` invocations
  that silently diverge.

**Decisions forced:**
- Binary output directory is `./bin/`, not `./build/` or `./out/`. Matches
  Go convention.
- `make bench` does not persist results to disk yet — it writes to stdout
  only. Result persistence is deferred to step 21 (baseline results doc)
  when we know what format the artifact needs.

**Next:** roadmap step 5 (this log + roadmap).

---

## Entry 4 — Scanner `.env` cross-reference (roadmap step 3)

**Commit:** `40c308e`

**Goal:** wire the `detector.CheckEnvValue` strength gate into the scanner
so custom secrets that no pattern rule would match still get surfaced.

**Built:**
- Two-pass `scanner.Scan`: pass 1 collects `.env`-sourced values that
  pass the gate (length ≥ 12, entropy ≥ 3.0, not on denylist), pass 2
  runs the detector per file and additionally checks the cross-ref set.
- `ENV_CROSSREF` finding label for non-`.env` files: "Value from a .env
  file — possible hardcoded credential."
- `ENV_CUSTOM_SECRET` finding label for the `.env` source itself:
  "Custom secret (passed strength gate)." Surfaces values that are strong
  enough to be credentials but fall just below the standalone entropy
  threshold (4.5 bits/char).
- `spanAlreadyCovered` prevents double-counting when a pattern rule
  already covered the same offset range.
- Fixture updated: `testdata/fixtures/leaky-project/.env` gains
  `INTERNAL_LEGACY_TOKEN=Mk9zPwXr7AqN4bVtC2yLhG` (22 chars, entropy 4.46 —
  deliberately below standalone threshold); `src/index.js` gains a
  `LEGACY_AUTH` constant with the same value. Scan output now shows 9
  findings across 3 files instead of 7 across 2.

**Learned:**
- **The standalone entropy threshold (4.5) and the cross-ref gate (3.0)
  must differ, and the gap matters.** A value strong enough to be a
  real secret sometimes falls below 4.5 because its charset is narrow
  (e.g., alphanumeric with a predictable case distribution). The signal
  "this value is literally in `.env`" is additional evidence we can
  exploit. The two-tier model lets us be conservative on standalone
  detection (low false positive rate) while still catching the hard
  cases when context supports it.
- The `spanAlreadyCovered` check exists because the cross-ref pass and
  the detector pass both walk the same content; without dedup, a
  Stripe key in `.env` would be reported twice (once as a pattern match,
  once as a cross-ref). The span check is the right primitive because
  both passes produce `[start,end)` intervals.
- Writing a fixture that demonstrates a feature to a human reader
  (who will read the masked scan output) is almost more work than
  writing the feature itself, because the demonstration must be
  realistic: a hardcoded fallback from "a 3am incident in 2024" is
  plausible, random test gibberish is not.

**Decisions forced:**
- The cross-reference is scoped to `.env`-shaped files only, not
  arbitrary config files (YAML, TOML, JSON). Expanding later is a
  one-day change; starting narrow means the first version's FP
  rate is predictable.
- Cross-ref findings have their own labels (`ENV_CROSSREF`,
  `ENV_CUSTOM_SECRET`), not a shared `CUSTOM` label. When we eventually
  add YAML-sourced cross-ref we will add a third label rather than
  overload one.

**Next:** roadmap step 4 (Makefile).

---

## Entry 3 — Known-placeholder allowlist (roadmap step 2, part B)

**Commit:** `a7be147`

**Goal:** stop flagging AWS-documented credentials (`AKIAIOSFODNN7EXAMPLE`
and the matching secret access key) which are syntactically valid but
universally public.

**Built:**
- `internal/rules/allowlist.go` with `KnownExamples` map and
  `IsKnownExample(value) bool`. Every entry carries a citation comment.
- Allowlist applied at two points in `detector.Detect`: after a pattern
  match (drops the AKIA example from the `aws-access-key` rule) and after
  an entropy-fallback match (drops the 40-char base64 secret access key
  that otherwise triggers entropy).
- `TestDetect_KnownExamplesAllowlisted` covering both paths and the
  Stripe docs test key.
- Task 8 simplified: no longer carves out `aws-docs-placeholder.txt` as
  a known issue; now expects zero findings across the entire corpus.

**Learned:**
- **A single allowlist is checked at two detector stages, not one.**
  I originally only checked in stage 1 (pattern match). The AWS secret
  access key is 40 base64 chars with no fixed prefix — no pattern rule
  catches it, so it took the entropy fallback path and evaded the
  allowlist. Task 8 caught this immediately on re-run; one more fix,
  re-run, green. This is the eval loop working as designed.
- The allowlist is deliberately tiny (4 entries). The temptation is to
  add every `.env.example` value we've ever seen; the invariant we keep
  is "only values that are *universally* public and documented." Keeps
  the list auditable.

**Decisions forced:**
- Allowlist entries require a citation (URL or doc reference) in the
  comment. This is the filter that prevents creeping additions.
- Allowlist is literal-match, not pattern-match. A rule like "any AWS
  access key ending in `EXAMPLE`" sounds tempting but would mask real
  vulnerabilities.

**Next:** roadmap step 3 (`.env` cross-reference).

---

## Entry 2 — Eval harness + tasks 7 & 8, detector calibration (roadmap steps 1 + 2, part A)

**Commit:** `d26b9b3`

**Goal:** stand up the product-agnostic eval harness with two tasks that
run without a live agent, then fix whatever the tasks reveal.

**Built:**
- `internal/eval/iface.go` — product-agnostic `Redactor` interface with
  `NewSession`, `Redact`, `Rehydrate`, `ResolvePlaceholder`.
- `internal/eval/adapter_shhh.go` — shhh reference adapter wrapping the
  Phase 0 library behind the interface. Maintains a per-session
  `*redactor.Redactor` keyed by opaque `SessionID`.
- `internal/eval/task.go` — `Task` interface, `Mode` / `Tier` / `Result`
  types. Four modes: `no-redaction`, `redact`, `redact-rehydrate`,
  `redact-rehydrate-compensatory`. Tasks declare their supported modes.
- `internal/eval/runner.go` — `Run` loop, `Matrix` report.
- `internal/eval/tasks/t07_placeholder_entropy.go` — generates 100
  distinct Stripe-shaped keys, redacts each in a fresh session, verifies
  no placeholder collisions across sessions and no 6+ char substring of
  any real value's body appears in its placeholder.
- `internal/eval/tasks/t08_public_corpus.go` — runs the redactor over
  `eval-corpus/public-examples/` and fails on any finding outside the
  (then) excluded AWS docs file.
- `eval-corpus/public-examples/` — 5 known-safe files: git SHAs, UUIDs,
  package-lock integrity hashes, version constants (`password`,
  `changeme`, `localhost`), AWS-documented credentials.
- `cmd/shhh-eval/main.go` — CLI.
- **Detector calibration fixes surfaced by running task 8 and
  observing 4 false positives:**
  - Token regex `[A-Za-z0-9+/=_\-]{16,}` → `[A-Za-z0-9+/\-]{16,}`.
    Removing `=` and `_` makes `PREVIOUS_COMMIT=f1e2...` tokenize into
    `PREVIOUS`, `COMMIT`, `f1e2...` instead of one span whose mixed
    charset inflated entropy past the 4.5 threshold.
  - Charset-diversity gate: at least 18 distinct characters required for
    an entropy match. Rejects pure hex (16 distinct) and UUID-with-hyphen
    (17) without affecting base64 (up to 64). This is a sharper filter
    than raising the entropy threshold because it directly encodes the
    intuition "if the alphabet is small, this is probably a public
    identifier, not a secret."
  - Subresource Integrity prefix skip (`sha1-`, `sha256-`, `sha384-`,
    `sha512-`). Matches the shape of `package-lock.json` integrity
    fields, which are genuinely high-entropy base64 but public content
    hashes.

**Learned:**
- **Tokenization bugs hide as entropy bugs.** When the token regex is
  too permissive, entropy goes up because the charset widens, and the
  bug looks like "threshold too low." The real fix is tokenization, not
  threshold tuning. This is the single most important calibration
  lesson so far: next time entropy fires on something unexpected, check
  the token boundary first.
- **The eval's first run producing failures is the point, not a bug.**
  Initial results: task 7 passed, task 8 failed with 4 FPs. That is the
  eval working as designed. If task 8 had passed on the first run I
  would not have found the tokenization bug until much later.
- The harness design deliberately does not know shhh exists. All task
  code talks to `Redactor` as an interface, not `*redactor.Redactor`.
  When we eventually split `shhh-eval` into its own repo (roadmap step
  22), the only import surface that has to move is the adapter.
- `AllModes()` is called in the matrix renderer even for tasks that
  declare a single mode; the renderer renders unsupported cells as `—`.
  This lets the matrix be visually stable as the suite grows.

**Decisions forced:**
- **Charset-diversity gate over entropy-threshold bump.** Could have
  raised the threshold from 4.5 to 5.0 instead. Chose diversity because
  base64 (entropy 6+) still passes even with the new gate, while hex
  and UUIDs are cleanly rejected, and the intuition ("small alphabet =
  public ID") is easier to explain in documentation than "bits-per-char
  ≥ 5.0 is the magic number."
- **Integrity-prefix skip is data, not rule metadata.** Stored as a
  string slice on the detector rather than as an extra `SkipPrefix` field
  on individual rules, because the prefix skip applies to the entropy
  pass, not to any pattern rule.
- **Task tier classification is part of the `Task` interface, not the
  runner.** This means the renderer can group by tier later without the
  runner knowing anything about task content.

**Next:** roadmap step 2 part B (known-placeholder allowlist).

---

## Entry 1 — Phase 0 foundation (roadmap step 0)

**Commit:** `b41d75c`

**Goal:** land the PRD rewrite, phase documentation, and a working
detector + scan CLI before adding any eval or agent machinery.

**Built:**
- PRD rewrite covering threat model reframe, rehydration commitment,
  PostToolUse replacing bash parsing, Edit/Write refusal, compensatory
  MCP tools as first-class surface, screenshot-safe scan output, entropy
  gates on `.env` cross-reference, self threat model, reach/retention/
  quality metrics split.
- `docs/` with one file per phase (0–6), plus `README.md` index.
- `internal/rules` — 15 built-in patterns (Stripe, AWS, GitHub, OpenAI,
  Anthropic, Slack, Google, JWT, postgres/mongo URLs, PEM keys).
- `internal/detector` — pattern + Shannon entropy pipeline,
  configurable threshold, denylist for common sentinels.
- `internal/session` — salted deterministic placeholder map, in-memory
  only, fail-closed resolve on unknown placeholders, `Compare`
  primitive for the future compensatory tool.
- `internal/redactor` — redact/rehydrate round-trip on top of detector
  and session. Splice walks findings in reverse to keep offsets valid.
- `internal/scanner` — directory walker with sensitive-path detection,
  skip dirs, binary-file rejection, 10 MB size cap.
- `cmd/shhh` — CLI with `scan` (text/json/md, screenshot-safe default,
  `--show-details` opt-in) and `redact` (stdin/file, `--rehydrate`).
- Unit tests for detector, session, redactor, scanner.
- `testdata/fixtures/leaky-project/` with `.env`, `config/auth.json`,
  `src/index.js`.

**Learned:**
- Stdlib `flag` stops parsing at the first non-flag argument, so
  `shhh scan <path> --format json` leaves `--format json` unparsed. Fix
  was a small `reorderFlagsFirst` pre-pass that splits flags from
  positionals. A nicer fix would be `github.com/spf13/pflag` but adding
  a dependency for one ergonomic issue is not worth it in Phase 0.
- Offset-based splice (reverse-walking findings) is trivially correct
  but the ordering matters: if you walk forward and substitute in place,
  every subsequent finding's offsets shift by `len(placeholder) - len(value)`.
  Reverse-walking sidesteps the bookkeeping entirely.
- The masking in the scan output (`sk_live_•••`, `postgresql://•••`)
  has to be in the *display layer*, not the detector or redactor. The
  redactor knows the real value; the scan CLI is the thing that chooses
  what to show the user. This separation means the eval harness (which
  never displays to a user) can use the raw values freely.

**Decisions forced:**
- **Go, not Rust.** Bubble Tea for the future TUI and the Go HTTP ecosystem
  for the Phase 4 proxy were the deciding factors. Phase 0 does not use
  either yet, but picking the language is a step-0 decision.
- **15 built-in rules, not gitleaks.** Phase 0 starts narrow to keep
  the detector easy to reason about. Gitleaks integration is roadmap
  step 13, after the eval has validated what the detection signal needs
  to cover.
- **Session map in-memory only, no persistence.** The only state that
  crosses process boundaries is the redacted output itself. Placeholders
  are valid only within the session that generated them.

**Next:** roadmap step 1 (eval harness).

---

## How to add a new entry

When you finish a roadmap step:

1. Bump the entry number and write the new entry at the *top*.
2. Fill in every field: `Goal`, `Built`, `Learned`, `Decisions forced`,
   `Next`. The hardest one is `Learned` — force yourself to write
   something even if the step "just worked," because "nothing surprised
   me" is itself useful signal.
3. Update the `Status` column of the corresponding roadmap step.
4. Commit the log entry in the same commit as the code, not separately.
