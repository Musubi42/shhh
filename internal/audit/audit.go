// Package audit implements the forensic audit produced by `shhh audit`.
//
// The audit answers two questions no other secret scanner can: (1) which
// secrets have ALREADY been sent to a coding agent (Claude Code today)
// via past sessions, paste cache, prompt history, or file-edit history;
// and (2) which secrets are currently on disk in known project
// directories that an agent could read on its next session.
//
// The audit is read-only and strictly local. No network I/O, no
// exfiltration, no side effects beyond writing snapshots under
// ~/.shhh/audits/. Raw secret values never appear in any output —
// every finding is rendered as a session-salted placeholder.
package audit

import "time"

// Status is the per-project verdict.
//
// Unprotected: the project has findings (leaked or at-risk) and shhh is
// not installed here. The default dangerous state.
//
// Protected: shhh is installed for this project. Findings may still
// appear (historical leaks, fixture files) but future sessions are
// covered by the hook.
//
// Archived: the project directory no longer exists on disk, but Claude
// Code's local history still holds transcripts/paste-cache entries
// attributed to it. Nothing to rotate on disk, but past leaks remain
// leaked.
//
// Clean: no findings at all, in any source, and no on-disk secrets.
type Status string

const (
	StatusUnprotected Status = "unprotected"
	StatusProtected   Status = "protected"
	StatusArchived    Status = "archived"
	StatusClean       Status = "clean"
)

// Severity classifies a finding.
//
// Leaked: the secret appeared in at least one Claude Code local source
// (transcript, paste cache, prompt history, file-edit history) before
// shhh was installed. Assume Anthropic has the raw value — rotation
// is non-negotiable.
//
// AtRisk: the secret is currently in a project file on disk and shhh
// is not protecting that project. It has not yet been sent to Claude,
// but the next session that reads this file would leak it.
type Severity string

const (
	SevLeaked Severity = "leaked"
	SevAtRisk Severity = "at-risk"
)

// Finding is a single detected secret, enriched with the audit context
// (where it appeared, when, how many times). It is the aggregated view
// of what the detector found across one or more AuditItems.
//
// Raw secret values are NEVER stored here. Placeholder carries the
// session-salted, type-preserving redaction of the value.
type Finding struct {
	Placeholder string    // e.g. "[STRIPE_LIVE_KEY:sk_live_...:a1b2]"
	Label       string    // e.g. "STRIPE_LIVE_KEY"
	Severity    Severity  // leaked or at-risk
	Sources     []string  // e.g. ["transcript", "paste-cache"]
	Occurrences int       // total across all AuditItems that carry this value
	FirstSeen   time.Time // earliest timestamp across occurrences
	LastSeen    time.Time // latest timestamp across occurrences
	Locations   []string  // human-readable locations, deduplicated
	SessionIDs  []string  // distinct Claude session IDs where this appeared
	RotationURL string    // optional remediation link (populated per Label)
}

// Project is one Claude Code project, keyed by its absolute path.
//
// A Claude project exists iff there is a corresponding dir under
// ~/.claude/projects/ (Claude Code creates one per unique cwd). The
// project's on-disk folder may or may not still exist — that's
// what OnDisk and Status=Archived track.
type Project struct {
	AbsPath         string     // decoded absolute path, e.g. "/Users/alice/work/backend"
	DisplayPath     string     // tilde-abbreviated, e.g. "~/work/backend"
	DashName        string     // raw dir name under ~/.claude/projects/
	Status          Status
	OnDisk          bool       // whether AbsPath exists on disk right now
	SessionsTotal   int        // distinct sessions Claude Code recorded for this project
	FirstSeen       time.Time  // earliest session timestamp
	LastSessionAt   time.Time  // most recent session timestamp
	ShhhInstalledAt *time.Time // nil if shhh is not protecting this project
	Leaked          []Finding  // severity == leaked, sorted by FirstSeen asc
	AtRisk          []Finding  // severity == at-risk, sorted by Label
}

// Result is the complete output of one audit run. It is the single
// unit of data that gets rendered (CLI, HTML, JSON), persisted
// (snapshot), and compared (delta).
type Result struct {
	SchemaVersion int           // bumped on incompatible snapshot changes
	Agent         string        // "claude-code" in v0.2
	AuditTime     time.Time     // when Run() was called
	ScanDuration  time.Duration // wall-clock of the audit
	Projects      []Project     // every project Claude Code knows about, clean or not
	Summary       Summary       // aggregate counts
	Delta         *Delta        // nil on the very first audit
}

// Summary is the aggregated counts that feed the header bar.
//
// The project counts are mutually exclusive (sum to ProjectsTotal).
// The secret counts can overlap: a project with 2 leaked and 1
// at-risk contributes to SecretsLeaked AND SecretsAtRisk.
type Summary struct {
	ProjectsTotal       int
	ProjectsUnprotected int
	ProjectsProtected   int
	ProjectsArchived    int
	ProjectsClean       int
	SecretsLeaked       int
	SecretsAtRisk       int
	SecretsProtected    int // distinct secret values in PROTECTED projects
}

