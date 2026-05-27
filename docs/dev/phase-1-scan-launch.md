# Phase 1 — Public Launch: Scan + Eval

**Duration:** 3 weeks
**Visibility:** Public (this is the launch)
**Prerequisite phase:** Phase 0 complete with baseline numbers in hand

## Goal

Ship `npx shhh scan .` publicly, and at the same time publish the `shhh-eval` repository with the complete Phase 0 baseline results, methodology, and honest discussion of what shhh does and doesn't do.

## Why this phase exists

The viral acquisition strategy needs a hook: a zero-install command that produces an immediately tweetable, scary-but-respectable screenshot. `shhh scan` is that hook.

But the original PRD tried to launch with *only* the scan, saving the credibility work for later. The critique made clear this is backwards: the scan gets you stars, but it doesn't get you the serious developers who decide whether to install in production. The serious developers read a README carefully, look for evidence of quality, and churn immediately if they find marketing where they expected substance.

The launch narrative that works for both audiences is: **"I built a redaction tool and here's how I measured whether it actually works."** The scan is the hook, the eval is the proof, and publishing them together is the differentiator. No other tool in this space has a public eval suite.

## Deliverables

### `shhh scan` polish

- [ ] Terminal-colorized output matching the §6.1 spec from PRD.
- [ ] **Screenshot-safe default.** Usernames, hostnames, and connection-string internals are masked with `•••`. Any metadata that could embarrass a user who tweets the output is hidden by default.
- [ ] `--show-details` flag: full output for local inspection. Never the default. Scary warning in the output when used.
- [ ] `--format json` (machine-readable).
- [ ] `--format md` (Markdown report for PR comments and docs).
- [ ] Performance: scan of a typical Node.js project (~2000 files) completes in under 3 seconds on a modern laptop.
- [ ] Skip known safe directories: `node_modules`, `.git`, `vendor`, `__pycache__`, `.venv`, `dist`, `build`, `target`.
- [ ] Handle large repos gracefully: do not OOM on a repo with 100MB of `.log` files.
- [ ] Exit code: 0 if no secrets found, 1 if secrets found (CI-friendly).

### Cross-platform npm distribution

- [ ] Platform-specific Go binaries for darwin-amd64, darwin-arm64, linux-amd64, linux-arm64, windows-amd64.
- [ ] npm package using the esbuild-style distribution pattern: a `postinstall` script (or lazy `bin` shim) selects the right binary for the host platform. **Budget one full week for this** — the esbuild pattern is non-trivial and easy to get wrong.
- [ ] `npx shhh scan .` works end-to-end on all five platforms.
- [ ] Homebrew tap (`Musubi42/tap`) with a `shhh` formula.
- [ ] Curl installer script at `https://shhh.dev/install.sh` for Linux servers and CI.
- [ ] `go install github.com/Musubi42/shhh@latest` works for Go developers.
- [ ] GitHub Releases with signed binaries for manual download.

### `shhh-eval` publication

- [ ] Repository made public: `Musubi42/shhh-eval`.
- [ ] MIT license.
- [ ] `README.md` explaining what the eval measures, what the 40-cell matrix means, and how to run it on a fresh machine.
- [ ] `make bench` reproduces the full matrix.
- [ ] `docs/eval-results.md` — the 40-cell matrix with bars, confidence intervals, and brief discussion of what each task means. **Including the cells where shhh fails.**
- [ ] Redactor interface documented as a standard, with a reference implementation and an example stub for third-party redactors.
- [ ] Open invitation in the README: "Want Rehydra / contextio / your tool benchmarked? Open a PR with a conforming adapter."

### `shhh` repository publication

- [ ] Repository made public: `Musubi42/shhh`.
- [ ] MIT license.
- [ ] `README.md` matching the PRD §9.2 structure:
  - [ ] Above-the-fold: name, tagline, one-liner install, masked scan screenshot.
  - [ ] Compatibility line in the first 3 lines.
  - [ ] Scan command before install section.
  - [ ] `<details>` blocks for each tool with Claude Code open by default.
  - [ ] Link to the eval results prominently.
