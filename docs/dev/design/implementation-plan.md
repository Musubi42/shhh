# Implementation plan — audit feature + installer refactor

This is the consolidated build doc for the work discussed across the
audit design sessions. It assumes the reader has already skimmed
[`README.md`](./README.md), opened
[`mockups/overview.html`](./mockups/overview.html) and
[`mockups/project-detail.html`](./mockups/project-detail.html), and
read [`installer-tui.md`](./installer-tui.md) +
[`cli-output.md`](./cli-output.md).

It is ordered so that each section unlocks the next — implement
top-to-bottom.

## Scope lock

Everything here is in scope for the audit feature silo. Anything not
listed here is explicitly out of scope.

**In scope:**
- New package `internal/audit/` with a Source interface and four
  concrete sources (transcripts, paste-cache, prompt history, file
  edit history)
- New subcommand `shhh audit` with the flags documented in
  [`cli-output.md`](./cli-output.md)
- HTML report generation via `html/template`, fonts embedded as
  base64 data URIs so the report works offline
- Ephemeral local HTTP server bound to `127.0.0.1:0` (OS-chosen
  port), lifecycle tied to the `shhh audit` process
- Snapshot persistence at `~/.shhh/audits/<timestamp>.json` with a
  `latest.json` symlink
- Delta computation between the most recent snapshot and the
  previous one
- Installer TUI refactor: agent-first flow, per-agent project
  multi-select, scan opt-in with disclosure
- Removal of the `global vs project` scope prompt from the
  interactive installer flow (code path kept for the scripted
  `shhh install claude-code`)

**Out of scope (deferred or dropped):**
- Git log scanning (dropped, not even behind `--deep`)
- Shell history, `~/.aws/credentials`, docker, keychain scanning
  (dropped)
- Auto-remediation (rotate, move-to-vault) — not v0.2
- Codex and Cursor agents — separate feature silos
- Obfuscation level picker — resolved α (one level)
- Timeline view in HTML (current state + per-project detail only)
- SARIF export format (JSON is placeholder for SARIF later)
- NPM wrapper package and GitHub Actions release workflow — still
  deferred until the user approves publication infrastructure

## Package layout

```
cmd/shhh/
  main.go                          # add case "audit" → cmdaudit.Run
  cmdaudit/
    cmdaudit.go                    # Run(args []string) entry point, flag parsing
    render_cli.go                  # terminal renderer (ANSI colors, groups, footer)
    render_html.go                 # HTML renderer via html/template
    server.go                      # ephemeral HTTP server, port 0, Ctrl-C lifecycle
    json_export.go                 # --json mode writer
  cmdinstall/
    interactive.go                 # REFACTOR: agent-first flow + per-project prompts
    plan.go                        # REFACTOR: Plan.Execute no longer takes Scope
                                   # from prompts (scripted path still supports it)
    project_select.go              # NEW: list projects from ~/.claude/projects/
                                   # for the TUI's multi-select

internal/
  audit/
    audit.go                       # Audit struct, Run(agents, projects) → Result
    source.go                      # AuditSource interface + AuditItem
    transcripts.go                 # Claude Code projects/*.jsonl source
    pastecache.go                  # Claude Code paste-cache/*.txt source
    prompthistory.go               # Claude Code history.jsonl source
    filehistory.go                 # Claude Code file-history/** source
    aggregator.go                  # runs detector, groups findings, builds Result
    snapshot.go                    # read/write ~/.shhh/audits/<ts>.json + latest
    delta.go                       # compute delta between two snapshots
    projectpath.go                 # decode the dash-encoded directory names
    status.go                      # decide Protected / Unprotected / Archived per project
```

New dependencies: zero. `html/template`, `net/http`, and
`encoding/json` are stdlib. No new `go.mod` entries.

## Data model

