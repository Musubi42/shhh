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

**Historical note (filed at entry 12):** entries 1–11 describe the
prior 22-step eval-first roadmap that got scrapped. They are retained
as history and contain real calibration learnings (tokenization vs
entropy, charset-diversity gate, integrity-prefix skip, structural URL
redaction, the gitleaks transcription decision) but they are **not**
the current direction. Do not use them to drive forward work. See
`docs/postmortem-eval-overbuild.md` and `CLAUDE.md`.

---

## Entry 13 — Per-finding visibility in the Read + Bash narration

**Date:** 2026-04-15
**Context:** Eurio real-agent session surfaced that when shhh
redacted a secret mid-prose, Claude could only tell the user "shhh
protected a secret" with no way to point at which one or where. On
`.env` files the problem didn't exist: the `KEY=[PLACEHOLDER]`
line-preserving shape let Claude infer the mapping from the file
content itself, which is exactly why we hadn't noticed the gap
until a file with a Stripe key embedded in a markdown sentence
crossed the wire.

**Built:**
- `cmd/shhh/cmdhook/read.go` — `narrateRedactions` now takes the
  original content and the redactor and, for non-env files, appends
  a per-finding list: `LABEL at line N (placeholder: [...])`. Env
  files are deliberately skipped (the redacted file body already
  carries the mapping, and duplicating it just inflates context).
- `internal/redactor/redactor.go` — new `PlaceholderForFinding`
  helper. Forwards to the existing session map; idempotent.
  Exported so the hook narration layer can cite the exact
  placeholder alongside each finding without reparsing output.
- `cmd/shhh/cmdredact/cmdredact.go` — when invoked with `--session`
  (i.e. from the Bash hook wrapper) and there are findings, emits
  a trailing narration block on stdout with the same
  `LABEL at output line N (placeholder: [...])` format. Gated on
  `--session` so standalone `shhh redact` (eval harness, manual
  CLI) stays byte-exact on stdout.
- `cmd/shhh/cmdhook/cmdhook_test.go` — new
  `TestReadNarrationListsLineNumbersForProseFile` exercising the
  non-env path end-to-end, plus a regression guard inside the
  existing env narration test that fails if `at line` ever leaks
  into the env-path output.
- `testdata/fixtures/hook-playground/notes-with-secret.md` — new
  markdown fixture with a Stripe key embedded in a sentence on
  line 3.
- `testdata/fixtures/hook-playground/README.md` — Test 8b added
  with hook-layer narration pasted from a real run.

**Learned:**
- The visibility gap is narrower than it first looked. Half the
  fixture surface (`.env`, `printenv` output, `cat .env` through
  the Bash wrapper) already round-trips the variable name on the
  same line as the placeholder, so Claude reconstructs the mapping
  for free. Adding metadata on that half is pure noise, which is
  why `envMode` suppresses the per-line listing. Test 8 in the
  fixture README is the empirical record of that — Claude built
  a perfect per-variable table with no help from narration.
- Bash interception is *not* architecturally symmetric with Read.
  Read fires at PreToolUse with the file content already in hand;
  Bash fires at PreToolUse before the command has produced a byte
  of output, so `bash.go` has no findings to narrate. The only
  place in the Bash pipeline that sees findings is `shhh redact`
  running inside the wrapped shell command. This is why the
  Bash-side narration lives in `cmdredact.go` and rides on stdout
  rather than on `hookSpecificOutput.additionalContext`. The
  handoff asked for "symmetric treatment in bash.go" but symmetric
  is a category error here — same *effect*, different code path.
- The Read→Edit ledger bug from entry... (well, from Test 2 in the
  hook-playground README) bit this session hard: the test fixture
  file itself contains a Stripe placeholder, which triggers
  redaction on every `Read` of `cmdhook_test.go`, which makes the
  `Edit` tool refuse the file because its internal read-ledger
  records the cache path instead of the original. The workaround
  inside this session was to patch test files via `python3` from
  the Bash tool, which works but is slow and breaks the normal
  Edit/Read loop. This is more evidence the ledger bug needs a
  real fix — not a workaround in the hook — because it cascades
  into *every* file that happens to contain a detectable token,
  including source files where that token is a test fixture.

**Decisions forced:**
- Narration for Bash lives in `cmdredact.go`, not `bash.go`.
  Documented at the callsite so the next reader doesn't expect
  PreToolUse symmetry with Read.
- `--session` is now the "am I running under the hook?" signal for
  cmdredact. Before this change it only changed the session-map
  plumbing; now it also gates narration emission on stdout. A
  future daemon- or socket-based session threading will need to
  keep the same semantics or narration will start leaking into
  manual pipelines.
- `PlaceholderForFinding` is exported on `Redactor`. It is the
  second method after `Resolve` that exists purely for the hook
  narration layer. If a third appears, the right move is probably
  a small `Narrator` type that holds a `*Redactor` and owns all
  placeholder-citation formatting.

**Next:**
- The Read→Edit ledger bug is now the highest-friction thing in
  the fixture loop. Fixing it cleanly is the next piece that
  actually moves the demo forward.
- Consider an allowlist / bypass affordance (`#shhh:allow` marker
  or per-path config) for cases like `cmdhook_test.go` where a
  fixture deliberately contains a placeholder-shaped string — the
  current behavior (silently redact the placeholder itself) is
  defensible but produces cascading Edit failures inside the shhh
  repo's own test files.

---

## Entry 12 — Pivot: the roadmap was wrong, the product does not exist yet

**Date:** 2026-04-13
**Context:** during step 16 (Docker Claude Code runner) planning.

At the planning conversation for step 16, the user stopped the session
and pointed out, in substance: *"The idea was very simple — a hook
that intercepts secrets before they reach the LLM and replaces them
with a reference. Why are you talking to me about Docker and running
Claude with `claude -p`?"*