- [ ] `docs/threat-model.md` — the four threats from PRD §1, what shhh defends against, what it doesn't, and a link to the eval task 6 results showing real-value leakage rate.
- [ ] `CONTRIBUTING.md` — how to add detection rules, how to propose eval tasks.
- [ ] Issue templates: false positive, false negative, detection request, eval task proposal.

### Launch content

- [ ] Show HN post, framed around the eval, not around "secrets are scary."
- [ ] Twitter/X thread: "I built a secret-redaction tool for AI coding agents. Here's the benchmark I built to measure whether it actually works, and here's where it fails."
- [ ] Product Hunt submission.
- [ ] Blog post on a personal or company blog with the full methodology.
- [ ] Comment on Claude Code issue #44868 linking shhh as a candidate solution (not as "the solution").
- [ ] Demo video: 60 seconds, terminal only, scan → see output → read the eval results. No voice-over needed.

## Ship criterion

Two tests, both must pass:

1. **The drive-by test.** A developer who sees a tweet with the screenshot can run `npx shhh scan .` in one command, get a screenshot-safe output, and understand what it means without reading any documentation. Time from tweet to screenshot: under 60 seconds.

2. **The skeptic test.** A senior engineer who doesn't trust marketing can clone `shhh-eval`, run `make bench` on their machine, and reproduce our published numbers (within confidence intervals) without asking us any questions. Time from skepticism to verified numbers: under 30 minutes.

If either test fails, Phase 1 is not done.

## Explicitly out of scope for Phase 1

- Any hook integration. The scanner is standalone.
- Proxy daemon. Nothing runs in the background.
- MCP server. No IDE integration yet.
- `shhh init` TUI. That's Phase 5.
- `shhh status`, `shhh logs`, `shhh doctor`. Nothing to monitor yet.
- `shhh install <tool>`. Nothing to install yet.
- Teams / enterprise features. Phase 6 at earliest.
- Telemetry beyond anonymous scan counter (and only if it meaningfully informs Phase 2 decisions).
- Detection rule customization via `shhh add-rule`. Phase 5.
- Any paid or commercial features.

## Phase-specific risks

| Risk | Severity | Response |
|------|----------|----------|
| npm cross-platform distribution eats more than one week | High | Time-box the esbuild pattern to 7 days. If not done, fall back to a simpler install (direct curl installer + Homebrew) and ship `npx shhh` in Phase 1.5. |
| A false positive in the scan lands on a popular repo's screenshot on Twitter | High | Task 8 (public-example corpus) must be at 100% pass before public launch. Add a conservative default mode that errs on silence. |
| Eval results are embarrassing on a Tier 1 task | Medium | Publish anyway with an honest discussion of why and what Phase 3+ will fix. This is what credibility looks like. |
| Launch discussion focuses on "you're attacking Anthropic" instead of the actual product | Medium | Frame every piece of launch content around the eval, not around provider criticism. The threat model doc explains the actual threat without pointing fingers. |
| Name collision on npm / GitHub / domain | Medium | Resolve the name question (PRD §11.1) *before* Phase 1 starts. Cannot rename after launch without losing the star history. |
| Prompt-injection task 6 results generate "you're publishing attacks" criticism on HN | Low | The attacks are already in public literature (Simon Willison's research). Put the payloads behind `docs/threats/` with a clear `THREATS.md` explaining why, and a `security-tier` tag separate from the capability tier. |

## Dependencies

- Phase 0 complete.
- `shhh-eval` baseline numbers exist and are credible (not placeholder).
- Name and domain resolved.
- Detection rules at <5% false-positive rate on task 8.
- Prompt-injection task 6 shows zero real-value leakage in the full mode.

If any dependency is not met, Phase 1 does not start. The right thing is to extend Phase 0, not to launch with weak numbers.