// Delta captures the change since the previous audit. It is nil on
// the first run. DeltaCount.Change is After minus Before (positive
// means "more secrets now", negative means "fewer").
type Delta struct {
	Since     time.Time
	Leaked    DeltaCount
	AtRisk    DeltaCount
	Protected DeltaCount
}

// DeltaCount is one counter's before/after/change triple.
type DeltaCount struct {
	Before int
	After  int
	Change int
}

// Config controls an audit run. Built from cmdinstall.Config (the shhh
// user config on disk) plus any per-run flags from the CLI.
//
// SelectedProjects is the opt-in list of project dash-names to audit.
// Empty means "audit every project ~/.claude/projects/ knows about."
//
// OnProgress, if non-nil, is called from inside Run as the audit
// progresses. It receives typed events the renderer uses to drive
// the live "scrolling log + footer" terminal UI. Set to nil in tests
// and non-interactive paths. See ProgressEvent for the event shape.
type Config struct {
	Agent            string // "claude-code"
	ClaudeRoot       string // usually ~/.claude, overrideable for tests
	SelectedProjects []string
	// IgnoredPaths is the user's persistent skip list (loaded by
	// cmdaudit from ~/.shhh/config.json before calling Run). Projects
	// whose absolute path matches an entry exactly are dropped before
	// any scanning happens — they don't appear in counts, don't
	// trigger transcript reads, don't show up in the report. Empty
	// list means audit everything.
	IgnoredPaths []string
	// ScopePaths is the per-run "only include projects under these
	// roots" allow-list. Each entry is an absolute path. A project
	// passes the filter if its AbsPath equals an entry or lives under
	// one. Empty means "no scope restriction" (the default — audit
	// every project that survives IgnoredPaths).
	//
	// This is distinct from SelectedProjects (which matches by Claude
	// dash-name) and IgnoredPaths (deny list, persisted). ScopePaths
	// is the CLI-positional `shhh audit <path>...` filter, ephemeral
	// to one run.
	ScopePaths     []string
	ShhhConfigPath string // usually ~/.shhh/config.json, overrideable
	AuditDir       string // usually ~/.shhh/audits, overrideable
	OnProgress     func(ProgressEvent)
}

// ProgressKind labels the kind of progress event being emitted.
type ProgressKind int

const (
	// ProgressEnumerated fires once, right after the project list is
	// known. ProjectsTotal and SessionsTotal are populated.
	ProgressEnumerated ProgressKind = iota + 1
	// ProgressSourceCount fires periodically from drainSources. Source
	// and Count are populated. Cumulative count, not delta.
	ProgressSourceCount
	// ProgressSessionFinished fires once per Claude transcript file
	// (one user session) after it has been fully read. The renderer
	// uses this to drive the "N/M sessions scanned" counter.
	ProgressSessionFinished
	// ProgressProjectScanned fires once per project after all its
	// transcripts have been read by TranscriptSource. Findings are
	// not yet aggregated at this point — the renderer uses this as a
	// "we just finished one project's transcripts" signal to advance
	// the live project counter and append a tentative scroll entry.
	// ProjectIndex / ProjectDisplay are NOT populated (we only know
	// the dashName at this stage); the renderer is expected to map it
	// from a registry built at ProgressEnumerated time.
	ProgressProjectScanned
	// ProgressFinding fires once per distinct (placeholder, project)
	// pair the aggregator sees, so the renderer can tick the "leaked
	// so far" counter live instead of waiting for Finalize().
	ProgressFinding
	// ProgressProjectFinished fires once per project after its
	// findings are finalised and status is decided. Project*, Sessions,
	// Leaked, AtRisk, and Status are populated.
	ProgressProjectFinished
	// ProgressDone fires once when Run is about to return.
	ProgressDone
)

// ProgressEvent is the unit of progress reporting. A single struct
// rather than a sum type so the channel signature stays trivial.
// Unused fields are zero values; renderers should switch on Kind.
type ProgressEvent struct {
	Kind ProgressKind

	// ProgressEnumerated:
	ProjectsTotal int
	SessionsTotal int

	// ProgressSourceCount:
	Source string
	Count  int

	// ProgressProjectFinished:
	ProjectIndex   int    // 0-based among the enumerated projects
	ProjectDisplay string // tilde-shortened display path
	Sessions       int    // session count for this project
	Leaked         int    // distinct secrets already leaked
	AtRisk         int    // distinct secrets in files on disk
	Status         Status // Protected / Unprotected / Archived / Clean

	// ProgressProjectScanned / ProgressFinding:
	ProjectDashName string // identifies the project for both events
	Placeholder     string // only set for ProgressFinding
}

// CurrentSchemaVersion is the version Result is stamped with when
// written as a snapshot. Readers refuse older/newer versions instead
// of silently misinterpreting them.
const CurrentSchemaVersion = 1