That one sentence did what eleven log entries and a Makefile ship
criterion had failed to do: it re-grounded the work in the PRD §1
product description. The 22-step roadmap had been building a
validation harness for a product that was never actually built. The
library components (detector, session map, redactor, rules, scanner)
are real and useful. The eval harness, the four-mode matrix, the
task tier framework, and the planned Docker runner are scaffolding
that never served a real user.

**Filed:** `docs/postmortem-eval-overbuild.md` — full diagnosis of
what drifted, warning signs that should have triggered the stop
earlier, and the exact kept/demoted/deleted list.

**Updated docs (this entry's scope):**
- `docs/implementation-roadmap.md` — replaced the 22-step table with
  a short milestone list. Milestone 1 is the Claude Code hook. No
  phases, no tiers, no eval framework as a deliverable.
- `CLAUDE.md` — new file at the repo root. Loaded into every Claude
  session. Names the forcing function ("is the demo closer to
  working?"), the hard rules derived from the drift, and a reading
  order that privileges the postmortem.
- `PRD.md` — minimal surgical note above §10 (Phase 0) pointing
  readers to the postmortem so neither §10 nor principle 7 can be
  re-weaponized into another eval-first roadmap.
- This log — entries 1–11 stay as history; this entry marks the
  pivot.

**Not touched in this entry:**
The code under `internal/`, `cmd/shhh/`, `cmd/shhh-eval/`. The
library stays. The eval package stays as `go test` regression
coverage. No renames, no deletions in this pass — the goal is to
stop treating eval as a product, not to reshape the files. Code
reshaping belongs to the next session, starting from milestone 1.

**Learned (retroactively):**

1. The instinct *"green checkpoint → next roadmap step → learn →
   commit"* was locally correct and globally wrong. Local correctness
   is not evidence of global alignment. A forcing function tied to a
   real user-visible artifact (the hook actually running in a real
   agent) is the only thing that prevents this drift reliably.

2. "Eval-driven, not architecture-driven" (PRD principle 7) is a
   fragile instruction because it does not pin *when* eval happens.
   Eval after the product ships is the original intent; eval as a
   prerequisite for the product is the inversion we fell into. The
   new `CLAUDE.md` pins the ordering explicitly: the hook ships
   first, real-agent observation drives any further work.

3. AI agents will elaborate any scaffolding they are given. The only
   reliable counter is a tightly-worded instruction file and a
   human who checks, periodically, whether the product described in
   the PRD actually exists. Entry 12 itself is evidence that the
   system can self-correct — but only when a human intervenes.

**Next:** a new session starts from `docs/implementation-roadmap.md`
milestone 1 (Claude Code hook). This log may or may not continue;
if it does, future entries describe product work, not scaffolding.

---

## Entry 11 — Structural redaction for connection strings + tasks 2 & 9 (roadmap steps 14, 15)

**Goal:** land the structural URL redactor the PRD §5 example promised
("`postgresql://admin:s3cret@prod-db:5432/myapp` → `[POSTGRES_CONNSTRING:admin@prod-db:5432/myapp]`"), then ship the two tasks
that validate it: task 2 for connection-string diff, task 9 for
"non-sensitive URLs survive redaction."

**Built:**
- **`Rule.Normalize` hook.** `internal/rules/rules.go` gains an
  optional per-rule `Normalize func(value string) string` that returns
  a structural public description for matched values. Returning `""`
  signals "no structural form; fall back to `PublicPrefix...`" so the
  hook is safe to add without breaking existing rules.
- **`rules.NormalizeConnString`.** Uses `net/url` from the stdlib to
  parse a URL-shaped string and return `user@host:port/path`
  (password stripped, query string stripped). On parse failure or
  missing scheme/host it returns the empty string, which triggers the
  fallback to opaque placeholder — a connection string we cannot parse
  is still worth redacting, just not structurally. Wired into
  `postgres-url` and `mongodb-url`. Square brackets in the structural
  output (e.g., IPv6 `[::1]`) are rewritten to round brackets to
  avoid confusing the placeholder regex.
- **`Finding.StructuralDesc`.** New field on `detector.Finding`,
  populated at detection time by calling the rule's `Normalize` if
  non-nil. Propagates through to the redactor.
- **`session.Map.PlaceholderFor` signature widened.** Fourth argument
  `structuralDesc string`. When non-empty, the placeholder renders as
  `[LABEL:structural_body:suffix]` — no `public_prefix...` middle
  section. When empty, old behavior. All existing callers updated
  with an explicit `""` argument so the compiler catches any miss.
- **`internal/eval/tasks/t02_connstring_diff.go`** — task 2. Two
  postgres URLs that differ only in the password. Rubric classifies
  the diff into `nothing | password | credentials-or-query |
  structural`:
    - `password` is the baseline-mode reading (agent parses two raw
      URLs and notices only the password field differs).
    - `credentials-or-query` is the redact-mode reading: two
      placeholders with byte-identical structural bodies and
      different hash suffixes → agent knows the hidden component
      changed without knowing which one.
    - `structural` is the "host/port/path also differ" bucket.
  Passes in all four modes by design. In `+compen` mode it also
  calls `compare_secrets` as a belt-and-suspenders check that the
  placeholder pair does not spuriously resolve to the same raw
  value.
- **`internal/eval/tasks/t09_url_mismatch.go`** — task 9. One plain
  HTTPS URL (`https://api.example.com/v1/charges`) and one with an
  embedded Stripe key in a query parameter. Validates two
  invariants:
    1. The plain URL is byte-identical across all four modes (the
       detector is not over-reaching on generic URLs).
    2. The with-key URL keeps its `https://api.example.com/v1/charges`
       prefix in every mode; only the Stripe key inside the query
       string is replaced by its placeholder. The raw key value must
       not appear in the observed content in any redact mode.
  Passes in all four modes.
- **Session package test pins.** `TestPlaceholderFormat_Structural`
  asserts: (a) structural placeholders omit the `...`, (b) two
  different values with the same structural description produce
  different suffixes, (c) the structural bodies between the two
  placeholders are byte-identical. `TestPlaceholderRe` gains a
  structural-placeholder case so the rehydration regex cannot be
  weakened without the test catching it.
- `cmd/shhh-eval/main.go` — t02 and t09 wired into the suite in
  their numerical slots.

**Matrix after this entry:**

```
task                                 baseline  redact    redact+rehyd  +compen
[T1] t01-jwt-decode                  PASS      FAIL-OK   FAIL-OK       PASS
[T2] t02-connstring-diff             PASS      PASS      PASS          PASS
[T1] t03-consistency                 PASS      FAIL-OK   FAIL-OK       PASS
[T1] t05-grep-hardcoded              PASS      FAIL-OK   PASS          PASS
[T2] t09-url-mismatch                PASS      PASS      PASS          PASS
[T2] t07-placeholder-entropy         -         PASS      -             -
[T3] t08-public-corpus               -         PASS      -             -
```

Two new "design held everywhere" rows (t02, t09). The three
compensatory-tool-driven stories (t01, t03, t05) are unchanged. Task
8's zero-FP calibration still holds on the public corpus — the new
structural path did not introduce any over-match on non-connection-
string content.

**Learned:**

1. **Structural redaction is the right answer for composite secrets,
   not a workaround.** A postgres connection string is a composite
   of multiple fields, only one of which (the password) is the
   secret. Treating it as one opaque blob was never correct — the
   PRD spelled this out in §5 line 549 — but the implementation had
   shipped the opaque form anyway because adding a `Normalize` hook
   looked like "scope creep." Task 2 forced the fix. The fix is
   small (about 50 LOC including the test pins) and it doubles as
   the right architectural primitive for any future composite-secret
   rule. This is what the eval-drives-product discipline is supposed
   to produce — a spec that waited two steps to be claimed.

2. **`net/url.Parse` is enough for Phase-0 connection strings.**
   I considered writing a small custom parser to avoid surprises
   around escape characters in passwords (postgres allows `%3A`
   etc. in URL-encoded passwords). But every URL the task fixtures
   actually use follows the `scheme://user:pass@host:port/path`
   shape and `net/url.Parse` handles them all. If a future task
   produces a fixture that breaks `net/url`, the `Normalize` hook
   returns `""` and we fall back to opaque redaction — no silent
   data loss. The fallback path is what makes adding `Normalize`
   a safe refactor.

3. **The "design-held in every cell" task shape deserves a name.**
   Tasks 2, 7, 8, 9 all produce the same matrix signature: `PASS`
   in every supported mode. That is a different kind of signal
   from tasks 1/3/5 (which demonstrate a compensatory unblock). It
   means "the product handles this case without intervention." The
   suite now has a mix of both. When writing the Phase-1 baseline
   doc (roadmap step 21) we should probably present them in two
   sections — "compensatory unblocks" vs "no-intervention handled"
   — because they tell different stories about the product.

4. **The classifier in `classifyConnstringDiff` turned out
   shorter than expected.** I thought I would need to do fuzzy
   structural matching on the two placeholders. In practice the
   test is "are the bodies byte-identical and do the suffixes
   differ?" — a one-line structural check on two placeholder
   strings. This works because the session map deduplicates by
   byte-identity of the *value* (not of the structural part), so
   when the structural part is the same and the value differs,
   you get two distinct placeholders with the same body prefix
   and different suffixes. That is exactly the signal the agent
   needs and it falls out of the existing session-map semantics
   for free.

**Decisions forced:**

- **`Rule.Normalize` is the extension point for all structural
  redaction going forward.** Any future rule that wants to expose
  more than a public prefix (e.g., an email address's domain, an
  S3 bucket's region) should provide a `Normalize` function that
  returns the public-but-informative portion. The detector and
  session code are stable; only the per-rule hook changes.
- **Query-string parameters are stripped entirely, not sampled.**
  The temptation was to keep query-string *keys* (so an agent
  could see `?api_key=…` and know where the credential lives)
  while dropping values. Decided against: keys sometimes encode
  tenant info (`?tenant=acme-corp`), and the structural
  description is already human-readable at the path level. Strip
  everything after `?` and let the public URL prefix in the
  surrounding content carry the "this was a URL with query"
  signal.
- **`PlaceholderFor`'s signature grew.** Four string args is at
  the edge of readable, but adding an options struct for two
  rarely-used fields would have cost more clarity than it saved.
  If a fifth field ever appears, switch to an options struct.

**Tech debt logged:**
- `classifyConnstringDiff` only distinguishes "credentials-or-
  query changed" from "structure changed." A finer classifier
  could separate query-param changes from password changes, but
  Phase 0 does not need it. The fixture's "only the password
  changed" case is enough signal for the rubric.
- The structural description is rendered using `net/url`'s
  `u.Host` (which includes port) and `u.Path`. Some deployments
  use `?dbname=foo` style (no path component) where the database
  is in the query string — those would lose the database name
  entirely in the structural form. Skipped; the common case is a
  path component.
- `NormalizeConnString` does not URL-decode the username, so a
  percent-encoded user would show encoded in the placeholder.
  Fine for Phase 0; a production version might decode for
  readability.

**Next:** roadmap step 16 (Docker Claude Code runner). This is the
heavy infrastructure lift — the first real agent integration. All
prior tasks are deterministic Go simulations; step 16 replaces the
simulation boundary with a Docker-hosted Claude Code instance
driven by the harness. Step 17 (task 6 — prompt injection
exfiltration with positive control) and steps 18–19 are downstream
of that runner.

---

## Entry 10 — Gitleaks coverage widening (roadmap step 13) — curated transcription, not library import

**Goal:** widen secret detection coverage using gitleaks' rule set
without committing shhh to gitleaks' dependency graph.

**What we measured first:**

A plain `go get github.com/zricethezav/gitleaks/v8` on the zero-dep
shhh module produced:

- 64 indirect dependencies added
- go.sum from 0 lines to 407 lines
- Transitive graph included:
  - `github.com/tetratelabs/wazero` — a full WebAssembly runtime,
    brought in so gitleaks can optionally run Google's RE2 regex
    engine via WASM (build tag `gore2regex`)
  - `github.com/wasilibs/go-re2` + helpers — the RE2 WASM binding
  - `github.com/spf13/viper` + `cobra` + `pflag` — the gitleaks
    binary's CLI/config framework
  - `github.com/Masterminds/sprig/v3` — Go template engine
  - `github.com/mholt/archives` pulling in `bodgit/sevenzip`,
    `dsnet/compress`, `nwaples/rardecode/v2`, `pierrec/lz4/v4`,
    `ulikunitz/xz`, `therootcompany/xz`, `sorairolake/lzip-go`,
    `klauspost/compress`, `klauspost/pgzip`, `minio/minlz`,
    `andybalholm/brotli`, `h2non/filetype` — archive-format parsers
    for gitleaks' "scan a .zip or .tar.gz for secrets" feature
  - `github.com/BobuSumisu/aho-corasick` — the keyword pre-filter
  - charmbracelet/lipgloss, muesli/termenv, rs/zerolog, hashicorp
    multierror/version, Masterminds/semver, etc.

None of these are wrong — they are what gitleaks needs to be the
gitleaks CLI. They are wrong for shhh, which wants a zero-dep
auditable scanner whose binary fits in a screenshot.

**Decision:** reverted the import (`go mod edit -droprequire`, deleted
`go.sum`, confirmed build OK) and took a curated-transcription
approach instead — copy the subset of gitleaks' anchored-prefix
rules into our `rules.go` by hand.

**Built:**
- Transcribed **24 new anchored-prefix rules** from gitleaks v8.30.1's
  `config/gitleaks.toml` into `internal/rules/rules.go`:
  - Identity/secrets: `1password-secret-key`, `age-secret-key`
  - VCS tokens: `github-app-token` (ghs_/ghu_), `github-refresh-token`
    (ghr_), `github-fine-grained-pat` (github_pat_), `gitlab-pat`
    (glpat-), `gitlab-ptt` (glptt-)
  - AI / ML: `huggingface-access-token` (hf_)
  - Chat/notifications: `slack-app-token` (xapp-), `slack-webhook-url`
  - Package registries: `npm-access-token` (npm_), `pypi-upload-token`
    (pypi-AgEIc...), `rubygems` (dropped — too generic to anchor
    safely at `rubygems_` prefix without a keyword filter)
  - Email / comms: `sendgrid-api-token` (SG.)
  - Data / analytics: `databricks-api-token` (dapi),
    `grafana-service-account-token` (glsa_)
  - Cloud / infra: `digitalocean-pat` (dop_v1_),
    `digitalocean-access-token` (doo_v1_), `doppler-api-token` (dp.pt.),
    `vault-service-token` (hvs.)
  - Project mgmt: `linear-api-key` (lin_api_), `notion-api-token` (ntn_),
    `postman-api-token` (PMAK-)
  - Commerce: `shopify-access-token` (shpat_),
    `shopify-shared-secret` (shpss_)
- **Updated `aws-access-key`** to cover the full gitleaks
  `aws-access-token` prefix set (`A3T[A-Z0-9]|AKIA|ASIA|ABIA|ACCA`)
  with the correct RFC 4648 base32 body charset (`[A-Z2-7]` — no
  0/1/8/9, which is the spec-correct alphabet that AWS actually uses).
- **Updated test fixtures** from `AKIA3EXAMPLE7XYZABC1` →
  `AKIA3EXAMPLE7XYZABC4` in four places (detector_test,
  redactor_test, scanner_test, testdata/fixtures/leaky-project/.env)
  because the trailing `1` is not a valid base32 character and the
  stricter rule rejected it. `AKIAIOSFODNN7EXAMPLE` (AWS docs
  placeholder) is naturally base32-valid and still passes through
  the known-example allowlist, so task 8's FP test is unaffected.
- **`TestDetect_GitleaksDerivedRules`** — one assertion per new rule
  covering 24 fixtures, pinned so later edits cannot silently break
  any of the transcribed patterns.
- Package-level comment in `rules.go` credits gitleaks and explains
  the provenance (including the MIT license) so a future reader is
  not mystified by the `// from gitleaks:` markers.

**Learned:**

1. **The cost of `go get` is mostly invisible until you run it.**
   I expected gitleaks to pull ~10 deps. Actual was 64. The heavy
   hitters are not the obvious ones (viper/cobra are expected) —
   they are wazero+go-re2 and the archive-format fan-out from
   `mholt/archives`. This is the kind of fact you cannot reason
   about from docs; you have to add the module and look. Lesson:
   for any library of non-trivial scope on a zero-dep project, the
   first step is "import it, read `go.mod`, then decide."

2. **Gitleaks rules split cleanly into two populations: anchored
   prefix rules and context-dependent rules.** The anchored ones
   (`ghs_`, `glpat-`, `pypi-AgEIcHlwaS5vcmc`, `hvs.`) are plain
   `\bPREFIX[charset]{N}\b` regexes that transcribe into our
   `Rule` struct with zero changes. The context-dependent ones
   (`(?i)[\w.-]{0,50}?(?:airtable)(?:[ \t\w.-]{0,20})[\s'"]{0,3}...`)
   look for a service name near an assignment operator, with a
   capture group for the actual token. Those need a keyword
   pre-filter (gitleaks uses aho-corasick) and a capture-group API
   to carve out the real secret span. Our detector has neither.
   Transcribing those as-is would produce high FP rates because
   every string in a config file matches "40 alphanumeric chars
   near the word `alibaba`." Skipped them entirely. About ~60% of
   gitleaks' rules are in this category.

3. **The RFC 4648 base32 alphabet matters.** gitleaks uses
   `[A-Z2-7]` for AWS key bodies because AWS access keys really
   are base32. Our prior rule used `[A-Z0-9]`, which is slightly
   looser. Loose patterns would be the safer default if the goal
   were "never miss a real key," but in practice AWS keys never
   contain 0/1/8/9, so the tight pattern drops exactly zero real
   keys while gaining explicit documentation of "we know the
   format." The fixture update cost was four one-line edits.

4. **`TestDetect_PublicExampleCorpus` and task 8 did not regress
   despite adding 24 new rules.** That is the most important
   empirical result of this step: widening coverage did not cost
   us any FP-rate ground. The anchored-prefix discipline is load
   bearing — every new rule carries a ≥3-character fixed prefix
   that is extraordinarily unlikely to appear by accident in
   non-secret content. The same calibration work that entry 2 did
   for the hand-written rules pays off automatically for the
   transcribed ones. If we had instead adopted the context-
   dependent rules, task 8 would almost certainly have failed.

5. **"Rule retirement" is the wrong mental model for this step.**
   The roadmap "Teaches" column asked which hand-written rules
   gitleaks could replace. Answer: none, cleanly. gitleaks does
   have equivalents for most of our 15 (stripe, github-pat,
   github-oauth, google-api-key, anthropic, openai, jwt, etc.),
   but the overlap is "similar regex, same intent" — there is no
   DRY win unless we import gitleaks-the-library and let it
   manage its own rule set. Since we refused that import, the
   overlap cost stays zero. The real win is *addition*, not
   replacement.

**Decisions forced:**

- **Zero deps is a hard constraint, not a nice-to-have.** Adding
  gitleaks would have been a visible architectural departure
  from "auditable, small binary, screenshot-safe." Logged as an
  invariant future steps must respect. Any future "we should just
  import X" consideration needs to run `go get` first and count
  indirect deps before the discussion happens. The rule from here
  on: **Phase 0 ships with exactly the standard library.**
- **Rule additions require citation + anchored prefix.** Same
  discipline as the known-placeholder allowlist (entry 3). Every
  new rule must have a `// from gitleaks:` or `// from <doc>:`
  comment in the source and a `\b`-anchored unique prefix. No
  context-dependent rules without a keyword pre-filter design
  first.
- **Task 8 stays as the calibration gate for rule additions.**
  Future rule additions must prove zero-FP on `eval-corpus/public-
  examples/` before landing. The corpus itself can grow, but the
  contract is: adding content to the corpus or rules must never
  break the gate.

**Tech debt logged:**
- ~60% of gitleaks' rule set (the context-dependent family) is
  unreachable for shhh today. To pick those up we would need a
  keyword pre-filter + capture-group extension to `Rule`. Worth
  doing if eval tasks ever reveal a *specific* missed secret that
  gitleaks catches via that path; not worth doing speculatively.
- The transcribed rules drift from gitleaks master whenever the
  upstream TOML changes. We have no mechanism to re-sync. An
  annual manual re-sync pass against `config/gitleaks.toml` is
  probably fine given the rate of change.
- gitleaks' rule `vault-service-token` has an `s\.[a-z0-9]{24}`
  alternative we did not transcribe because it is too generic.
  If Vault's `s.`-prefixed tokens become the dominant form in the
  wild, we will need to pair the rule with a keyword filter.

**Next:** roadmap steps 14 & 15 (task 2 — connection-string diff at
query-param granularity, and task 9 — URL mismatch detection). Both
depend on the wider detector from this step. The new rules do not
directly help URL/query-string handling, but the "rule additions
without calibration regressions" discipline proven here is what
task 2 will exercise on a different axis.

---

## Entry 9 — Task 5 grep-hardcoded + grep_hardcoded tool (roadmap steps 11, 12)

**Goal:** land the first task where `redact+rehydrate` mode *passes*
without needing a compensatory tool, and measure whether the
`grep_hardcoded` tool is still worth shipping.

**Built:**
- `internal/eval/tasks/t05_grep_hardcoded.go` — task 5. Creates a
  temporary directory with five files (three containing the same
  hardcoded Stripe-live key, two decoys). Simulates an agent reading
  one entry-point file, extracting the placeholder, and trying to
  locate every other file that contains the same secret. Cleans up
  the tempdir with `defer os.RemoveAll`.
- `internal/eval/compensatory.go` — `GrepHardcoded(placeholder, dir)`
  resolves the placeholder via the session map, walks the directory,
  returns sorted file paths whose content contains the raw value.
  Fail-closed on unknown placeholders. Walk errors swallowed (an
  agent would interpret them as "no hits" anyway).
- `internal/eval/compensatory_test.go` — `TestGrepHardcoded` pins two
  hits in a tempdir fixture, confirms sort order, confirms decoys
  are excluded, confirms unknown placeholders return nil.
- `cmd/shhh-eval/main.go` — task 5 wired into the suite between task
  3 and task 7.

**Matrix after this entry:**

```
task                                 baseline  redact    redact+rehyd  +compen
[T1] t01-jwt-decode                  PASS      FAIL-OK   FAIL-OK       PASS
[T1] t03-consistency                 PASS      FAIL-OK   FAIL-OK       PASS
[T1] t05-grep-hardcoded              PASS      FAIL-OK   PASS          PASS
[T2] t07-placeholder-entropy         -         PASS      -             -
[T3] t08-public-corpus               -         PASS      -             -
```

The `redact+rehyd` cell of t05 is the first `PASS` in that column.
That is the whole point of the task.

**Learned:**

1. **Rehydration-in-tool_use-args is the right layer for search-like
   tasks.** When the agent wants to do something *with* the value
   rather than reason *about* it, the value has to end up on the
   other side of a tool boundary anyway. Task 5's grep is the clean
   case: needle goes into a tool arg → proxy rehydrates → tool runs
   against raw bytes → results come back → PostToolUse redaction
   rewrites matches into consistent placeholders. Task 1 (JWT
   claims) is the opposite: the agent needs to *understand* the
   value mid-reasoning, with no tool call on the critical path.
   The same rehydration layer cannot help. This is the empirical
   distinction the PRD hinted at and now the matrix shows
   side-by-side.

2. **`grep_hardcoded` is a session-boundary refinement, not a
   capability unlock.** If rehydrate+bash already covers the task,
   why ship the tool at all? The answer is not "because it unblocks
   anything the rehydrate path can't do." It is:
     a) The rehydrate path sends the raw value to a shell command.
        Shell history, process listings, and stray logging can leak
        it. `grep_hardcoded` keeps the value inside the daemon.
     b) Agent UX: one MCP call with a placeholder ID is easier to
        reason about than constructing a grep invocation where the
        needle is opaque.
     c) Deterministic result format. The shell output shape varies
        across grep implementations; the tool returns a typed list.
   None of these reasons are "task 5 fails without it." The roadmap
   previously implied they might be — they are not. Corrected in
   the step-11 notes.

