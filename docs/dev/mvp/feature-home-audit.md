# Feature: home audit (`shhh scan --home`)

**Status:** draft. Unresolved Open questions below.

## Forcing function

```
$ shhh scan --home

🛡️  shhh audit — scanning $HOME for secrets at risk

🔴 ~/work/backend                   4 secrets   (.env, config/auth.json)
   STRIPE_LIVE_KEY, AWS_ACCESS_KEY, GITHUB_PAT, POSTGRES_CONNSTRING

🔴 ~/work/frontend                  1 secret    (.env.local)
   OPENAI_PROJECT_KEY

🔴 ~/personal/blog                  2 secrets   (.env)
   OPENAI_PROJECT_KEY, SENDGRID_API_KEY

🟡 ~/experiments/scratchpad         — skipped (no project root)

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
  3 projects at risk · 7 secrets · 0 protected by shhh
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

⚠️  shhh is NOT installed in your Claude Code config.
    Run `npx shhh install` to protect these secrets.
```

A developer on a machine they've been using for months gets a
one-command audit of every secret across every project they have,
plus a call-to-action that names exactly what to do next.

This is the PRD §1 principle 4 — *"scan is the marketing"* —
feature. It's shareable. It runs in seconds. It doesn't require
installing anything destructive before producing value. A
screenshot of this output is the hero image of the README.

## What already exists in the codebase

Almost all of this.

- `internal/scanner` — file walker with sensitive-path detection,
  `.env` cross-reference (stage 3), and the 39-rule detector
  pipeline. Already runs on arbitrary directories.
- `cmd/shhh/cmdscan` — the existing `shhh scan [path]` subcommand.
  Hooks up the walker to pretty output.
- `internal/detector.CheckEnvValue` — the looser env-context
  gate that lets the scanner catch short high-quality tokens
  (the fix we added for milestone 1 lives here too).
- `internal/rules` — full rule set.

## What's new

Thin layer on top of the scanner:

1. **Project-root walker.** Given `$HOME`, enumerate directories
   that look like project roots. A directory is a project root if
   it contains any of: `.git/`, `package.json`, `go.mod`,
   `Cargo.toml`, `pyproject.toml`, or simply a `.env` / `.env.*`
   file. Walk stops descending once a project root is found (so
   we don't rescan the same tree from nested sub-projects).
2. **Multi-project aggregated output.** Instead of the current
   single-directory report, group findings by project and show a
   summary footer (count of projects, count of secrets, count
   protected). The "protected" column reads agent config files
   to check whether shhh is installed globally or locally.
3. **Agent-config inspection.** Read `~/.claude/settings.json`,
   `~/.codex/*`, `~/.cursor/*` and report whether shhh is
   installed. This is the "0 protected by shhh" line in the
   footer. Drives the call-to-action at the bottom.
4. **A new CLI flag `--home`** on the existing `shhh scan`
   subcommand. `shhh scan` (no args) keeps its current behavior.
   `shhh scan --home` scans from `$HOME` with the multi-project
   walker. `shhh scan <path>` still scans that path.

## Open questions

### Q1. Project-root heuristic

Which markers count as a project root? My current list:

- `.git/` — obvious
- `package.json` — JS
- `go.mod` — Go
- `Cargo.toml` — Rust
- `pyproject.toml` / `setup.py` / `requirements.txt` — Python
- Any `.env` or `.env.*` file

Do we want to include more (Gemfile, composer.json, pom.xml,
build.gradle, Dockerfile-only directories)? Or is the current
list enough for v0.2 and we extend as users complain?

### Q2. How deep does the walker go?

$HOME can contain enormous trees (node_modules, .cache,
~/Library/..., ~/Downloads). Current plan: hard-skip well-known
noise dirs (`node_modules`, `.cache`, `Library`, `.Trash`,
`.local/share/Trash`, `go/pkg`, `.rustup`, `.cargo/registry`,
`venv`, `.venv`, `__pycache__`, `.tox`). Stop descending into a
directory when a project root is found.

Is that enough? Should we have a `--max-depth` flag? A timeout?

### Q3. Output format

Three candidates:

- **Grouped table** (what I sketched above). Each project is a
  block. Good for screenshots.
- **Flat list** (one line per finding, like `shhh scan` today).
  Easier to pipe to grep. Less visually striking.
- **Both.** Table by default, `--format flat` for piping,
  `--format json` for tooling.

My vote: **grouped by default**, `--format json` for tooling
consumers, no flat mode for v0.2 (add if anyone asks).

### Q4. Should the audit also flag un-hooked agents?

The mockup's footer line (*"shhh is NOT installed in your Claude
Code config"*) reads agent settings files to detect install
state. This adds complexity (file reading, JSON parsing, three
agent configs to know about) but ties the audit to the install
call-to-action.

Alternative: keep the audit pure (just secrets), and have the
installer prompt *"run an audit now?"* at the end. User sees the
same information but in two steps.

My vote: **include the agent-config check**, because the
call-to-action is what makes the audit actionable instead of
just alarming.

### Q5. Privacy around the report

The grouped report contains absolute project paths (`~/work/backend`
resolves to `/Users/alice/work/backend`). If a user screenshots
it for a bug report or shares it in a team channel, those paths
leak. PRD §1 principle ("zero trust in telemetry") suggests:

- Default output uses tilde-abbreviated paths (`~/work/backend`).
- A `--show-details` flag (mirroring the one already on
  `shhh scan` for host/user info) expands to absolute paths.
- Screenshots are safe by default.

Confirm?

## Out of scope for this feature

- **Remediation.** The audit reports; it does not offer to
  rotate, move, or delete any secret. Any form of "fix it for
  me" is a v0.3+ conversation.
- **Continuous monitoring.** No file-watcher, no cron, no "run
  in the background and alert me." One-shot audit only.
- **Network-based discovery.** shhh only reads local files. It
  does not query GitHub for exposed secrets, does not hit
  HaveIBeenPwned, does not phone home for anything.
- **Installer integration.** The installer *calls* this feature
  as an optional post-install step. The feature itself does not
  know about the installer. That coupling lives in the
  installer spec, not here.

## What I need from you to start implementing

Answers to Q1–Q5. Everything else is turning existing code into
a slightly different output format.
