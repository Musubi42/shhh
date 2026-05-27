# `shhh audit` — agent reference

Terse, AI-discoverable reference for the audit subsystem. Use this
to find commands, types, and side-effects without reading source.
See section "Source map" at the bottom for the actual files.

## What audit does

Scans the local Claude Code state directory (`~/.claude/projects/*`)
plus 3 secondary sources for secret-shaped values that already
reached Claude, classifies each project by current shhh install
state, and renders a CLI summary + interactive HTML report.

Read-only on `~/.claude/`. Writes only to `~/.shhh/` (snapshots +
config). No network. No process left running unless the report
server is up (default behavior; `--no-serve` opts out).

## CLI surface

```
shhh audit                     interactive picker → scan → CLI summary → HTML server
shhh audit --no-select         skip picker (audit every non-ignored project)
shhh audit --no-serve          terminal only, no HTML, no server
shhh audit --html-only         terminal + write HTML to disk; print the path
shhh audit --open              default + launch the browser on the report URL
shhh audit --json              machine-readable JSON to stdout (implies --no-serve, --no-select)

shhh audit ignore <abs-path>   persist a skip for one project
shhh audit unignore <abs-path> reverse the above
shhh audit ignored             print the current ignore list
```

Selection is driven by an interactive picker (`huh.MultiSelect`) on
TTY. Unchecking a project writes its absolute path to
`~/.shhh/config.json::ignored_paths` (persistent). Re-checking
removes it. The `ignore`/`unignore` subcommands are equivalent
scripting paths.

## Status values (per project)

| Status | When | Display badge (CLI / HTML) |
|---|---|---|
| `protected` | shhh hook is currently wired into a settings.json covering this project's abs path | `[HOOKED ✓]` / `Hooked ✓` |
| `unprotected` | project has findings AND no shhh hook covers it | `[NOT HOOKED]` / `Not hooked` |
| `archived` | project's directory no longer exists on disk | `[FOLDER GONE]` / `Folder gone` |
| `clean` | project exists, has zero findings, may or may not be hooked | `[CLEAN]` / `Clean` |

Status decision: `internal/audit/status.go::DecideStatus`. The
`protected` decision is **defensive**: it reads the referenced
settings.json and verifies a `shhh hook` substring is present, not
just trusts `~/.shhh/config.json::installed_paths`. Stale config
will not produce false `protected`.

## Finding kinds

| Kind | Where it comes from | Surfaces as |
|---|---|---|
| **leaked** | a secret-shaped value found in past Claude transcripts / prompts / pastes / file-history | `🚨 Already leaked to Claude` block in HTML; required rotation |
| **at-risk** | a secret-shaped value found in files on disk under the project, found by `RedactEnvFile` on `.env*` and similar | `🔒 Will be redacted on next read` (hooked) or `⚠ At risk on next session` (not hooked) |

The `HIGH_ENTROPY` rule from the detector is filtered out at the
aggregator (drowns real findings in transcript noise: session UUIDs,
tool_use ids, git SHAs, etc.). Named rules only. See
`internal/audit/aggregator.go::Process` for the rationale comment.

## Session / event / project — units

- **Project** = a directory under `~/.claude/projects/<dash-encoded>/`. Claude Code creates one per unique `cwd`.
- **Session** = a single `.jsonl` file inside a project dir. One per `claude` CLI invocation.
- **Event** = one JSON line inside a `.jsonl`. Hundreds to thousands per session.

User-facing counters speak in **sessions**, not events. The internal
`ProgressSourceCount` event counts events per-source for debug paths
but is not displayed by the live UI.

Project path resolution: `ResolveProjectPath(dashName, projectDir)`
in `internal/audit/projectpath.go`. Prefers the `cwd` field from
inside any transcript (loss-less). Falls back to dash-decoding the
dir name (lossy for paths with literal hyphens — `open-source` →
`open/source`). Don't add another dash-decode call site.

## Progress events (programmatic surface)

Run() emits typed events via `Config.OnProgress` so callers can
build live UIs. Kinds defined in `internal/audit/audit.go`:

