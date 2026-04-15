package audit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func collectPasteCache(t *testing.T, root string, selected []string) []AuditItem {
	t.Helper()
	src := NewPasteCacheSource(root)
	if src.Name() != "paste-cache" {
		t.Fatalf("Name()=%q, want %q", src.Name(), "paste-cache")
	}
	out := make(chan AuditItem, 16)
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

func TestPasteCacheSourceWalk(t *testing.T) {
	tmp := t.TempDir()
	pcDir := filepath.Join(tmp, "paste-cache")
	if err := os.MkdirAll(pcDir, 0o755); err != nil {
		t.Fatal(err)
	}

	writeFile := func(name string, body []byte) {
		if err := os.WriteFile(filepath.Join(pcDir, name), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	writeFile("abc1.txt", []byte(strings.Repeat("x", 50)+" hello STRIPE_LIVE_KEY content "+strings.Repeat("y", 20)))
	writeFile("abc2.txt", []byte{'a', 'b', 0, 'c', 'd'})
	writeFile("abc3.txt", []byte{})

	items := collectPasteCache(t, tmp, nil)
	if len(items) != 1 {
		t.Fatalf("got %d items, want 1", len(items))
	}
	it := items[0]
	if it.SourceName != "paste-cache" {
		t.Errorf("SourceName=%q", it.SourceName)
	}
	if !strings.HasPrefix(it.Location, "paste-cache:") {
		t.Errorf("Location=%q, want paste-cache: prefix", it.Location)
	}
	if !strings.Contains(it.Location, "abc1.txt") {
		t.Errorf("Location=%q, want abc1.txt", it.Location)
	}
	if !strings.Contains(it.Content, "STRIPE_LIVE_KEY") {
		t.Errorf("Content missing STRIPE_LIVE_KEY: %q", it.Content)
	}
	if it.ProjectDashName != "" || it.ProjectAbsPath != "" || it.SessionID != "" {
		t.Errorf("expected empty project metadata, got %+v", it)
	}
	if it.Timestamp.IsZero() {
		t.Errorf("expected non-zero Timestamp")
	}
}

func TestPasteCacheSourceNoDir(t *testing.T) {
	tmp := t.TempDir()
	items := collectPasteCache(t, tmp, nil)
	if len(items) != 0 {
		t.Fatalf("expected 0 items, got %d", len(items))
	}
}

func TestPasteCacheSourceIgnoresSelectedProjects(t *testing.T) {
	tmp := t.TempDir()
	pcDir := filepath.Join(tmp, "paste-cache")
	if err := os.MkdirAll(pcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pcDir, "a.txt"), []byte("something"), 0o644); err != nil {
		t.Fatal(err)
	}

	items := collectPasteCache(t, tmp, []string{"-some-project-that-does-not-exist"})
	if len(items) != 1 {
		t.Fatalf("expected 1 item even with selectedProjects filter, got %d", len(items))
	}
}

func TestPasteCacheSourceRespectsSizeCap(t *testing.T) {
	tmp := t.TempDir()
	pcDir := filepath.Join(tmp, "paste-cache")
	if err := os.MkdirAll(pcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	big := make([]byte, 5*1024*1024)
	for i := range big {
		big[i] = 'a'
	}
	if err := os.WriteFile(filepath.Join(pcDir, "big.txt"), big, 0o644); err != nil {
		t.Fatal(err)
	}

	items := collectPasteCache(t, tmp, nil)
	if len(items) != 0 {
		t.Fatalf("expected 0 items (oversize skipped), got %d", len(items))
	}
}