3. **Task-5 redact-mode failure is trivial and deserves a one-line
   comment in the log.** The agent greps the placeholder string;
   the filesystem contains raw bytes; zero hits. This is obvious in
   hindsight but still worth writing down because "placeholder ≠
   disk bytes" is the single fact that makes the three redact-mode
   designs (rehydrate args, re-redact results, compensatory tool)
   even necessary. Without that failure mode, none of the Phase-3
   architecture would matter.

4. **`GrepHardcoded`'s "swallow walk errors" decision is load-
   bearing.** If the walk returns an error partway through (a file
   whose permission bits flipped, a broken symlink), the tool could
   surface it to the caller. But then an agent would have to distinguish
   "no hits" from "error during walk" and do some recovery dance.
   The current behavior returns whatever hits were found before the
   error, which matches the UX of `grep -r` on a real shell
   (prints partial results, exits non-zero). Fail-open on walk
   errors is consistent with the tool's role as "best-effort
   search, not an audit."

**Decisions forced:**

- **Phase 3 does NOT need PostToolUse redaction as a blocker for
  task 5.** Rehydrate-in-tool_use-args alone suffices. PostToolUse
  is still motivated by other tasks (task 6 prompt injection, task
  10 tool_use round-trip) but task 5 is no longer on its critical
  path. Roadmap step 11's "Teaches" column updated to reflect this.