```go
// internal/audit/audit.go

type Status string
const (
  StatusUnprotected Status = "unprotected"
  StatusProtected   Status = "protected"
  StatusArchived    Status = "archived"
  StatusClean       Status = "clean"
)

type Severity string
const (
  SevLeaked  Severity = "leaked"
  SevAtRisk  Severity = "at-risk"
)

// Finding is one detected secret, enriched with audit context. It
// wraps detector.Finding and adds where/when/how-many.
type Finding struct {
  Placeholder   string           // typed placeholder from session.PlaceholderFor
  Label         string           // e.g. "STRIPE_LIVE_KEY"
  Severity      Severity
  Sources       []string         // e.g. ["transcript", "paste-cache"]
  Occurrences   int              // how many times across the audit surface
  FirstSeen     time.Time        // earliest appearance
  LastSeen      time.Time        // latest appearance
  Locations     []string         // file:line or session-id:msg-index
  RotationURL   string           // optional
}

// Project is one Claude Code project, identified by its dash-path.
type Project struct {
  AbsPath      string         // decoded: /Users/alice/work/backend
  DisplayPath  string         // tilde-abbreviated: ~/work/backend
  DashName     string         // raw: -Users-alice-work-backend
  Status       Status
  SessionsTotal int
  FirstSeen    time.Time
  LastSessionAt time.Time
  Leaked       []Finding
  AtRisk       []Finding
  ShhhInstalledAt *time.Time   // nil if not installed
  OnDisk       bool            // false ⇒ archived
}

// Result is the complete audit output for one run.
type Result struct {
  Agent       string    // "claude-code"
  AuditTime   time.Time
  ScanDuration time.Duration
  Projects    []Project // all projects, including clean ones
  Summary     Summary
  Delta       *Delta    // nil on first run
}

type Summary struct {
  ProjectsTotal       int
  ProjectsUnprotected int
  ProjectsProtected   int
  ProjectsArchived    int
  ProjectsClean       int
  SecretsLeaked       int
  SecretsAtRisk       int
}

type Delta struct {
  Since         time.Time
  Leaked        DeltaCount
  AtRisk        DeltaCount
  Protected     DeltaCount
}

type DeltaCount struct {
  Before int
  After  int
  Change int  // After - Before
}
```

## The Source interface

```go
// internal/audit/source.go

type AuditItem struct {
  SourceName   string     // "transcript", "paste-cache", "prompt-history", "file-history"
  ProjectDashName string  // which project this item is attributed to (may be empty)
  Location     string     // human-readable — "sess:abc123:msg:42" or "file:path@v3"
  Timestamp    time.Time  // when this content was created
  Content      string     // the text to feed the detector
}

type AuditSource interface {
  Name() string
  // Walk yields items under the selected projects. It passes each
  // item to the aggregator via the channel and returns when done.
  // Errors are non-fatal (best-effort) and logged via the logger.
  Walk(ctx context.Context, projects []string, out chan<- AuditItem) error
}
```

Each concrete source implements `Walk` and appends items. The
aggregator runs the detector on each `item.Content`, attaches
location/timestamp metadata, and groups by `(Label, Value)` into
`Finding`s.

## Source concretes

### transcripts.go — `~/.claude/projects/<dash>/<sid>.jsonl`

One `AuditItem` per message in the JSONL. Content = concatenation
of text parts (user messages, assistant messages, tool results).
Skip messages where `role == "system"`. Project attribution comes
from the enclosing dir name. Timestamp comes from each message's
`timestamp` field.

### pastecache.go — `~/.claude/paste-cache/*.txt`

One `AuditItem` per file. Content = entire file. No per-project
attribution possible (paste cache is flat; all pastes share a hash
filename). `ProjectDashName = ""` means "attribute to all projects"
in the aggregator — but realistically the aggregator reports
paste-cache findings as project-agnostic in the "global findings"
footer of the HTML. For CLI we roll them up under an explicit
"📎 Pasted content (no project)" bucket.

Wait — re-checking during the design session, the user said the
audit should be **grouped by project**. Paste-cache has no project
info. Options:

1. **Attribute to "unknown project"** and show a single bucket at
   the bottom of the project list.
2. **Cross-reference** each paste-cache finding with transcripts:
   if the same secret value appears in a transcript for project X,
   attribute the paste-cache finding to X too.

Option 2 is the cleaner product — a secret is "leaked in project X"
regardless of which source surfaced it first. Implementation:
during aggregation, after all sources are walked, walk the
findings-by-value map and propagate project attribution across
sources. O(n·m) where n = paste findings and m = transcript
findings, both small.

**Decision:** implement option 2. Fallback bucket "📎 Detected in
paste cache, no matching project" for findings that truly don't
appear anywhere else.

### prompthistory.go — `~/.claude/history.jsonl`

One `AuditItem` per line. Content = the `display` field (what the
user typed). `project` field in the line attributes it to a
project. Timestamp from the `timestamp` field.

### filehistory.go — `~/.claude/file-history/<session-id>/<path-hash>@v<N>`

One `AuditItem` per version file. Content = full file contents.
Project attribution = look up the session-id against
`~/.claude/sessions/<id>.json` to find the cwd, or fall back to
dash-scanning the filename if that file doesn't give us a cwd.
Timestamp from file mtime.

**If session-to-project resolution fails:** attribute to "unknown"
and use the cross-reference trick from paste-cache (option 2 above).

## Status decision

Per project, after detector runs:

