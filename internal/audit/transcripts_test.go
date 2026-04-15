package audit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const fixtureTranscript = `{"type":"permission-mode","permissionMode":"bypassPermissions","sessionId":"abc123"}
{"type":"user","sessionId":"abc123","timestamp":"2026-04-13T21:25:23.667Z","message":{"role":"user","content":"stripe live key sk_live_xyz"}}
{"type":"file-history-snapshot","messageId":"m1"}
{"type":"tool_result","sessionId":"abc123","timestamp":"2026-04-13T21:25:24.000Z","message":{"role":"tool","content":[{"type":"text","text":"AWS_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE"}]}}
`

func writeFixture(t *testing.T, root, dashProject, sessionFile, contents string) {
	t.Helper()
	dir := filepath.Join(root, "projects", dashProject)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, sessionFile), []byte(contents), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}
}

func collect(t *testing.T, src AuditSource, selected []string) []AuditItem {
	t.Helper()
	ctx := context.Background()
	out := make(chan AuditItem, 64)
	done := make(chan error, 1)
	go func() {
		done <- src.Walk(ctx, selected, out)
		close(out)
	}()
	var items []AuditItem
	for it := range out {
		items = append(items, it)
	}
	if err := <-done; err != nil {
		t.Fatalf("walk: %v", err)
	}
	return items
}

func TestTranscriptSourceWalk(t *testing.T) {
	tmp := t.TempDir()
	writeFixture(t, tmp, "-test-project", "abc123.jsonl", fixtureTranscript)

	src := NewTranscriptSource(tmp)
	items := collect(t, src, nil)

	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d: %+v", len(items), items)
	}
	for _, it := range items {
		if it.SourceName != "transcript" {
			t.Errorf("SourceName = %q, want transcript", it.SourceName)
		}
		if it.ProjectDashName != "-test-project" {
			t.Errorf("ProjectDashName = %q, want -test-project", it.ProjectDashName)
		}
		if it.ProjectAbsPath != "/test/project" {
			t.Errorf("ProjectAbsPath = %q, want /test/project", it.ProjectAbsPath)
		}
	}

	var userItem, toolItem *AuditItem
	for i := range items {
		if strings.Contains(items[i].Content, "sk_live_xyz") {
			userItem = &items[i]
		}
		if strings.Contains(items[i].Content, "AKIAIOSFODNN7EXAMPLE") {
			toolItem = &items[i]
		}
	}
	if userItem == nil {
		t.Errorf("no item contained sk_live_xyz; items=%+v", items)
	}
	if toolItem == nil {
		t.Errorf("no item contained AKIAIOSFODNN7EXAMPLE; items=%+v", items)
	}
}

func TestTranscriptSourceRespectsSelectedProjects(t *testing.T) {
	tmp := t.TempDir()
	writeFixture(t, tmp, "-alpha", "s1.jsonl", fixtureTranscript)
	writeFixture(t, tmp, "-beta", "s2.jsonl", fixtureTranscript)

	src := NewTranscriptSource(tmp)
	items := collect(t, src, []string{"-alpha"})
	if len(items) == 0 {
		t.Fatal("expected items from -alpha")
	}
	for _, it := range items {
		if it.ProjectDashName != "-alpha" {
			t.Errorf("unexpected project %q in filtered walk", it.ProjectDashName)
		}
	}
}

func TestTranscriptSourceSkipsMalformedLines(t *testing.T) {
	tmp := t.TempDir()
	mixed := `this is not json
{"type":"user","sessionId":"abc","timestamp":"2026-04-13T21:25:23.667Z","message":{"role":"user","content":"hello world"}}
`
	writeFixture(t, tmp, "-p", "s.jsonl", mixed)

	src := NewTranscriptSource(tmp)
	items := collect(t, src, nil)
	if len(items) != 1 {
		t.Fatalf("expected 1 item (malformed line skipped), got %d", len(items))
	}
	if !strings.Contains(items[0].Content, "hello world") {
		t.Errorf("content = %q, want hello world", items[0].Content)
	}
}