- **`GrepHardcoded` ships anyway.** The three reasons in "Learned"
  point 2 together justify the tool: session-boundary containment,
  UX, deterministic shape. Roadmap step 12 moves to `done`.
- **Fixture lives on real disk, not an in-memory filesystem.** Go's
  `io/fs` has `fstest.MapFS` but `GrepHardcoded` takes a directory
  path because that is what the real tool will accept from MCP
  callers. Using a real tempdir keeps the Phase-0 test exercising
  the same codepath the Phase-3 tool will. The cost is an
  `os.MkdirTemp` + `defer os.RemoveAll` per run, which is cheap.

**Tech debt logged:**
- `GrepHardcoded` does whole-file `strings.Contains` over every file
  in the tree. For a large repo this is expensive — the real
  implementation should at least skip binary files and have a size
  cap (the scanner already has these). Factoring the scanner's
  `isText` + `maxFileBytes` gate into a shared helper the
  compensatory tool can reuse is a one-hour job once task 5 proves
  the tool's shape. Not now.
- The grep simulation inside the task (`grepDir`) duplicates the
  walk logic from the tool. Both could share a single
  `filesContaining` helper. They currently do, through
  `t05_grep_hardcoded.filesContaining`, but the tool keeps its own
  copy in `compensatory.go` so the tool-layer code has no package
  dependency on the task layer. Worth a shared internal helper if
  a third caller appears.

