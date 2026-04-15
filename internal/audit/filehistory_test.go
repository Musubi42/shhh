package audit

import (
	"context"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func collectFileHistory(t *testing.T, root string, selected []string) []AuditItem {
	t.Helper()
	src := NewFileHistorySource(root)
	if src.Name() != "file-history" {
		t.Fatalf("Name()=%q, want %q", src.Name(), "file-history")
	}
	out := make(chan AuditItem, 32)
	errCh := make(chan error, 1)
	go func() {
		errCh <- src.Walk(context.Background(), selected, out)
		close(out)
	}()
	var items []AuditItem
	for item := range out {
		items = append(items, item)
	}
	if err := <-errCh; err != nil {
		t.Fatalf("Walk() error: %v", err)
	}
	return items
}

func writeFH(t *testing.T, root, sessionID, name string, body []byte) {
	t.Helper()
	dir := filepath.Join(root, "file-history", sessionID)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeSessionMeta(t *testing.T, root, sessionID, cwd string) {
	t.Helper()
	dir := filepath.Join(root, "sessions")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte(`{"cwd":"` + cwd + `"}`)
	if err := os.WriteFile(filepath.Join(dir, sessionID+".json"), body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestFileHistorySourceWalk(t *testing.T) {
	tmp := t.TempDir()
	// Two versions of the same tracked file AND two versions of a
	// different tracked file. The walker keeps only the LATEST per
	// path-hash — older versions were already part of earlier
	// transcripts and re-scanning them is duplicate work.
	writeFH(t, tmp, "sess-aaaa", "hash1@v1", []byte("password=foo"))
	writeFH(t, tmp, "sess-aaaa", "hash1@v2", []byte("password=bar"))
	writeFH(t, tmp, "sess-aaaa", "hash2@v1", []byte("other=baz"))
	writeSessionMeta(t, tmp, "sess-aaaa", "/test/project")

	items := collectFileHistory(t, tmp, nil)
	if len(items) != 2 {
		t.Fatalf("got %d items, want 2 (latest hash1@v2 + only hash2@v1)", len(items))
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Location < items[j].Location })

	var gotBar, gotBaz bool
	for _, it := range items {
		if it.SourceName != "file-history" {
			t.Errorf("SourceName=%q", it.SourceName)
		}
		if it.SessionID != "sess-aaaa" {
			t.Errorf("SessionID=%q", it.SessionID)
		}
		if it.ProjectAbsPath != "/test/project" {
			t.Errorf("ProjectAbsPath=%q", it.ProjectAbsPath)
		}
		if it.ProjectDashName != "-test-project" {
			t.Errorf("ProjectDashName=%q", it.ProjectDashName)
		}
		if it.Timestamp.IsZero() {
			t.Errorf("expected non-zero Timestamp")
		}
		if it.Content == "password=bar" {
			gotBar = true // latest version of hash1
		}
		if it.Content == "other=baz" {
			gotBaz = true // only version of hash2
		}
		if it.Content == "password=foo" {
			t.Errorf("older version should have been skipped: %q", it.Content)
		}
	}
	if !gotBar || !gotBaz {
		t.Errorf("missing expected contents: gotBar=%v gotBaz=%v", gotBar, gotBaz)
	}
}

func TestFileHistorySourceWithoutSessionMetadata(t *testing.T) {
	tmp := t.TempDir()
	writeFH(t, tmp, "sess-bbbb", "foo@v1", []byte("some file content"))

	items := collectFileHistory(t, tmp, nil)
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	it := items[0]
	if it.ProjectAbsPath != "" || it.ProjectDashName != "" {
		t.Errorf("expected empty project metadata, got abs=%q dash=%q", it.ProjectAbsPath, it.ProjectDashName)
	}
	if it.SessionID != "sess-bbbb" {
		t.Errorf("SessionID=%q", it.SessionID)
	}
	if it.Content != "some file content" {
		t.Errorf("Content=%q", it.Content)
	}
}

func TestFileHistorySourceSkipsBinary(t *testing.T) {
	tmp := t.TempDir()
	writeFH(t, tmp, "sess-cccc", "foo@v1", []byte{'a', 'b', 0, 'c'})

	items := collectFileHistory(t, tmp, nil)
	if len(items) != 0 {
		t.Fatalf("got %d items, want 0 (binary skipped)", len(items))
	}
}

func TestFileHistorySourceNoDir(t *testing.T) {
	tmp := t.TempDir()
	items := collectFileHistory(t, tmp, nil)
	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}
}

func TestFileHistorySourceRespectsSelectedProjects(t *testing.T) {
	tmp := t.TempDir()

	writeFH(t, tmp, "sess-aaaa", "h@v1", []byte("content-a"))
	writeSessionMeta(t, tmp, "sess-aaaa", "/test/project/a")

	writeFH(t, tmp, "sess-bbbb", "h@v1", []byte("content-b"))
	writeSessionMeta(t, tmp, "sess-bbbb", "/test/project/b")

	// Filter to project-a only.
	items := collectFileHistory(t, tmp, []string{"-test-project-a"})
	if len(items) != 1 {
		t.Fatalf("filtered: got %d items, want 1", len(items))
	}
	if items[0].ProjectAbsPath != "/test/project/a" {
		t.Errorf("got ProjectAbsPath=%q, want /test/project/a", items[0].ProjectAbsPath)
	}

	// Empty filter should return items from both sessions.
	all := collectFileHistory(t, tmp, []string{})
	if len(all) != 2 {
		t.Fatalf("empty filter: got %d items, want 2", len(all))
	}
}
