# shhh-native engine

shhh's first-party detection engine. Exists to cover capabilities
that the linked gitleaks engine does not address by design.

This is not a "gitleaks replacement." For broad provider coverage,
prefer `gitleaks`. shhh-native is the right pick when the user's
threat model depends on the two specific behaviors below.

## What it is

A small Go package living in `internal/detector/` (the
`shhh-native` Backend). Built and maintained alongside shhh
itself, no external dependency.

## What it covers that gitleaks does not

### Env cross-reference

Detects when a value defined as a secret in `.env`
(`STRIPE_KEY=sk_live_...`) appears hardcoded somewhere else in
the project tree (e.g. dropped into a Markdown file, a comment,
or a config). gitleaks scans each file independently and does
not maintain this cross-file linkage.

### Structural URL preservation

For URLs of the shape `scheme://user:password@host[:port]/path`,
masks the credential portion while keeping the rest of the URL
visible: `postgres://[DB_USER]:[DB_PASSWORD]@db.internal:5432/app`.
The agent can still reason about the host, database, and port —
which is usually what the user wants when debugging a connection
issue — without seeing the raw credentials.

gitleaks would either flag and redact the whole URL or miss it
entirely; it does not split-and-preserve.

## What it does not do

- **Fewer provider rules than gitleaks.** The hand-maintained
  rule set is intentionally smaller.
- **No upstream community.** Rule updates are on shhh's release
  cycle, not gitleaks'.

## When shhh picks it

`shhh-native` is currently the default for fresh installs, and
can be selected explicitly via `shhh install <agent>`. It can
also be set per-project. The two engines do not compose today
(no "run both") — that mode was removed; see commit
`refactor(engine): rename legacy → shhh-native, drop both mode`.

## Further reading

- Engine architecture, precedence, naming history:
  `docs/dev/engine-architecture.md`