```
func decideStatus(p *Project, shhhConfig *cmdinstall.Config) Status {
  if !p.OnDisk {
    return StatusArchived
  }
  if shhhConfig != nil && configCoversPath(shhhConfig, p.AbsPath) {
    return StatusProtected
  }
  if len(p.Leaked) == 0 && len(p.AtRisk) == 0 {
    return StatusClean
  }
  return StatusUnprotected
}

// configCoversPath reports whether shhh's install protects the given
// project path. For v0.2:
//   - global install → protects every project (always true)
//   - project install → protects iff path is under the install cwd
func configCoversPath(c *cmdinstall.Config, absPath string) bool {
  if c.Scope == string(cmdinstall.ScopeGlobal) {
    return true
  }
  for _, p := range c.Paths {
    // p is the settings.json path; project root is its parent's parent
    root := filepath.Dir(filepath.Dir(p))
    if strings.HasPrefix(absPath, root) {
      return true
    }
  }
  return false
}
```

A global install flips every project to `Protected`. Visually that
means every project that has findings shows as `Protected ✓` with
its findings in the "fixtures (allowlisted)" or "legacy (rotate
anyway)" bucket. The leaked ones from BEFORE the install still count
— those secrets reached Anthropic already; installing shhh now
protects future reads but does not unsend past ones.

## Snapshot format

File: `~/.shhh/audits/<YYYY-MM-DDTHH-MM-SSZ>.json`

```json
{
  "schema_version": 1,
  "audit_time": "2026-04-14T12:45:03Z",
  "agent": "claude-code",
  "summary": { ... },
  "projects": [ ... ]  // same shape as json_export.go
}
```

Symlink: `~/.shhh/audits/latest.json` → most recent snapshot.

Delta computation (`delta.go`): load `latest.json` before writing
the new snapshot, build a `Delta` struct by comparing counts per
status, return it as part of the new `Result`. Then write the new
snapshot and re-point the symlink.

## Render layers

### render_cli.go

Walk the `Result`, emit colored terminal output matching
[`cli-output.md`](./cli-output.md). Uses
`github.com/fatih/color` if added as a dep, OR raw ANSI via stdlib
if we want to avoid the dep (the output is simple enough).

**Decision:** raw ANSI via stdlib. One small `colorize` helper,
no new dep.

### render_html.go

Uses `html/template`. Single-file template with the full layout,
matching `mockups/overview.html` and `mockups/project-detail.html`
byte-for-byte on structure. Template receives the `Result` directly.

Layout files to produce per audit:
- `~/.shhh/audits/<ts>/index.html` — overview page
- `~/.shhh/audits/<ts>/projects/<slug>.html` — one per project

The server serves from this directory. Cross-page links use
relative paths so the files work both under the server and when
opened directly via `file://`.

**Fonts:** Redaction and Geist Mono are embedded as base64 data
URIs in the `<style>` block. No network needed when viewing the
report offline. Binary blob ~280 KB total. Embedded at build time
via `go:embed` against a fonts dir.

### server.go

```go
func ServeReport(ctx context.Context, dir string) (string, error) {
  lis, err := net.Listen("tcp", "127.0.0.1:0")
  if err != nil { return "", err }
  port := lis.Addr().(*net.TCPAddr).Port
  url := fmt.Sprintf("http://127.0.0.1:%d/", port)

  srv := &http.Server{
    Handler: http.FileServer(http.Dir(dir)),
    ReadHeaderTimeout: 5 * time.Second,
  }

  go func() {
    <-ctx.Done()
    shutdownCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
    defer cancel()
    _ = srv.Shutdown(shutdownCtx)
  }()

  go func() {
    _ = srv.Serve(lis)  // returns when Shutdown is called
  }()

  return url, nil
}
```

Main audit command installs a signal handler on SIGINT/SIGTERM,
cancels the context, waits for the server goroutine to drain,
exits 0.

## Installer refactor

Files to touch:
- `cmd/shhh/cmdinstall/interactive.go` — rewrite the `huh` form
  composition to match [`installer-tui.md`](./installer-tui.md)
- `cmd/shhh/cmdinstall/plan.go` — Plan still has `Scope` but the
  interactive path always sets it to `ScopeGlobal` and does not
  prompt for it. Tests stay green because the scripted path
  (`shhh install claude-code`) still exercises both scopes.
- `cmd/shhh/cmdinstall/project_select.go` — NEW. Lists Claude Code
  projects from `~/.claude/projects/*/` with decoded display
  paths. Used by the interactive form's multi-select.

The `Plan` struct gains one field:

```go
type Plan struct {
  Scope   Scope
  Agents  []string
  Cwd     string
  RunScan bool
  SelectedProjects []string  // NEW — per-agent project opt-in list
}
```

`SelectedProjects` is persisted into `Config` so the audit
subcommand knows which projects to include. Absent or empty means
"all projects the agent knows about."

## Task breakdown

Roughly the order to implement:

