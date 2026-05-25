package cmdinstall

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	auditpkg "github.com/Musubi42/shhh/internal/audit"
)

// ClaudeProject is the TUI-facing description of one Claude Code
// project that the user can opt in or out of at install time. A
// project is anything Claude Code has a directory for under
// ~/.claude/projects/ — it may or may not still exist on disk.
type ClaudeProject struct {
	DashName    string // raw dir name, e.g. "-Users-alice-work-backend"
	AbsPath     string // decoded, e.g. "/Users/alice/work/backend"
	DisplayPath string // tilde-abbreviated, e.g. "~/work/backend"
	OnDisk      bool   // whether AbsPath still exists
}

// ListClaudeProjects enumerates Claude Code's project directories,
// decoding each one's cwd and marking whether the folder still
// exists. Used by the interactive installer's multi-select.
//
// Sort order: on-disk projects alphabetically first, then archived
// (folder-gone) projects. Within each group, alphabetical by display
// path. Keeps the UI predictable across runs.
//
// Missing ~/.claude/projects/ is not an error — returns an empty
// slice. This matches the "no past sessions" case in the wireframes.
func ListClaudeProjects() ([]ClaudeProject, error) {
	root, err := auditpkg.ClaudeRoot()
	if err != nil {
		return nil, err
	}
	dir := auditpkg.ClaudeProjectsDir(root)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	out := make([]ClaudeProject, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dash := e.Name()
		abs := auditpkg.DecodeDashPath(dash)
		out = append(out, ClaudeProject{
			DashName:    dash,
			AbsPath:     abs,
			DisplayPath: auditpkg.TildePath(abs),
			OnDisk:      auditpkg.PathExists(abs),
		})
	}

	sort.Slice(out, func(i, j int) bool {
		if out[i].OnDisk != out[j].OnDisk {
			return out[i].OnDisk // on-disk first
		}
		return strings.ToLower(out[i].DisplayPath) < strings.ToLower(out[j].DisplayPath)
	})
	return out, nil
}

// ClaudeProjectsDirName is a thin wrapper so other files in this
// package can compute the path without importing audit. Kept so the
// eventual `uninstall` path can verify shhh state without pulling in
// the whole audit package.
func ClaudeProjectsDirName(claudeHome string) string {
	return filepath.Join(claudeHome, "projects")
}