**Next:** roadmap step 13 (Gitleaks library integration). Now that we
have three Tier-1 tasks empirically validating the detection pipeline
+ compensatory-tool pattern, widening detection coverage via gitleaks
is a mechanical improvement that doesn't risk breaking the current
story. Steps 14–15 (URL/query-param tasks) depend on the wider rules.

---

## Entry 8 — Task 3 consistency-across-files + base64-aware compare_secrets (roadmap steps 9, 10)

**Goal:** land the first cross-file identity task and the compensatory
tool it needed, and decide whether base64 canonicalization lives in the
detector or in the compensatory tool.

**Built:**
- `internal/eval/tasks/t03_consistency.go` — task 3. Builds three
  fixtures in memory: a `.env` file, a Go source file with a hardcoded
  `const`, and a Kubernetes-style Secret manifest with the secret
  encoded as base64. All three are redacted in the same session. The
  simulated agent extracts the value observable at each site and asks
  whether the three refer to the same underlying secret.
- `internal/eval/compensatory.go` — `CompareSecrets` enhanced with a
  cross-representation equality rule. If byte equality fails, try
  base64-decoding either side (both `StdEncoding` with padding and
  `RawStdEncoding` without). This is the task-3 unblock; without it
  the compensatory cell would also fail.
- `internal/eval/compensatory_test.go` — pins the three shapes
  (raw==raw, raw==base64-raw, raw==base64-padded) and fail-closed on
  unknown placeholders.
