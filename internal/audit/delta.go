package audit

import (
	"errors"
	"time"
)

// ComputeDelta compares a newly-produced Result against the most
// recent previous snapshot (from auditDir). Returns a *Delta with
// before/after/change for each of the three headline counters:
// Leaked, AtRisk, Protected.
//
// Returns (nil, nil) if there is no previous snapshot — the caller
// leaves result.Delta as nil on a first run.
//
// Schema mismatch on the previous snapshot is NOT treated as an
// error: it degrades to "no previous snapshot for comparison
// purposes" and returns (nil, nil). Only real I/O / decode errors
// are propagated.
func ComputeDelta(auditDir string, newResult *Result) (*Delta, error) {
	if newResult == nil {
		return nil, errors.New("ComputeDelta: nil result")
	}
	prev, err := LoadLatestSnapshot(auditDir)
	if err != nil {
		if errors.Is(err, ErrSchemaMismatch) {
			return nil, nil
		}
		return nil, err
	}
	if prev == nil {
		return nil, nil
	}
	return DeltaFromCounts(prev.Summary, prev.AuditTime, newResult.Summary), nil
}

// DeltaFromCounts builds a Delta given two Summaries (previous and
// current) and the previous audit's timestamp. Pure function, no I/O.
func DeltaFromCounts(prev Summary, prevTime time.Time, curr Summary) *Delta {
	return &Delta{
		Since: prevTime,
		Leaked: DeltaCount{
			Before: prev.SecretsLeaked,
			After:  curr.SecretsLeaked,
			Change: curr.SecretsLeaked - prev.SecretsLeaked,
		},
		AtRisk: DeltaCount{
			Before: prev.SecretsAtRisk,
			After:  curr.SecretsAtRisk,
			Change: curr.SecretsAtRisk - prev.SecretsAtRisk,
		},
		Protected: DeltaCount{
			Before: prev.ProjectsProtected,
			After:  curr.ProjectsProtected,
			Change: curr.ProjectsProtected - prev.ProjectsProtected,
		},
	}
}
