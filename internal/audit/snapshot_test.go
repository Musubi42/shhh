package audit

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mustMarshal(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return string(b)
}

func sampleResult(ts time.Time) *Result {
	return &Result{
		SchemaVersion: CurrentSchemaVersion,
		Agent:         "claude-code",
		AuditTime:     ts,
		ScanDuration:  250 * time.Millisecond,
		Projects: []Project{
			{
				AbsPath:     "/work/backend",
				DisplayPath: "~/work/backend",
				DashName:    "-work-backend",
				Status:      StatusUnprotected,
				OnDisk:      true,
				Leaked: []Finding{
					{Placeholder: "[STRIPE:sk_live_...:aa]", Label: "STRIPE", Severity: SevLeaked, Occurrences: 2},
				},
			},
		},
		Summary: Summary{
			ProjectsTotal:       1,
			ProjectsUnprotected: 1,
			SecretsLeaked:       1,
		},
	}
}

func TestSaveAndLoadSnapshot(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ts := time.Date(2026, 4, 13, 10, 20, 30, 0, time.UTC)
	r := sampleResult(ts)

	path, err := SaveSnapshot(dir, r)
	if err != nil {
		t.Fatalf("SaveSnapshot: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("snapshot file missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "latest.json")); err != nil {
		t.Fatalf("latest.json missing: %v", err)
	}

	loaded, err := LoadLatestSnapshot(dir)
	if err != nil {
		t.Fatalf("LoadLatestSnapshot: %v", err)
	}
	if loaded == nil {
		t.Fatal("expected non-nil loaded result")
	}

	if got, want := mustMarshal(t, loaded), mustMarshal(t, r); got != want {
		t.Errorf("round-trip mismatch:\n got  = %s\n want = %s", got, want)
	}
}

func TestLoadLatestSnapshotNone(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	r, err := LoadLatestSnapshot(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r != nil {
		t.Errorf("expected nil result, got %+v", r)
	}
}

func TestLoadLatestSnapshotSchemaMismatch(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	body := []byte(`{"SchemaVersion":999,"Agent":"claude-code"}`)
	if err := os.WriteFile(filepath.Join(dir, "latest.json"), body, 0o600); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := LoadLatestSnapshot(dir)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, ErrSchemaMismatch) {
		t.Errorf("expected ErrSchemaMismatch, got %v", err)
	}
}

func TestSaveSnapshotHandlesCollision(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	ts := time.Date(2026, 4, 13, 12, 30, 45, 0, time.UTC)

	r1 := sampleResult(ts)
	p1, err := SaveSnapshot(dir, r1)
	if err != nil {
		t.Fatalf("SaveSnapshot 1: %v", err)
	}

	r2 := sampleResult(ts)
	r2.Agent = "claude-code-2"
	p2, err := SaveSnapshot(dir, r2)
	if err != nil {
		t.Fatalf("SaveSnapshot 2: %v", err)
	}

	if p1 == p2 {
		t.Fatalf("collision not handled: both at %s", p1)
	}
	if _, err := os.Stat(p1); err != nil {
		t.Errorf("first snapshot missing: %v", err)
	}
	if _, err := os.Stat(p2); err != nil {
		t.Errorf("second snapshot missing: %v", err)
	}
	if !strings.Contains(filepath.Base(p2), "-2.json") {
		t.Errorf("expected second snapshot to carry -2 suffix, got %s", filepath.Base(p2))
	}

	// latest.json should point to p2.
	loaded, err := LoadLatestSnapshot(dir)
	if err != nil {
		t.Fatalf("LoadLatestSnapshot: %v", err)
	}
	if loaded.Agent != "claude-code-2" {
		t.Errorf("latest.json points to wrong snapshot: agent=%s", loaded.Agent)
	}
}

func TestListSnapshots(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	times := []time.Time{
		time.Date(2026, 4, 13, 10, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 13, 11, 0, 0, 0, time.UTC),
		time.Date(2026, 4, 13, 12, 0, 0, 0, time.UTC),
	}
	for _, ts := range times {
		if _, err := SaveSnapshot(dir, sampleResult(ts)); err != nil {
			t.Fatalf("save %v: %v", ts, err)
		}
	}
	names, err := ListSnapshots(dir)
	if err != nil {
		t.Fatalf("ListSnapshots: %v", err)
	}
	if len(names) != 3 {
		t.Fatalf("expected 3 snapshots, got %d: %v", len(names), names)
	}
	for _, n := range names {
		if n == "latest.json" {
			t.Errorf("latest.json should be excluded")
		}
	}
	// Oldest-first: name starting with 10-hour should come first.
	if !strings.Contains(names[0], "10-00-00") {
		t.Errorf("expected oldest first, got %v", names)
	}
	if !strings.Contains(names[2], "12-00-00") {
		t.Errorf("expected newest last, got %v", names)
	}
}
