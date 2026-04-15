package audit

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPromptHistorySourceWalk(t *testing.T) {
	tmp := t.TempDir()
	// Line 1: display-only.
	// Line 2: pastedContents with a "content" field.
	// Line 3: empty both → should be skipped.
	fixture := `{"display":"/model","pastedContents":{},"timestamp":1774010392796,"project":"/Users/alice/proj","sessionId":"sess-1"}
{"display":"here is my key","pastedContents":{"p1":{"content":"AWS_SECRET=supersecret"}},"timestamp":1774010400000,"project":"/Users/alice/proj","sessionId":"sess-2"}
{"display":"","pastedContents":{},"timestamp":1774010500000,"project":"/Users/alice/proj","sessionId":"sess-3"}
`
	if err := os.WriteFile(filepath.Join(tmp, "history.jsonl"), []byte(fixture), 0o644); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	src := NewPromptHistorySource(tmp)
	items := collect(t, src, nil)
	if len(items) != 2 {
		t.Fatalf("expected 2 items, got %d: %+v", len(items), items)
	}

	for _, it := range items {
		if it.SourceName != "prompt-history" {
			t.Errorf("SourceName = %q, want prompt-history", it.SourceName)
		}
		if it.ProjectAbsPath != "/Users/alice/proj" {
			t.Errorf("ProjectAbsPath = %q, want /Users/alice/proj", it.ProjectAbsPath)
		}
		if it.ProjectDashName != "-Users-alice-proj" {
			t.Errorf("ProjectDashName = %q, want -Users-alice-proj", it.ProjectDashName)
		}
		if it.Timestamp.IsZero() {
			t.Errorf("Timestamp is zero, want non-zero")
		}
	}

	if !strings.Contains(items[0].Content, "/model") {
		t.Errorf("first item content = %q, want /model", items[0].Content)
	}
	if items[0].Timestamp.UnixMilli() != 1774010392796 {
		t.Errorf("Timestamp millis = %d, want 1774010392796", items[0].Timestamp.UnixMilli())
	}
	if !strings.Contains(items[1].Content, "supersecret") {
		t.Errorf("second item content = %q, want to contain supersecret", items[1].Content)
	}
	if !strings.Contains(items[1].Content, "here is my key") {
		t.Errorf("second item content = %q, want to contain display field", items[1].Content)
	}
}

func TestPromptHistorySourceNoFile(t *testing.T) {
	tmp := t.TempDir()
	src := NewPromptHistorySource(tmp)
	items := collect(t, src, nil)
	if len(items) != 0 {
		t.Fatalf("expected 0 items from empty root, got %d", len(items))
	}
}
