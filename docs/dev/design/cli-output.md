# `shhh audit` — terminal output

The terminal output is the primary face of the audit for users who
live in their terminal. The HTML report is richer, but the CLI
output has to stand on its own: it must tell the full story without
the user needing to open a browser. The HTML link at the bottom is a
bonus, not a requirement.

## Layout shape

```
┌─────────────────────────────────────────────────────────────────────┐
│  HEADER BAR          shhh audit · claude code · timestamp          │
├─────────────────────────────────────────────────────────────────────┤
│  SCAN PROGRESS       ▸ scanning ...                                 │
│                      ▸ reading ...                                  │
│                      (erased after scan completes)                  │
├─────────────────────────────────────────────────────────────────────┤
│  SUMMARY BAR         13 projects · 3 unprotected · ...              │
├─────────────────────────────────────────────────────────────────────┤
│  DELTA (if prior     Since last audit (4 days ago):                 │
│  snapshot exists)    🚨 Leaked: 7 → 4 (−3, 3 rotated)               │
├─────────────────────────────────────────────────────────────────────┤
│  PROJECTS            📁 ~/work/backend        [UNPROTECTED]         │
│  (grouped by         🚨 already leaked ...                          │
│   project)           ⚠️ currently at risk ...                       │
│                                                                     │
│                      📁 ~/personal/blog       [UNPROTECTED]         │
│                      ...                                            │
├─────────────────────────────────────────────────────────────────────┤
│  FOOTER ACTION       → rotate · → protect                           │
├─────────────────────────────────────────────────────────────────────┤
│  HTML SERVER LINE    🌐 Report served at http://127.0.0.1:54281/    │
│                      Press Ctrl-C to stop.                          │
└─────────────────────────────────────────────────────────────────────┘
```

## Full worked example

```
🛡️  shhh audit — Claude Code · 2026-04-14 12:45:03 UTC

▸ scanning project files................... 13 projects · 7 with secrets
▸ reading Claude session transcripts........ 94 sessions · 3 years
▸ reading Claude paste cache................ 423 blocks
▸ reading Claude prompt history............. 1 file · 584 KB
▸ reading Claude file edit history.......... 125 edit sessions
scan took 1.2s

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  13 projects · 3 unprotected · 1 protected · 1 archived · 8 clean
  🚨 4 leaked   ⚠️ 2 at risk   ✅ 1 protected
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

📊 Since last audit · 2026-04-10 · 4 days ago
     🚨 Leaked     7 → 4   (▼ 3 rotated, good work)
     ⚠️ At risk    6 → 2   (▼ 4 newly protected)
     ✅ Protected  0 → 1   (▲ 1 project shielded)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

📁 ~/work/backend                                       [UNPROTECTED]
   18 sessions · first seen 2026-01-12
   🚨 Already leaked to Claude
      [STRIPE_LIVE_KEY:sk_live_...:a1b2]   4 sessions · since 2026-03-21
      [AWS_ACCESS_KEY:AKIA...:c3d4]        2 sessions · since 2026-03-24
   ⚠️  Currently at risk
      [POSTGRES_CONNSTRING:admin@prod-db.internal:5432/myapp:e5f6]
                                           .env:4
      [SENDGRID_API_KEY:SG...:g7h8]        config/mail.json:12

📁 ~/personal/blog                                      [UNPROTECTED]
   6 sessions · first seen 2026-03-28
   🚨 Already leaked to Claude
      [OPENAI_PROJECT_KEY:sk-proj-...:i9j0]    1 session · prompt history

📁 ~/Documents/Musubi42/shhh                            [PROTECTED ✓]
   shhh installed 2026-04-13 · 12 protected sessions since
   ⚠️  Fixtures (allowlisted — test data, not real secrets)
      [STRIPE_LIVE_KEY:sk_live_...:k1l2]
           testdata/fixtures/leaky-project/.env

📁 /tmp/test-fb-com                                     [ARCHIVED]
   folder gone · 1 session retained in Claude history
   🚨 Already leaked to Claude
      [OPENAI_PROJECT_KEY:sk-proj-...:m3n4]    1 session · 2026-04-05

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  ✗ 4 secrets already leaked — rotation is not optional
  ✗ 2 secrets at risk — protect with:

    cd ~/work/backend && shhh install
    cd ~/personal/blog && shhh install

  ✓ Rotation dashboards:
    stripe:  https://dashboard.stripe.com/apikeys
    aws:     https://console.aws.amazon.com/iam/home#/users
    openai:  https://platform.openai.com/api-keys

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

🌐 Full interactive report: http://127.0.0.1:54281/
   Press Ctrl-C to stop the report server.
```

## Color coding

| Element | Color | ANSI |
|---|---|---|
| `🚨 leaked` sections | red | 31 |
| `⚠️ at risk` sections | yellow | 33 |
| `✅ protected` / `✓` | green | 32 |
| Placeholders `[LABEL:...]` | default fg (for readability) | 0 |
| Paths (`~/work/backend`) | bold default | 1 |
| Delta: `▼` improvements | green | 32 |
| Delta: `▲` regressions | red | 31 |
| Horizontal rules (`━━`) | dim | 2 |
| Section titles (`📊 Since last audit`) | bold | 1 |
| `[UNPROTECTED]` badge | red, bold | 1;31 |
| `[PROTECTED ✓]` badge | green, bold | 1;32 |
| `[ARCHIVED]` badge | dim, italic | 2;3 |