- `consistencySecret` is a 40-byte `sk_live_…` key whose raw form hits
  the `stripe-live-key` pattern rule and whose base64 form (54 chars,
  36 distinct, entropy 4.96) hits the entropy detector without any
  calibration changes. Picked deliberately so the task exercises two
  different detection paths without new rule work.
- `cmd/shhh-eval/main.go` — task 3 wired into the suite between task 1
  and task 7 so the matrix reads top-to-bottom in Tier order.

**Matrix after this entry:**

```
task                                 baseline  redact    redact+rehyd  +compen
[T1] t01-jwt-decode                  PASS      FAIL-OK   FAIL-OK       PASS
[T1] t03-consistency                 PASS      FAIL-OK   FAIL-OK       PASS
[T2] t07-placeholder-entropy         -         PASS      -             -
[T3] t08-public-corpus               -         PASS      -             -
```

Both Tier-1 tasks now follow the "design held" story — pure redaction
breaks reasoning, compensatory tools unblock.

**Learned:**

1. **Base64 canonicalization belongs in the compensatory tool, not the
   detector.** The roadmap flagged this as the open question: should
   the detector recognize "this 54-char base64 blob is the encoded form
   of some raw value we already hashed," or should the tool handle it
   at lookup time? The detector path would require it to keep a
   rolling map of every value it has seen in the session and try
   base64-decoding every high-entropy token against that map. That
   destroys statelessness: the detector's only input today is the
   current content buffer. Pushing the logic into `CompareSecrets`
   keeps detection stateless and localizes the "agent needs to
   compare" logic in the place the agent is already calling.

   The practical cost is that an agent doing free-form reasoning
   ("does this key look the same?") still can't tell — but a
   free-form agent has the raw bytes in its context anyway. The tool
   call is the boundary where cross-representation equality is
   actually needed.

