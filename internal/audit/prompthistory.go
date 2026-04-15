package audit

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"
)

// PromptHistorySource reads Claude Code's prompt history file at
// <claudeRoot>/history.jsonl. Each line records one prompt the user
// sent, along with any pasted blobs. Both are interesting for audit:
// pasted content frequently contains raw secrets.
type PromptHistorySource struct {
	claudeRoot string
}

// NewPromptHistorySource constructs the source rooted at the given
// Claude Code state directory. If claudeRoot is empty, it calls
// ClaudeRoot() to resolve the default.
func NewPromptHistorySource(claudeRoot string) *PromptHistorySource {
	if claudeRoot == "" {
		if r, err := ClaudeRoot(); err == nil {
			claudeRoot = r
		}
	}
	return &PromptHistorySource{claudeRoot: claudeRoot}
}

// Name implements AuditSource.
func (s *PromptHistorySource) Name() string { return "prompt-history" }

// promptHistoryLine is a permissive view of one history.jsonl row.
type promptHistoryLine struct {
	Display        string                     `json:"display"`
	PastedContents map[string]json.RawMessage `json:"pastedContents"`
	Timestamp      int64                      `json:"timestamp"`
	Project        string                     `json:"project"`
	SessionID      string                     `json:"sessionId"`
}

// Walk implements AuditSource.
func (s *PromptHistorySource) Walk(ctx context.Context, selectedProjects []string, out chan<- AuditItem) error {
	if s.claudeRoot == "" {
		return nil
	}
	path := ClaudeHistoryFile(s.claudeRoot)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("open prompt history %s: %w", path, err)
	}
	defer f.Close()

	selected := make(map[string]struct{}, len(selectedProjects))
	for _, p := range selectedProjects {
		selected[p] = struct{}{}
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)

	lineNo := 0
	for scanner.Scan() {
		lineNo++
		if err := ctx.Err(); err != nil {
			return err
		}
		raw := scanner.Bytes()
		if len(raw) == 0 {
			continue
		}
		var line promptHistoryLine
		if err := json.Unmarshal(raw, &line); err != nil {
			continue
		}

		// Skip lines with no interesting content.
		if line.Display == "" && len(line.PastedContents) == 0 {
			continue
		}

		dashName := EncodeDashPath(line.Project)
		if len(selected) > 0 {
			if _, ok := selected[dashName]; !ok {
				continue
			}
		}

		// Build the content: display field plus any pasted blobs.
		var b strings.Builder
		if line.Display != "" {
			b.WriteString(line.Display)
		}
		for _, val := range line.PastedContents {
			text := extractPastedText(val)
			if text == "" {
				continue
			}
			if b.Len() > 0 {
				b.WriteByte('\n')
			}
			b.WriteString(text)
		}

		// If after extraction there's still nothing, skip.
		if b.Len() == 0 {
			continue
		}

		var ts time.Time
		if line.Timestamp > 0 {
			ts = time.UnixMilli(line.Timestamp)
		}

		item := AuditItem{
			SourceName:      "prompt-history",
			ProjectDashName: dashName,
			ProjectAbsPath:  line.Project,
			SessionID:       line.SessionID,
			Location:        fmt.Sprintf("history:line:%d", lineNo),
			Timestamp:       ts,
			Content:         b.String(),
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- item:
		}
	}
	if err := scanner.Err(); err != nil {
		log.Printf("prompt-history: scan %s: %v", path, err)
	}
	return nil
}

// extractPastedText pulls text from a pastedContents value. Each
// value is typically an object with a "content" or "text" field, but
// may also be a bare string.
func extractPastedText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}
	var m map[string]interface{}
	if err := json.Unmarshal(raw, &m); err == nil {
		if c, ok := m["content"].(string); ok && c != "" {
			return c
		}
		if t, ok := m["text"].(string); ok && t != "" {
			return t
		}
	}
	return ""
}
