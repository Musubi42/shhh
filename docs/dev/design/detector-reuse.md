# Should shhh reuse an open-source secret scanner?

## The question

shhh has its own detector (`internal/detector`) with ~39 named rules
plus a Shannon-entropy fallback. Building and calibrating this took
real work. Gitleaks, Trufflehog, and detect-secrets already exist,
are well-maintained, have hundreds of rules, and solve most of the
"what patterns look like secrets" problem better than we ever will.

**Should we delete our detector and import one of theirs?**

This doc is the considered answer so we don't re-debate it every
time a false positive lands.

## The candidates

| Tool | Lang | License | Approach | Stars |
|------|------|---------|----------|-------|
| [gitleaks](https://github.com/gitleaks/gitleaks) | Go | MIT | Regex rules + entropy, git-aware | ~17k |
| [trufflehog](https://github.com/trufflesecurity/trufflehog) | Go | **AGPL-3** | Regex + live credential verification via API calls | ~17k |
| [detect-secrets](https://github.com/Yelp/detect-secrets) | Python | Apache-2.0 | Plugin-based detectors, git-aware baselines | ~4k |
| [ggshield](https://github.com/GitGuardian/ggshield) | Python | MIT | Wraps GitGuardian cloud API | ~1.8k |

Two (trufflehog, ggshield) call out to networks as part of their
value proposition. That is immediately disqualifying for shhh: we
promise "nothing leaves your machine," and shipping a detector that
phones home — even to verify credential validity — contradicts the
product thesis.

That leaves **gitleaks** (Go, MIT, in-process importable) and
**detect-secrets** (Python, requires a subprocess) as the real
candidates.

## What shhh actually needs from a detector

Before comparing, it is worth writing down what the job is. A lot of
the "gitleaks vs ours" debate collapses once the requirements are
explicit.

1. **Latency budget ≤ ~20 ms** per invocation. The hook runs on every
   `Read` tool call inside Claude Code. Anything above ~50 ms starts
   to feel like a stutter to the user.
2. **Offset-accurate findings** — not just "does this file contain a
   secret?" but `[start, end)` spans so the redactor can replace
   them in place without corrupting the surrounding text.
3. **Typed labels** — our product returns
   `[STRIPE_LIVE_KEY:sk_live_...:hash]`, not `[SECRET:hash]`. The
   label comes from the rule that matched, and the `PublicPrefix`
   (`sk_live_`) tells the LLM what *kind* of secret was there
   without revealing the value.
4. **Session-stable placeholder hashes** — the same secret in the
   same session must redact to the same placeholder, so the LLM can
   reason across multiple Read calls.
5. **Structural descriptors for URL-shaped secrets** — a postgres
   connstring redacts to `admin@prod-db:5432/myapp`, not just
   `[POSTGRES:hash]`, because the LLM needs to see the host/port/db
   to reason about the architecture. This is what `Rule.Normalize`
   does in our rules package.
6. **Public-example allowlist** — `AKIAIOSFODNN7EXAMPLE` from AWS
   docs must not fire. Tuned once, pinned with tests.
7. **No-false-positive calibration for agent context** — agents
   generate tool-use IDs, git SHAs, UUIDs, session IDs constantly.
   The detector must skip them or the LLM drowns in `[SECRET:...]`
   noise. This is exactly what entry 10 of the implementation log
   calibrated against.

Our detector does 1–7 directly. Any external library we adopt has to
match them, not just "find secrets."

## Gitleaks as a library: the honest evaluation

Gitleaks v8 exposes `github.com/zricethezav/gitleaks/v8/detect` as
an importable Go package. `detect.Detector` has a `Detect(fragment)`
method that returns `[]report.Finding` with byte offsets, rule IDs,
and matched values. That covers requirements 1 (in-process, fast),
2 (offsets), and partially 3 (rule IDs, but not typed labels).

What we would gain by importing it:

- **Rule coverage.** Gitleaks ships ~170 rules vs. our ~39.
  Immediate coverage of Slack, Twilio, SendGrid, Mailgun, Okta,
  Asana, Atlassian, Contentful, Discord, Dropbox, Datadog, Facebook,
  Firebase, Heroku, Intercom, Lob, Mailchimp, Messagebird,
  New Relic, PagerDuty, Pivotal, Square, Twitch, Zendesk, etc.
- **Maintenance.** When Stripe rotates their key format or GitHub
  introduces a new token prefix, gitleaks absorbs the update and we
  inherit it. Today we would have to transcribe each rule ourselves
  (we already did this once in implementation-log entry 10).
- **Ruleset authority.** "Why do you flag this as a Stripe key?"
  is easier to answer with "that's the gitleaks rule" than with
  "we wrote a regex."

What we would lose or complicate:

- **Typed labels / `PublicPrefix`.** Gitleaks findings have a
  `RuleID` like `stripe-access-token` but no structured
  `PublicPrefix` field. We would need a mapping table
  `{gitleaks-rule-id → (label, public-prefix)}` and keep it in
  sync when gitleaks adds or renames rules. This is a new
  maintenance surface.
- **`Normalize` hooks.** The postgres-connstring "structural
  descriptor" logic has no gitleaks equivalent. It would stay in
  our code anyway, layered on top of gitleaks findings.
- **Calibration ownership.** Our false-positive tuning
  (`distinctChars < 18`, `integrityPrefixes`, the new 3+ slash
  path-skip) is agent-specific. Gitleaks is tuned for "scanning
  git history for leaks a human should rotate," which is a
  different cost function — it is allowed to be noisy because a
  human reviews the output. We are not; the LLM reads the
  narration without review, so every false positive we miss turns
  into user-facing noise.
- **Transitive dependencies.** Gitleaks pulls in its own CLI
  scaffolding, viper/cobra config, git libraries, etc. As a Go
  module dependency we would inherit the compile-time closure.
  `go mod why` says roughly "a lot."
- **Latency ceiling.** Gitleaks' rule engine is fast, but ~170
  regex passes per file is more work than our ~39. Needs
  benchmarking before committing.
- **The postmortem risk.** CLAUDE.md hard rule 4 is "beware
  elaboration bias — does this bring the demo closer?" Swapping
  the detector does not bring the demo closer; it swaps one
  working thing for another working thing with a different
  tradeoff profile. That is exactly the kind of move the
  postmortem calls out.

## Recommendation

**Do not swap today. Keep our detector, but adopt gitleaks'
ruleset as a source-of-truth feed.** Specifically:

1. **Keep `internal/detector` as the runtime engine.** Latency,
   typed labels, `Normalize` hooks, calibration — all stay.
2. **Continue transcribing gitleaks rules when real usage surfaces
   a missed provider.** This is what we already did in log entry
   10 for github-app-token, slack-app-token, etc. It is a lazy
   pull, not a push: a user hits a missing secret type → we check
   if gitleaks has a rule → we transcribe it with a fixture.
3. **Write a `scripts/compare-gitleaks.sh`** (optional, later) that
   runs our detector and gitleaks over the same corpus and diffs
   the findings. Not for automated CI, but as a periodic audit
   tool to catch ruleset drift.
4. **Re-evaluate if the gitleaks rule feed becomes a bottleneck.**
   Concrete trigger: if we transcribe more than ~5 rules in a
   month because users keep hitting missed providers, it is
   cheaper to import the library and accept the tradeoffs above.
   Until then, the ~1 rule/month transcription cost is genuinely
   lower than the integration cost.

### What this means practically

- The `docs/design/vault/mockups/DESIGN-NOTES` false positive we
  just fixed was a calibration issue, not a rule-coverage issue.
  Gitleaks would not have helped — it has the same
  Shannon-entropy fallback with similar or worse tuning. The fix
  (`3+ slash skip` in entropy tokenizer) is strictly ours to make.
- The "which secret did shhh protect?" visibility work from the
  Eurio session is also orthogonal to the detector: it is about
  what metadata the hook surfaces in `additionalContext`, not
  what rules fired.
- The Read→Edit tracking break is unrelated to the detector.
  Swapping engines would not touch it.

None of the open problems on shhh's plate would be solved by
adopting gitleaks. That is the clearest evidence that the swap
would be elaboration, not progress.

## If we reopen this

The conditions under which this decision should be revisited:

1. **User-reported coverage gap.** More than 5 "shhh missed a
   real secret" reports in a month against a provider gitleaks
   already handles.
2. **Maintenance cost.** Transcribing rules eats more than one
   session per month.
3. **Ecosystem shift.** Gitleaks (or a successor) ships an
   agent-tuned ruleset with low-false-positive calibration
   explicitly for runtime interception. This does not exist
   today.
4. **Trufflehog re-licenses.** Unlikely, but AGPL is the only
   thing blocking trufflehog, and its live-verification feature
   would be genuinely interesting for an audit mode.

## References

- CLAUDE.md hard rule 4 (elaboration bias)
- `docs/postmortem-eval-overbuild.md` (why we don't over-build
  calibration)
- `docs/implementation-log.md` entry 10 (the "gitleaks
  transcription" decision — we already chose this path once)
- `internal/detector/detector.go` (the runtime engine)
- `internal/rules/rules.go` (where the transcribed rules live)