2. **The session map's deduplication by byte-identity is the
   load-bearing assumption.** When the `.env` and Go files both
   contain the literal `sk_live_AbC7…`, the session map returns the
   *same* placeholder for both. That is what makes the 1-vs-2
   comparison trivially pass in redact mode. If the session map keyed
   on something lossier (e.g., a normalized form), the test for
   "placeholder identity reflects real identity" would be weaker. Log
   this as a Phase-0 invariant: placeholder identity == byte identity
   of the pre-redaction value.

3. **Padded vs unpadded base64 matters at ingestion time but not at
   comparison time.** The detector's token regex excludes `=` so
   padded base64 tokens lose their `==` when substituted — the
   placeholder is clean, the trailing `==` stays in the surrounding
   content. The compensatory tool accepts both padded and raw std
   encodings when decoding. The fixture uses `RawStdEncoding` to
   sidestep the extraction mess, but the tool would still work on a
   real kubectl dump with `==` padding because the map lookup finds
   the cleanly-substituted raw-encoded form after the regex has
   stripped the padding from the placeholder boundary.

4. **t03 needed no changes to the detector, scanner, rules, or
   session map.** That is the cleanest kind of task landing: the
   product surface expanded because one compensatory tool got one
   additional line of logic. The PRD's §7.7 framing — "compensatory
   tools as a first-class surface" — is doing the work it claimed.

**Decisions forced:**

