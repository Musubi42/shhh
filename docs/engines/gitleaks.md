# gitleaks engine

shhh's third-party detection engine. gitleaks is an MIT-licensed
secret scanner maintained by Zachary Rice and contributors,
linked into shhh as a Go module (not invoked as a subprocess).

This is the recommended engine for most users. shhh-native covers
specific gaps gitleaks does not address by design.

## What it is

- Source: `github.com/zricethezav/gitleaks/v8`
- License: MIT
- ~222 detection rules covering most major providers
  (AWS, Stripe, GitHub, OpenAI, Anthropic, GitLab, Slack, Twilio,
  PEM keys, JWTs, generic high-entropy strings, …)
- Curated global allowlist for noise sources (lockfiles, vendor
  directories, binaries, fonts, images)

## What it covers well

- Token-shaped secrets with recognizable prefixes
  (`sk_live_`, `ghp_`, `xoxb-`, `eyJ…`)
- Entropy-based fallback for high-randomness blobs
- Common provider patterns kept up to date by an active community

## What it does not do

- **No cross-file env reference.** gitleaks treats each file in
  isolation. A value defined as `STRIPE_KEY=...` in `.env` and
  hardcoded elsewhere in the repo is not linked across files.
- **No structural URL preservation.** A `postgres://user:pwd@host/db`
  string is either redacted whole or not redacted at all. shhh
  wants to keep host/db visible for agent reasoning while masking
  credentials — gitleaks does not split it.

These two gaps are precisely what `shhh-native` exists to fill.

## When shhh picks it

`gitleaks` is selectable via `shhh install <agent>` (the install
TUI lets the user pick). It can also be set per-project via the
shhh config in the repo.

## Allowlist composition

gitleaks ships its own global allowlist. shhh layers user-defined
`.shhhignore` files (gitignore syntax) on top of it. Precedence
and merge rules: see `docs/dev/engine-architecture.md` §2.3.

## License notice

The gitleaks license text is embedded in the shhh binary and
shown by `shhh licenses`. When the pinned version in `go.mod`
changes, `cmd/shhh/cmdlicenses/gitleaks-LICENSE.txt` must be
updated to match.