1. **`internal/audit/source.go`** — interface + `AuditItem` struct + logger plumbing (30 min)
2. **`internal/audit/projectpath.go`** — dash-path decode/encode + tilde-abbreviate + tests (30 min)
3. **`internal/audit/transcripts.go`** — Claude JSONL walker + tests against fixtures (1h)
4. **`internal/audit/pastecache.go`** — paste-cache walker (30 min)
5. **`internal/audit/prompthistory.go`** — history.jsonl walker (30 min)
6. **`internal/audit/filehistory.go`** — file-history walker + session-to-project resolution (1h)
7. **`internal/audit/aggregator.go`** — run detector on items, group findings, cross-source attribution (1h)
8. **`internal/audit/status.go`** — decide Protected/Unprotected/Archived (30 min)
9. **`internal/audit/snapshot.go`** — write/read/latest symlink + delta (1h)
10. **`internal/audit/audit.go`** — top-level `Run(ctx, cfg) (*Result, error)` (30 min)
11. **`cmd/shhh/cmdaudit/render_cli.go`** — terminal renderer (1.5h)
12. **`cmd/shhh/cmdaudit/json_export.go`** — JSON serializer (20 min)
13. **`cmd/shhh/cmdaudit/render_html.go`** — HTML template + font embedding + overview page (2h)
14. **`cmd/shhh/cmdaudit/render_html_project.go`** — per-project detail page template (1h)
15. **`cmd/shhh/cmdaudit/server.go`** — ephemeral HTTP server + Ctrl-C lifecycle (45 min)
16. **`cmd/shhh/cmdaudit/cmdaudit.go`** — flag parsing, orchestration, stdout/stderr wiring (45 min)
17. **`cmd/shhh/main.go`** — add `case "audit"` (5 min)
18. **`cmd/shhh/cmdinstall/project_select.go`** — project list builder for the TUI (30 min)
19. **`cmd/shhh/cmdinstall/interactive.go`** — refactor form to match wireframes (1h)
20. **`cmd/shhh/cmdinstall/plan.go`** — add `SelectedProjects` field + plumbing (30 min)
21. **Tests for the big ones** — aggregator, transcript walker, snapshot delta,
    status decider, project-path decode (2h)
22. **Smoke test end-to-end** against real `~/.claude/` (30 min)
23. **Update `docs/mvp/feature-home-audit.md`** to link to this folder and reflect the
    final scope (15 min — or delete and replace)

Rough total: 16 hours of focused work. Probably 2–3 sessions.

## Testing strategy

- **Unit tests per source** with fixture JSONL/TXT files under
  `internal/audit/testdata/`. The fixtures contain non-secret data
  that should NOT trip the detector, plus a handful of intentionally
  leaked values that SHOULD trip it. Each source test asserts the
  expected `AuditItem` stream.
- **Aggregator test** with a synthetic pipeline: hand-crafted
  `AuditItem`s with the same secret value across different sources
  → assert cross-source attribution is correct.
- **Snapshot round-trip** — write, read, compute delta, assert.
- **Status decider table test** — every combo of (OnDisk, has config,
  has findings) → expected Status.
- **HTML template test** — render against a fixture `Result`, assert
  the HTML contains expected strings. Not a pixel-diff; structural
  assertions only.
- **Server lifecycle test** — start server with a short context,
  cancel it, assert shutdown returns within timeout.
- **Interactive installer test** — cannot drive `huh` without a TTY,
  so test the Plan construction logic directly (as we did for
  milestone 1).

## Honest forcing-function check

Does this bring the demo closer? **Yes.** The demo becomes:

```
$ shhh audit
[visible forensic report in terminal]
🌐 Full interactive report: http://127.0.0.1:54281/
   Press Ctrl-C to stop the report server.
```

On a fresh machine with an existing Claude Code install and
several projects with past leaks, this single command produces:
- a terminal report the dev can read immediately
- a beautiful HTML report the dev can open in a browser
- a snapshot that next week's audit will compare against
- a concrete list of rotations to do NOW

If the user runs `shhh audit` and sees an empty, useless, or
confusing report, we know the feature is off — and we can fix it
BEFORE shipping. That's the forcing-function loop working.

## Things this plan explicitly does NOT decide

Left for implementation time, because the answer depends on
details I can't predict from the spec:

- Exact JSONL field names in `~/.claude/projects/*/*.jsonl` (need
  to open a real file to confirm)
- Exact shape of `~/.claude/history.jsonl` entries
- Session-to-project resolution for `file-history/` — need to check
  if `~/.claude/sessions/<id>.json` actually contains a cwd field
- Whether paste-cache filenames have any project metadata we can
  use before falling back to cross-source attribution

These are all ~15-minute fact-finding trips during implementation,
not architectural decisions. Flagged here so they don't become
surprises.

## What I need from you to start implementing

Nothing — all the open questions were resolved in the previous
design sessions. The next session is a "walk the task list and
write code" session. If anything in this plan contradicts something
you remember deciding differently, tell me before I start.
