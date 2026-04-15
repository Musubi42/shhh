package audit

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// maxFileHistoryFileSize caps individual file-history version files
// we will read. Source files (TS/Go/Python/JSON/YAML config) are
// virtually always well under this. Bundles, minified JS, and lockfiles
// exceed it and get skipped — the detector would produce garbage on
// them anyway (entropy noise overwhelms real secret signal).
const maxFileHistoryFileSize = 256 * 1024 // 256 KB

// maxFileHistoryFilesPerSession is a safety rail against pathological
// cases where a single session has thousands of version files. We
// already de-dupe to latest-per-path, but if a session edited many
// distinct files we still cap the scan.
const maxFileHistoryFilesPerSession = 200

// FileHistorySource walks Claude Code's file-history directory.
//
// Claude Code stores per-session snapshots of files it has edited at:
//
//	<claudeRoot>/file-history/<session-uuid>/<path-hash>@v<N>
//
// The left side of the filename is a hash of the edited file's path;
// the right side is a monotonic version number. Each file contains
// the raw content of the tracked file at that version.
//
// To attribute a version to a project, we try to resolve the session
// UUID to a cwd via <claudeRoot>/sessions/<session-uuid>.json (or
// .jsonl). If that lookup fails, we still emit the item with empty
// project metadata; the aggregator can cross-reference later.
type FileHistorySource struct {
	claudeRoot string
}

// NewFileHistorySource constructs a file-history source rooted at the
// given Claude Code state directory. If claudeRoot is empty, it calls
// ClaudeRoot() to resolve the default.
func NewFileHistorySource(claudeRoot string) *FileHistorySource {
	if claudeRoot == "" {
		if r, err := ClaudeRoot(); err == nil {
			claudeRoot = r
		}
	}
	return &FileHistorySource{claudeRoot: claudeRoot}
}

// Name implements AuditSource.
func (s *FileHistorySource) Name() string { return "file-history" }

// Walk implements AuditSource.
func (s *FileHistorySource) Walk(ctx context.Context, selectedProjects []string, out chan<- AuditItem) error {
	if s.claudeRoot == "" {
		return nil
	}
	dir := ClaudeFileHistoryDir(s.claudeRoot)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read file-history dir %s: %w", dir, err)
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
		sessionID := entry.Name()
		sessionDir := filepath.Join(dir, sessionID)

		absPath, resolved := resolveSessionProject(s.claudeRoot, sessionID)
		var dashName string
		if resolved {
			dashName = EncodeDashPath(absPath)
			if len(selected) > 0 {
				if _, ok := selected[dashName]; !ok {
					continue
				}
			}
		}
		// If not resolved, the filter does not apply — emit
		// unattributed items for the aggregator to reconcile later.

		files, err := os.ReadDir(sessionDir)
		if err != nil {
			log.Printf("file-history: read session dir %s: %v", sessionDir, err)
			continue
		}
		shortID := sessionID
		if len(shortID) > 8 {
			shortID = shortID[:8]
		}

		// Group versions by path-hash and keep only the latest per
		// tracked file. Rationale: each older @v<N> for the same path
		// was read by Claude at some point and thus already surfaces
		// in the transcript source, so re-scanning every historical
		// version is duplicate work. The latest version is still
		// valuable because it captures "what the file most recently
		// looked like according to Claude's view of the world,"
		// which may differ from what's on disk now.
		latestPerHash := map[string]string{} // hash → latest filename
		latestVersion := map[string]int{}    // hash → highest version seen
		for _, f := range files {
			if f.IsDir() {
				continue
			}
			fname := f.Name()
			atIdx := strings.Index(fname, "@v")
			if atIdx < 0 {
				continue
			}
			hash := fname[:atIdx]
			ver := parseVersionSuffix(fname[atIdx+2:])
			if existing, ok := latestVersion[hash]; !ok || ver > existing {
				latestVersion[hash] = ver
				latestPerHash[hash] = fname
			}
		}

		count := 0
		for _, fname := range latestPerHash {
			if err := ctx.Err(); err != nil {
				return err
			}
			if count >= maxFileHistoryFilesPerSession {
				break
			}
			count++
			filePath := filepath.Join(sessionDir, fname)
			if err := s.walkFile(ctx, filePath, sessionID, shortID, dashName, absPath, out); err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				log.Printf("file-history: walk file %s: %v", filePath, err)
			}
		}
	}
	return nil
}

// parseVersionSuffix parses the digits after "@v" in a filename like
// "abc123@v7". Returns 0 on parse failure, which is fine — we only
// use the result for relative ordering.
func parseVersionSuffix(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func (s *FileHistorySource) walkFile(ctx context.Context, path, sessionID, shortID, dashName, absPath string, out chan<- AuditItem) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat: %w", err)
	}
	if info.Size() == 0 {
		return nil
	}
	if info.Size() > maxFileHistoryFileSize {
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	head := make([]byte, 512)
	n, err := io.ReadFull(f, head)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return fmt.Errorf("read head: %w", err)
	}
	head = head[:n]
	if bytes.IndexByte(head, 0) >= 0 {
		return nil
	}
	rest, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	content := string(head) + string(rest)

	item := AuditItem{
		SourceName:      "file-history",
		ProjectDashName: dashName,
		ProjectAbsPath:  absPath,
		SessionID:       sessionID,
		Location:        fmt.Sprintf("file-history:%s:%s", shortID, filepath.Base(path)),
		Timestamp:       info.ModTime(),
		Content:         content,
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	case out <- item:
	}
	return nil
}

// sessionMeta is a permissive view of the shapes we might find in a
// session metadata file. Real Claude Code shapes vary; we try the
// obvious fields and give up quietly if none match.
type sessionMeta struct {
	Cwd         string `json:"cwd"`
	ProjectPath string `json:"projectPath"`
}

// resolveSessionProject tries to find the project cwd for a given
// session UUID by reading the corresponding sessions/ file. Returns
// ("", false) if the file is absent or no recognizable cwd field is
// present. This is a best-effort helper: failure is normal.
func resolveSessionProject(claudeRoot, sessionID string) (string, bool) {
	sessionsDir := ClaudeSessionsDir(claudeRoot)

	// Try .json first (single JSON object).
	jsonPath := filepath.Join(sessionsDir, sessionID+".json")
	if data, err := os.ReadFile(jsonPath); err == nil {
		var meta sessionMeta
		if err := json.Unmarshal(data, &meta); err == nil {
			if meta.Cwd != "" {
				return meta.Cwd, true
			}
			if meta.ProjectPath != "" {
				return meta.ProjectPath, true
			}
		}
	}

	// Try .jsonl (first line as JSON).
	jsonlPath := filepath.Join(sessionsDir, sessionID+".jsonl")
	if f, err := os.Open(jsonlPath); err == nil {
		defer f.Close()
		br := bufio.NewReader(f)
		line, err := br.ReadBytes('\n')
		if err != nil && err != io.EOF {
			return "", false
		}
		line = bytes.TrimSpace(line)
		if len(line) > 0 {
			var meta sessionMeta
			if err := json.Unmarshal(line, &meta); err == nil {
				if meta.Cwd != "" {
					return meta.Cwd, true
				}
				if meta.ProjectPath != "" {
					return meta.ProjectPath, true
				}
			}
		}
	}

	return "", false
}
