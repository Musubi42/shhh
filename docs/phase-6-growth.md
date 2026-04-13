# Phase 6 — Growth & Ecosystem

**Duration:** Ongoing
**Visibility:** Public
**Prerequisite phase:** Phase 5 shipped, product is stable and feature-complete for individual developers

## Goal

Grow shhh from "a working product for individual developers" into an ecosystem: CI integration, team features, IDE extensions, and community-driven expansion of the detection pipeline and compensatory tool surface.

## Why this phase exists

By Phase 5, shhh is a complete product for its primary user (an individual developer running AI coding agents). Phase 6 is about expanding the addressable surface: teams, CI pipelines, IDE users who don't touch the terminal, security-conscious organizations that need audit trails, and the long tail of secret types and tools that only a community can cover.

This phase has no fixed duration. It is an ongoing stream of prioritized work driven by:

1. **Community feedback** — issues, PRs, Discord/Slack discussions, tweets.
2. **Eval-revealed needs** — new tasks that surface new failure modes.
3. **Ecosystem changes** — new AI tools launching, existing tools changing their APIs.
4. **Strategic pivots** — responses to Anthropic/OpenAI shipping native features, competitors launching, market shifts.

Every Phase 6 deliverable must clear two gates: **"does a real user want this?"** (cite an issue, a tweet, or a direct ask) and **"does shhh actively benefit from building it?"** (versus pointing the user to a specialist tool).

## Candidate deliverables, prioritized by category

### CI and automation

- [ ] `shhh scan --ci` mode: machine-readable output, exit code 1 on secrets found, configurable severity gates.
- [ ] GitHub Action: `musubi-sasu/shhh-action` — runs `shhh scan` on PR diffs and comments with findings.
- [ ] Pre-commit hook integration: a `shhh pre-commit` subcommand that can be dropped into `.pre-commit-config.yaml` or `husky`.
- [ ] GitLab CI template.
- [ ] Circle CI / Buildkite orbs.
- [ ] SARIF output format for GitHub code scanning and other security dashboards.

### IDE extensions

- [ ] VS Code extension: status-bar indicator showing "shhh: active" or "shhh: inactive," click to open the daemon status. No redaction logic in the extension itself — it's a UI for the existing daemon.
- [ ] JetBrains plugin: same scope as VS Code.
- [ ] Neovim plugin (Lua): status line integration for Vim users.

### Team and enterprise features

These are the point where monetization becomes plausible. The decision to build them is also the decision to have a commercial tier.

- [ ] Team configuration sharing: a mechanism for a team to define shared detection rules, ignores, and trust policies, distributed via a git repo or a hosted config service.
- [ ] Audit log export: structured logs of all interception events, suitable for SIEM ingestion.
- [ ] Team dashboard (hosted): aggregate metrics across a team's installs, detection rule coverage, most common secret types.
- [ ] SSO / SAML for the dashboard if the dashboard exists.
- [ ] SOC 2 path for the hosted service if there is one.

### Detection expansion

- [ ] Community-contributed detection rules in a separate repo with clear contribution guidelines and a review process.
- [ ] Version-pinned rule bundles so users can lock to a specific detection version for reproducibility.
- [ ] A public "rule request" issue template that links to the detection pipeline docs.
- [ ] Quarterly "rule drift" review: check that detection still catches currently-used secret formats as providers evolve them.

### Compensatory tool expansion

Each new compensatory MCP tool ships only if an eval task demonstrates it is needed. Candidates that may emerge:

- [ ] `describe_certificate(placeholder_id)` — returns issuer, subject, expiry for a PEM cert without exposing the private key.
- [ ] `compare_connection_strings(a, b)` — structural diff without exposing passwords.
- [ ] `validate_oauth_token(placeholder_id)` — checks format and expiry without exposing the token.
- [ ] `list_hardcoded_secrets(directory)` — audit-mode scan that reports placeholders for every hardcoded secret found.

### Integrations with secret managers

- [ ] 1Password CLI integration: shhh detects when a secret could be replaced by a `op://` reference and suggests the rotation.
- [ ] Bitwarden CLI integration: same idea.
- [ ] HashiCorp Vault integration for enterprise environments.
- [ ] Doppler, Infisical, SOPS integrations if demand surfaces.

These are *suggestions*, not automated changes. shhh never rewrites the user's secret storage without explicit consent.

### Observability and telemetry

- [ ] Opt-in anonymous telemetry for: detection counts by type, interception counts by layer, eval pass rates in the wild, false-positive reports.
- [ ] Privacy-reviewed telemetry schema documented publicly. Any change requires a new opt-in.
- [ ] `shhh telemetry opt-in` and `shhh telemetry opt-out` commands, off by default.

## Ship criterion

Phase 6 has no single ship criterion — it is ongoing. Each deliverable ships individually against its own criterion.

**Global success signals for Phase 6 as a whole:**

- Monthly active installs trending up quarter over quarter.
- Community PR throughput (rules added, tools integrated, docs improved) is growing.
- At least one non-founder contributor has made a meaningful code contribution (not just docs).
- The detection pipeline's false-positive rate on the public-example corpus remains below 5% even as new rules are added.
- The eval suite has grown to ≥15 tasks, with new tasks contributed by the community.

## Explicit anti-goals

- **No LLM-local enrichment in v1 or v2.** The original "local LLM for classification" idea is shelved until an eval task demonstrates pattern-based labeling is insufficient. If that eval task ever surfaces, it is a Phase 6 candidate; otherwise it is never built.
- **No subtractive commercialization.** If a paid tier ships, it is additive (hosted dashboards, team management, audit log ingestion). Nothing that currently exists in the open-source product moves behind a paywall.
- **No rewriting the user's secret storage.** shhh suggests, it never rewrites. The one exception is `shhh edit-secret`, which is a direct user-invoked action, not a background task.
- **No speculative protocol support.** If a new AI tool ships, shhh integrates with it when a user asks, not preemptively.
- **No per-vendor marketing partnerships** that would compromise the cross-vendor positioning.

## Phase-specific risks

| Risk | Severity | Response |
|------|----------|----------|
| Feature creep driven by loud individual requests | High | Every feature needs to cite a user *and* explain why shhh (not a specialist tool) should build it. Log the decision publicly. |
| Community PRs introduce detection rules with high false-positive rates | Medium | Every new rule runs against task 8 (public-example corpus) in CI. PRs that regress the FP rate are blocked. |
| Commercial tier fragments the open-source community | High | Keep the commercial tier additive only. Document the commercial/open split clearly. Never move existing functionality behind the paywall. |
| An IDE extension becomes the default UX and the CLI atrophies | Low | The CLI is the source of truth. The extension is a thin client. Do not duplicate logic. |
| Phase 6 work crowds out Phase 3–5 maintenance | High | Protect ~30% of ongoing time for bug fixes, eval updates, and detection rule drift on the core product. New features are the remaining ~70%. |

## Dependencies

- Phase 5 shipped and stable.
- At least 3 months of post-Phase 5 production usage to surface real patterns.
- A clear backlog of community-requested features to prioritize against.
- If team/enterprise features are on the table: a decision on commercial model and legal structure (which is a whole separate conversation, not a Phase 6 deliverable).