- **`compare_secrets` knows about base64. Future compensatory tools
  may know about other common encodings.** Not URL-encoding yet
  (haven't seen a task that needs it), but the pattern is: when a
  new encoding shows up in a failing task, extend the tool, not the
  detector.
- **`rawEqualWithBase64` is duplicated in the task** for the
  baseline mode comparison. This is intentional: the compensatory
  tool's `CompareSecrets` needs a session map, but baseline mode has
  no placeholders to resolve. The two codepaths do the same base64
  work because they are answering the same question against
  different inputs (resolved values vs raw strings). Factoring a
  shared helper is possible but would require exporting the
  base64-equality primitive from `eval` as a standalone function; not
  worth the surface area for one caller pair.
- **Fixture uses `RawStdEncoding`, tool accepts both.** Keeps the
  fixture extraction simple (no trailing `=`) while preserving
  realism at the tool boundary (real k8s secrets are padded).

**Tech debt logged:**
- t03's `extractYAMLValue` is not a real YAML parser. It handles
  exactly the key:value-on-one-line shape the fixture uses. When
  task 4 (Edit/Write round-trip) or future tasks need a YAML fixture
  with block scalars, nested keys, or quoted strings, this helper
  needs to be replaced or supplemented. Not now.
- `rawEqualWithBase64` in t03 mirrors `base64Equals` in
  `compensatory.go`. If a second task needs baseline-mode base64
  equality, promote the helper to a shared internal function.

**Next:** roadmap step 11 (task 5 — grep for hardcoded secret). Task 5
tests whether an agent can find *new* occurrences of a known secret
outside the files it has already seen, which is the first real demand
for either PostToolUse redaction consistency (not built) or the
`grep_hardcoded` compensatory tool (step 12). Per the roadmap, step 11
directly informs Phase 3 architecture.

---

## Entry 7 — Code-review cleanup pass (no roadmap step)

**Goal:** act on a focused code review of the entry-6 checkpoint before
starting step 9. The review surfaced two correctness gaps in the scanner,
one piece of tech debt that was cheaper than the entry-6 note claimed,
and a handful of comment/helper bit-rot items.

**Built:**
- `redactor.Redactor.Resolve(placeholder)` — thin pass-through to
  `session.Map.Resolve`, which has been public the whole time. The
  `ShhhAdapter.ResolvePlaceholder` rehydration-of-a-single-string kludge
  is gone; the adapter now does a direct map lookup.
- `scanner.Scan` now iterates the cross-reference value set in sorted
  order *and* re-sorts the merged findings by offset before returning
  them. Two runs of `make bench` on the same fixture now produce
  identical finding orders. This was protecting the step-4 ship
  criterion more than the original cross-ref code was.
- `scanner.Scan` now reports *every* occurrence of a cross-referenced
  value in a file, not just the first. The previous `strings.Index`
  call capped the report at one per file even if the value was
  copy-pasted three times. Multi-occurrence is a loop over
  `strings.Index` with a moving cursor.
- `JWTClaims.Audience` is now `[]string` with a custom `UnmarshalJSON`
  that accepts both the single-string and string-array encodings
  permitted by RFC 7519 §4.1.3. Real IdPs ship array audiences and the
  entry-6 implementation would have silently returned `ok=false` on
  first contact with a Google or Auth0 token.
- `runner.Matrix` switched from ✅/❌/⚠️ emoji cells to ASCII labels
  (`PASS` / `FAIL-OK` / `REGRESS` / `SURPRISE`). `%-Ns` width
  specifiers count bytes, not terminal columns, so emoji cells were
  rendering 2–3 columns wider than ASCII cells and the matrix came out
  ragged. The mode-column width is bumped to 12 so `redact+rehyd` (12
  chars) fits cleanly.
- Detector sort switched from `sort.Slice` to `sort.SliceStable` so
  the dedupe comment's claim ("rule match wins because it's earlier in
  the slice") is actually guaranteed rather than accidental.
- Deleted the hand-rolled `itoa` in `adapter_shhh.go` (and its twin in
  `t07_placeholder_entropy.go`) plus a `indexString` reimplementation
  of `strings.Contains`. The comment justifying them ("keep this file
  dependency-free") was wrong — `strconv` and `strings` are stdlib.
  `t08_public_corpus.go` had been relying on t07's package-level
  `itoa`; now both use `strconv.Itoa`.
- Stale comment in `t08_public_corpus.go` removed: it still referenced
  the AWS-placeholder carve-out that entry 3 eliminated.
- `cmd/shhh-eval` help text updated to acknowledge task 1 (it still
  said "Phase 0 ships task 7 and task 8 only").
- Roadmap step 10 marked `partial`: the `CompareSecrets` method exists
  on `CompensatoryTools` from entry 6, but the task-3 use-site is
  still step 9's job.
- `Cell.Matches()` removed — it was referenced in exactly one place
  (the mismatch loop) where `IsRegression() || IsSurprisePass()` is
  the same expression. Three overlapping predicates muddied the
  mental model of what a Cell "means"; two is enough and matches the
  four-outcome legend verbatim (the two matching-design outcomes do
  not need their own method because nothing in the runner ever asks
  "did this match?" — it only asks "is this a regression or a
  surprise?").
- Tests added:
  - `TestScanEnvCrossReference_MultipleOccurrences` pins the new
    multi-occurrence scanner behavior and asserts file-order sorting.
  - `TestDecodeJWTPayload_AudienceShapes` pins the
    string/array/absent decoding of `aud`. Single-string tokens
    normalize to a one-element slice; array tokens pass through;
    absent audiences leave the field nil.

**Learned:**

1. **The entry-6 "tech debt — resolve via rehydration" note was wrong
   about the fix cost.** I had filed it as "waits for Phase 3 daemon
   direct lookup." In reality `session.Map.Resolve` had been public
   the whole time and the only missing piece was a 3-line re-export on
   `Redactor`. Lesson: when filing tech debt, grep for the primitive
   you claim is missing before writing the note. I made the claim
   plausible by phrasing it in terms of a future daemon API instead of
   reading the current session map surface.

2. **Scanner determinism matters more than it looks.** The
   cross-reference append was iterating a `map[string]struct{}`, whose
   iteration order is randomized by the Go runtime on each call. The
   scan output looked stable in practice because the test fixture only
   has one cross-ref value, but any project with two or more would
   have produced different orderings across runs — and `make bench`'s
   ship criterion is exact numerical reproducibility. This is the kind
   of bug that only bites at the exact moment it matters most.

3. **`strings.Index` alone is not a multi-occurrence search.** The
   cross-ref pass used `strings.Index` as if it reported presence,
   which it does, but the feature's intent is "find *every* hardcoded
   copy." Reviewing the *feature* (not just the code) caught this;
   reviewing only the code would not have, because the code was
   internally consistent.

4. **ASCII column alignment is free; emoji column alignment is not.**
   Emoji glyphs occupy 2 terminal columns but 3–4 bytes. Go's
   `fmt.Sprintf("%-14s", ...)` pads to 14 bytes. Result: emoji cells
   lean 2 columns wider than plain cells. Either use a runewidth-aware
   formatter (dependency) or switch to ASCII labels (free). The ASCII
   labels also read better in CI logs and grep output.

**Decisions forced:**

- **`JWTClaims.Audience` is always `[]string` in-memory.** Single-audience
  tokens are normalized to a one-element slice in `UnmarshalJSON`.
  Accepting both shapes at the field type level (e.g., `interface{}`)
  would push the branching into every consumer; normalizing at the
  decode boundary keeps `rubric()` and compensatory tool callers
  dealing with exactly one shape.
- **Matrix labels are ASCII, not emoji.** The design-held story is
  still preserved (`PASS` vs `FAIL-OK` encodes the expected-outcome
  distinction). Emoji were tempting but they cost column alignment and
  grep ergonomics for no additional signal.
- **Scanner cross-ref is now O(n_files × n_values × content_length).**
  Iterating the value set in sorted order and looping over each value
  with a moving `strings.Index` is fine for Phase-0 fixtures but will
  need revisiting if the corpus grows. Logged but not actioned.

**Next:** roadmap step 9 (task 3 — consistency across files). The
scanner is now deterministic enough to trust for cross-file findings,
which is what task 3 will stress.

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