```
ProgressEnumerated         — fires once, with ProjectsTotal + SessionsTotal
ProgressSourceCount        — periodic, per source; cumulative event count (debug only)
ProgressSessionFinished    — once per .jsonl read; drives the sessions counter
ProgressProjectScanned     — once per project's transcripts done; intermediate scroll entry
ProgressFinding            — once per unique (placeholder, project) pair seen; drives leaked-counter
ProgressProjectFinished    — once per project after status decided; upgrades scroll entry
ProgressDone               — once at the end
```

The cmdaudit live renderer (`cmd/shhh/cmdaudit/progress.go`) keeps
the live block visible after `ProgressDone` instead of clearing it,
so users see finalized ✓ rows alongside the full report below.

## Side-effects on disk

| Path | Read/Write | Purpose |
|---|---|---|
| `~/.claude/projects/**/*.jsonl` | read | source of transcript findings |
| `~/.claude/prompts/`, `~/.claude/pastes/`, `~/.claude/files/` | read | secondary sources |
| `~/.claude/settings.json` | read | verify shhh install state defensively |
| `~/.shhh/config.json` | read + write | ignored_paths, installed_paths |
| `~/.shhh/audits/` | write | snapshot per audit (used to compute next delta) |
| `~/.shhh/audits/html-<ts>/` | write | rendered HTML report (overview + per-project pages) |

No global state. Each audit is a fresh `auditpkg.Run(ctx, cfg)`.

## Common extension points

| Add… | Where |
|---|---|
| a new source (e.g. shell history) | implement `AuditSource` in `internal/audit/`; append to `sources` in `run.go::Run` |
| a new finding kind | extend `Project` struct + `DecideStatus`; thread through CLI + HTML renderers |
| a new CLI subcommand | branch in `cmdaudit.Run` before flag parsing (see how `ignore`/`unignore`/`ignored` are wired) |
| a new HTML section | add a field to `overviewViewModel`, populate in `BuildOverview`, render in `templates/overview.html.tmpl` |
| a new progress event kind | add to `ProgressKind` const block; emit from Run; handle in renderer |

## Out-of-scope (do NOT add speculatively)

Per `CLAUDE.md`:
- No proxy daemon / Unix socket / MCP server as first-class surface
- No remote runner, Docker, or cloud aggregation
- No real-time monitoring during Claude sessions (that's the hook's job)
- No telemetry without explicit per-feature user opt-in

A future `shhh verify <placeholder>` (provider ping) and richer
per-project install commands are documented in
`docs/per-project-install-kickoff.md` as kick-off work, gated on
user demand — don't preempt.

## Source map

```
cmd/shhh/cmdaudit/
  cmdaudit.go        # Run() entry point, flag parsing, subcommand dispatch
  picker.go          # huh.MultiSelect pre-scan picker (TTY only)
  ignore.go          # `ignore`/`unignore`/`ignored` subcommand handlers
  progress.go        # live TTY renderer (scroll log + footer)
  render_cli.go      # slim post-scan CLI summary
  render_html.go     # full HTML report builder
  render_json.go     # --json mode
  server.go          # ephemeral local HTTP server for the report
  templates/*.tmpl   # HTML templates

internal/audit/
  audit.go           # Config, Result, Project, Finding, ProgressEvent types
  run.go             # Orchestrator (enumerate → drain → aggregate → finalize)
  status.go          # DecideStatus rules (incl. defensive settings.json re-read)
  projectpath.go     # Dash-encoded path resolution (transcript cwd preferred)
  aggregator.go      # Findings collection + dedup; OnFinding live callback
  transcripts.go     # Primary source: ~/.claude/projects/**/*.jsonl
  prompthistory.go   # Secondary: ~/.claude/prompts/
  pastecache.go      # Secondary: ~/.claude/pastes/
  filehistory.go     # Secondary: ~/.claude/files/
  snapshot.go        # Persist audit snapshots → enables Delta
  delta.go           # ComputeDelta vs prior snapshot
```

## Verification one-liners

```sh
make build
./bin/shhh audit --no-select --no-serve     # full scan, no UI, prints CLI summary
./bin/shhh audit --json | jq .summary       # machine-readable counts
./bin/shhh audit ignored                    # confirm persistent state
```