If stdout is not a TTY, all color is stripped (`github.com/fatih/color`
or equivalent does this automatically). Pipes to `grep` or redirects
to files get clean text with no ANSI noise.

## Clean scan (best case)

When nothing is leaked and nothing is at risk:

```
🛡️  shhh audit — Claude Code · 2026-04-14 12:45:03 UTC

▸ scanning ... 13 projects analyzed.
scan took 0.9s

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  13 projects · 13 clean · 0 leaked · 0 at risk
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

📊 Since last audit · 2026-04-10 · 4 days ago
     🚨 Leaked     4 → 0   (▼ 4 all rotated, nice)
     ⚠️ At risk    2 → 0   (▼ 2 newly protected)
     ✅ Protected  1 → 3   (▲ 2)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

  ✓ No secrets found. All projects clean.

🌐 Full interactive report: http://127.0.0.1:54281/
   Press Ctrl-C to stop the report server.
```

No per-project listing when every project is clean. The header and
delta tell the whole story. The HTML report still has the full per-
project breakdown for archaeology.

## First-ever scan (no prior snapshot)

```
🛡️  shhh audit — Claude Code · 2026-04-14 12:45:03 UTC

▸ scanning ... 13 projects analyzed.
scan took 1.2s

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  13 projects · 2 unprotected · 0 protected · 0 archived · 11 clean
  🚨 2 leaked   ⚠️ 3 at risk   ✅ 0 protected
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

📊 First audit — no previous snapshot to compare against.
    From now on shhh will track progress between audits.

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

[... project list ...]
```

The delta section is replaced with a one-line note. All other
sections are unchanged.

## Flag matrix

```
shhh audit                      terminal + HTML report + ephemeral server (default)
shhh audit --no-serve           terminal only, no HTML file, no server, exit on done
shhh audit --html-only          terminal + HTML file written, no server, exit on done
shhh audit --open               terminal + HTML + server, AND launch $BROWSER on the URL
shhh audit --json               JSON report to stdout, no color, no HTML, no server
                                (machine-readable for CI, eventual SARIF export)
```

`--no-serve`, `--html-only`, and `--json` are mutually exclusive
with `--open`. Passing two of the four returns a usage error
before scanning starts.

## Server lifecycle

When the server is running (default behavior), the shhh process
stays in the foreground:

```
🌐 Full interactive report: http://127.0.0.1:54281/
   Press Ctrl-C to stop the report server.
```

A `Ctrl-C` triggers a graceful shutdown: the HTTP server is asked to
stop accepting new connections, any in-flight responses finish, the
goroutine returns, and the process exits 0. Closing the terminal
sends SIGHUP with the same effect. No way for the server to outlive
the process.

The port is chosen by the OS (`net.Listen("tcp", "127.0.0.1:0")`),
so collisions with existing services are impossible.

## JSON output mode

`shhh audit --json` emits a single JSON document to stdout, suitable
for CI pipelines and future SARIF conversion:

```json
{
  "schema_version": 1,
  "audit_time": "2026-04-14T12:45:03Z",
  "agent": "claude-code",
  "summary": {
    "projects_total": 13,
    "projects_unprotected": 3,
    "projects_protected": 1,
    "projects_archived": 1,
    "projects_clean": 8,
    "secrets_leaked": 4,
    "secrets_at_risk": 2,
    "secrets_protected": 1
  },
  "delta_since": "2026-04-10T08:12:00Z",
  "delta": {
    "leaked":    { "before": 7, "after": 4, "change": -3 },
    "at_risk":   { "before": 6, "after": 2, "change": -4 },
    "protected": { "before": 0, "after": 1, "change": +1 }
  },
  "projects": [
    {
      "path": "/Users/alice/work/backend",
      "display_path": "~/work/backend",
      "status": "unprotected",
      "sessions_total": 18,
      "first_seen": "2026-01-12T09:30:00Z",
      "leaked": [
        {
          "placeholder": "[STRIPE_LIVE_KEY:sk_live_...:a1b2]",
          "label": "STRIPE_LIVE_KEY",
          "first_seen": "2026-03-21T14:00:00Z",
          "last_seen": "2026-04-10T11:20:00Z",
          "session_count": 4,
          "sources": ["transcript", "paste-cache"],
          "rotation_url": "https://dashboard.stripe.com/apikeys"
        }
      ],
      "at_risk": [
        {
          "placeholder": "[POSTGRES_CONNSTRING:...:e5f6]",
          "label": "POSTGRES_CONNSTRING",
          "location": ".env:4",
          "rule": "postgres_url"
        }
      ]
    }
  ]
}
```

Raw secret values NEVER appear in JSON output either. Only
placeholders. The JSON is safe to attach to a PR comment.
