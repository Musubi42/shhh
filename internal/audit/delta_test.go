package audit

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDeltaFromCounts(t *testing.T) {
	t.Parallel()
	prevTime := time.Date(2026, 4, 12, 9, 0, 0, 0, time.UTC)

	cases := []struct {
		name       string
		prev, curr Summary
		want       Delta
	}{
		{
			name: "improved",
			prev: Summary{SecretsLeaked: 7, SecretsAtRisk: 6, ProjectsProtected: 0},
			curr: Summary{SecretsLeaked: 4, SecretsAtRisk: 2, ProjectsProtected: 1},
			want: Delta{
				Since:     prevTime,
				Leaked:    DeltaCount{Before: 7, After: 4, Change: -3},
				AtRisk:    DeltaCount{Before: 6, After: 2, Change: -4},
				Protected: DeltaCount{Before: 0, After: 1, Change: 1},
			},
		},
		{
			name: "regressed",
			prev: Summary{SecretsLeaked: 0},
			curr: Summary{SecretsLeaked: 2},
			want: Delta{
				Since:     prevTime,
				Leaked:    DeltaCount{Before: 0, After: 2, Change: 2},
				AtRisk:    DeltaCount{},
				Protected: DeltaCount{},
			},
		},
		{
			name: "unchanged",
			prev: Summary{SecretsLeaked: 3, SecretsAtRisk: 2, ProjectsProtected: 5},
			curr: Summary{SecretsLeaked: 3, SecretsAtRisk: 2, ProjectsProtected: 5},
			want: Delta{
				Since:     prevTime,
				Leaked:    DeltaCount{Before: 3, After: 3, Change: 0},
				AtRisk:    DeltaCount{Before: 2, After: 2, Change: 0},
				Protected: DeltaCount{Before: 5, After: 5, Change: 0},
			},
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := DeltaFromCounts(tc.prev, prevTime, tc.curr)
			if got == nil {
				t.Fatal("DeltaFromCounts returned nil")
			}
			if *got != tc.want {
				t.Errorf("\n got  = %+v\n want = %+v", *got, tc.want)
			}
		})
	}
}

func TestComputeDeltaNoPrevious(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	curr := sampleResult(time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC))
	d, err := ComputeDelta(dir, curr)
	if err != nil {
		t.Fatalf("ComputeDelta: %v", err)
	}
	if d != nil {
		t.Errorf("expected nil delta on first run, got %+v", d)
	}
}

func TestComputeDeltaRoundTrip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Previous audit: 5 leaked, 3 at-risk, 0 protected.
	tsA := time.Date(2026, 4, 12, 10, 0, 0, 0, time.UTC)
	a := sampleResult(tsA)
	a.Summary = Summary{SecretsLeaked: 5, SecretsAtRisk: 3, ProjectsProtected: 0}
	if _, err := SaveSnapshot(dir, a); err != nil {
		t.Fatalf("save A: %v", err)
	}

	// New audit: 2 leaked, 1 at-risk, 1 protected.
	tsB := time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC)
	b := sampleResult(tsB)
	b.Summary = Summary{SecretsLeaked: 2, SecretsAtRisk: 1, ProjectsProtected: 1}

	d, err := ComputeDelta(dir, b)
	if err != nil {
		t.Fatalf("ComputeDelta: %v", err)
	}
	if d == nil {
		t.Fatal("expected non-nil delta")
	}
	if d.Leaked.Before != 5 || d.Leaked.After != 2 || d.Leaked.Change != -3 {
		t.Errorf("Leaked = %+v, want {5,2,-3}", d.Leaked)
	}
	if d.AtRisk.Before != 3 || d.AtRisk.After != 1 || d.AtRisk.Change != -2 {
		t.Errorf("AtRisk = %+v, want {3,1,-2}", d.AtRisk)
	}
	if d.Protected.Before != 0 || d.Protected.After != 1 || d.Protected.Change != 1 {
		t.Errorf("Protected = %+v, want {0,1,1}", d.Protected)
	}
	if !d.Since.Equal(tsA) {
		t.Errorf("Since = %v, want %v", d.Since, tsA)
	}
}

func TestComputeDeltaSchemaMismatchIsSoft(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	body := []byte(`{"SchemaVersion":999,"Agent":"claude-code"}`)
	if err := os.WriteFile(filepath.Join(dir, "latest.json"), body, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	curr := sampleResult(time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC))
	d, err := ComputeDelta(dir, curr)
	if err != nil {
		t.Fatalf("expected nil error on schema mismatch, got %v", err)
	}
	if d != nil {
		t.Errorf("expected nil delta on schema mismatch, got %+v", d)
	}
}
