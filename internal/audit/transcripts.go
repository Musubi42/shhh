package audit

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TranscriptSource walks Claude Code session transcripts stored at
// <claudeRoot>/projects/<dash-encoded-cwd>/<session-uuid>.jsonl.
//
// Each JSONL line is a message of variable shape. We extract text
// from user / assistant / tool_use / tool_result messages and emit
// one AuditItem per message. Unknown message types are skipped.
type TranscriptSource struct {
	claudeRoot string
}

// NewTranscriptSource constructs a transcript source rooted at the
// given Claude Code state directory (usually ~/.claude). If
// claudeRoot is empty, it calls ClaudeRoot() to resolve the default.
func NewTranscriptSource(claudeRoot string) *TranscriptSource {
	if claudeRoot == "" {
		if r, err := ClaudeRoot(); err == nil {
			claudeRoot = r
		}
	}
	return &TranscriptSource{claudeRoot: claudeRoot}
}

// Name implements AuditSource.
func (s *TranscriptSource) Name() string { return "transcript" }

// transcriptLine is a permissive view of a transcript JSONL row.
// Every field is optional; unrelated fields are ignored silently.
type transcriptLine struct {
	Type      string `json:"type"`
	Cwd       string `json:"cwd"`
	SessionID string `json:"sessionId"`
	Timestamp string `json:"timestamp"`
	UUID      string `json:"uuid"`
	Message   struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// Walk implements AuditSource.
func (s *TranscriptSource) Walk(ctx context.Context, selectedProjects []string, out chan<- AuditItem) error {
	if s.claudeRoot == "" {
		return nil
	}
	projectsDir := ClaudeProjectsDir(s.claudeRoot)
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read projects dir %s: %w", projectsDir, err)
	}

	selected := make(map[string]struct{}, len(selectedProjects))
	for _, p := range selectedProjects {
		selected[p] = struct{}{}
	}

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !entry.IsDir() {
			continue
		}
		dashName := entry.Name()
		if len(selected) > 0 {
			if _, ok := selected[dashName]; !ok {
				continue
			}
		}
		projectDir := filepath.Join(projectsDir, dashName)
		absPath := DecodeDashPath(dashName)

		files, err := os.ReadDir(projectDir)
		if err != nil {
			log.Printf("transcripts: read project dir %s: %v", projectDir, err)
			continue
		}
		for _, f := range files {
			if err := ctx.Err(); err != nil {
				return err
			}
			if f.IsDir() || !strings.HasSuffix(f.Name(), ".jsonl") {
				continue
			}
			filePath := filepath.Join(projectDir, f.Name())
			fallbackSession := strings.TrimSuffix(f.Name(), ".jsonl")
			if err := s.walkFile(ctx, filePath, dashName, absPath, fallbackSession, out); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				log.Printf("transcripts: walk file %s: %v", filePath, err)
			}
		}
	}
	return nil
}

func (s *TranscriptSource) walkFile(ctx context.Context, filePath, dashName, absPath, fallbackSession string, out chan<- AuditItem) error {
	f, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer f.Close()

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
		var line transcriptLine
		if err := json.Unmarshal(raw, &line); err != nil {
			// Malformed line — skip silently, don't log content.
			continue
		}
		switch line.Type {
		case "user", "assistant", "tool_use", "tool_result":
		default:
			continue
		}

		content := extractText(line.Message.Content)

		sessionID := line.SessionID
		if sessionID == "" {
			sessionID = fallbackSession
		}
		shortID := sessionID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}

		var ts time.Time
		if line.Timestamp != "" {
			if t, err := time.Parse(time.RFC3339Nano, line.Timestamp); err == nil {
				ts = t
			} else if t, err := time.Parse(time.RFC3339, line.Timestamp); err == nil {
				ts = t
			}
		}

		item := AuditItem{
			SourceName:      "transcript",
			ProjectDashName: dashName,
			ProjectAbsPath:  absPath,
			SessionID:       sessionID,
			Location:        fmt.Sprintf("session:%s:line:%d", shortID, lineNo),
			Timestamp:       ts,
			Content:         content,
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case out <- item:
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("scan: %w", err)
	}
	return nil
}

// extractText pulls text out of a message.content field that may be
// a JSON string, an array of content parts, or null. It is
// intentionally over-inclusive: the goal is to surface anything that
// might contain a leaked secret.
func extractText(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}

	// Try as a bare string.
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return s
	}

	// Try as an array of content parts.
	var parts []map[string]interface{}
	if err := json.Unmarshal(raw, &parts); err == nil {
		var b strings.Builder
		for _, p := range parts {
			if t, ok := p["text"].(string); ok && t != "" {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				b.WriteString(t)
				continue
			}
			if c, ok := p["content"].(string); ok && c != "" {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				b.WriteString(c)
				continue
			}
			if input, ok := p["input"]; ok {
				if buf, err := json.Marshal(input); err == nil {
					if b.Len() > 0 {
						b.WriteByte('\n')
					}
					b.Write(buf)
					continue
				}
			}
			// Fall back: serialize the whole part.
			if buf, err := json.Marshal(p); err == nil {
				if b.Len() > 0 {
					b.WriteByte('\n')
				}
				b.Write(buf)
			}
		}
		return b.String()
	}

	// Last resort: return the raw JSON bytes as a string.
	return string(raw)
}
