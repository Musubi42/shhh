package audit

import (
	"context"
	"time"
)

// AuditItem is one unit of text to feed the detector, annotated with
// enough context to attribute a finding back to a specific project,
// session, and time. Each concrete AuditSource yields a stream of
// these.
//
// Content is the raw text to scan. Everything else is metadata the
// aggregator uses to group findings (SourceName), attribute to a
// project (ProjectDashName), timeline (Timestamp), and point the user
// at the exact location (Location).
type AuditItem struct {
	SourceName      string    // "transcript", "paste-cache", "prompt-history", "file-history"
	ProjectDashName string    // dash-encoded project dir name, or "" if unknown
	ProjectAbsPath  string    // absolute path decoded from DashName, or "" if unknown
	SessionID       string    // Claude Code session UUID, or "" if not session-scoped
	Location        string    // human-readable, e.g. "msg:42" or "paste-cache/abc.txt"
	Timestamp       time.Time // when this content was created (may be zero)
	Content         string    // the text to run through the detector
}

// AuditSource walks one of Claude Code's local data directories and
// yields AuditItems. Each source is independent; the aggregator
// orchestrates them.
//
// Sources MUST:
//   - Return ctx.Err() promptly when the context is cancelled.
//   - Best-effort on errors: log and continue rather than abort.
//   - Never panic on malformed input — Claude's formats evolve.
//   - Never leak raw secrets out-of-band (no logging Content, no
//     temp files containing Content).
//
// Sources MAY:
//   - Be called from multiple goroutines by the aggregator; they
//     must be safe to construct once and call Walk once.
type AuditSource interface {
	// Name identifies this source in Finding.Sources and in logs.
	// Returns a stable, lowercase, hyphenated string.
	Name() string

	// Walk streams items to out. It returns when it has exhausted
	// its surface or ctx is cancelled.
	//
	// selectedProjects is the opt-in list of project dash-names to
	// include. Empty means "all." Sources that are not project-
	// scoped (paste cache) ignore this filter.
	//
	// Walk closes nothing — the caller owns out.
	Walk(ctx context.Context, selectedProjects []string, out chan<- AuditItem) error
}
