package audit

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
)

// maxPasteCacheFileSize caps individual paste-cache files we will
// read. Anything bigger is almost certainly a binary blob or a
// programmatic dump, not a hand-paste we care about scanning.
const maxPasteCacheFileSize = 4 * 1024 * 1024 // 4MB

// PasteCacheSource walks Claude Code's paste-cache directory and
// yields one AuditItem per text file found there.
//
// Claude Code stores pasted content at:
//
//	<claudeRoot>/paste-cache/<content-hash>.txt
//
// The directory is flat and global: there is no per-project metadata
// attached to paste-cache files. The aggregator is responsible for
// cross-referencing paste-cache findings with transcript findings to
// attribute them to a specific project later.
type PasteCacheSource struct {
	claudeRoot string
}

// NewPasteCacheSource constructs a paste-cache source rooted at the
// given Claude Code state directory. If claudeRoot is empty, it calls
// ClaudeRoot() to resolve the default.
func NewPasteCacheSource(claudeRoot string) *PasteCacheSource {
	if claudeRoot == "" {
		if r, err := ClaudeRoot(); err == nil {
			claudeRoot = r
		}
	}
	return &PasteCacheSource{claudeRoot: claudeRoot}
}

// Name implements AuditSource.
func (s *PasteCacheSource) Name() string { return "paste-cache" }

// Walk implements AuditSource.
//
// NOTE: selectedProjects is intentionally ignored. Paste-cache files
// have no project metadata; filtering happens downstream via
// cross-reference with transcript findings.
func (s *PasteCacheSource) Walk(ctx context.Context, selectedProjects []string, out chan<- AuditItem) error {
	_ = selectedProjects // documented above: not project-scoped

	if s.claudeRoot == "" {
		return nil
	}
	dir := ClaudePasteCacheDir(s.claudeRoot)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read paste-cache dir %s: %w", dir, err)
	}

	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".txt") {
			continue
		}
		path := filepath.Join(dir, name)
		if err := s.walkFile(ctx, path, out); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			log.Printf("paste-cache: walk file %s: %v", path, err)
		}
	}
	return nil
}

func (s *PasteCacheSource) walkFile(ctx context.Context, path string, out chan<- AuditItem) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat: %w", err)
	}
	if info.Size() == 0 {
		return nil
	}
	if info.Size() > maxPasteCacheFileSize {
		return nil
	}

	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	defer f.Close()

	// Peek first 512 bytes for a NUL byte -> binary, skip.
	head := make([]byte, 512)
	n, err := io.ReadFull(f, head)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return fmt.Errorf("read head: %w", err)
	}
	head = head[:n]
	if bytes.IndexByte(head, 0) >= 0 {
		return nil
	}

	// Read the rest.
	rest, err := io.ReadAll(f)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}
	content := string(head) + string(rest)
	if content == "" {
		return nil
	}

	item := AuditItem{
		SourceName:      "paste-cache",
		ProjectDashName: "",
		ProjectAbsPath:  "",
		SessionID:       "",
		Location:        fmt.Sprintf("paste-cache:%s", filepath.Base(path)),
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
