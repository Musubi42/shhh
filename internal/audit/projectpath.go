package audit

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// Claude Code encodes each project's absolute cwd as a directory name
// under ~/.claude/projects/ by replacing every "/" with "-". So
// "/Users/alice/work/backend" becomes "-Users-alice-work-backend".
//
// This encoding is lossy when a path contains a literal hyphen
// followed by a directory that collides with a real path segment.
// In practice this is rare enough that Claude Code uses the direct
// replacement and accepts the ambiguity. We do the same: decode
// greedily and fall back to best-effort when the decoded path
// doesn't exist on disk.

// DecodeDashPath turns a dash-encoded project dir name into an
// absolute path. Dash at position 0 becomes a leading slash; every
// subsequent dash becomes a slash.
//
// Examples:
//
//	DecodeDashPath("-Users-alice-work-backend")
//	  → "/Users/alice/work/backend"
//
//	DecodeDashPath("-tmp-throwaway")
//	  → "/tmp/throwaway"
func DecodeDashPath(dashName string) string {
	if dashName == "" {
		return ""
	}
	if dashName[0] != '-' {
		return dashName // not dash-encoded; return as-is
	}
	return strings.ReplaceAll(dashName, "-", "/")
}

// EncodeDashPath is the inverse of DecodeDashPath. Useful for tests
// and for computing the expected dir name of a known absolute path.
func EncodeDashPath(absPath string) string {
	return strings.ReplaceAll(absPath, "/", "-")
}

// ResolveProjectPath returns the canonical absolute path for a
// Claude Code project directory.
//
// It prefers the `cwd` field recorded in the project's transcripts
// (loss-less, written by Claude Code itself) and falls back to
// dash-decoding the directory name when no transcript is readable.
// The dash-decode fallback is lossy for paths that contain a literal
// hyphen — e.g. "/Users/alice/open-source/shhh" round-trips to
// "/Users/alice/open/source/shhh" — so the transcript path is
// strongly preferred whenever available.
//
// projectDir is the absolute path to <claudeRoot>/projects/<dashName>.
func ResolveProjectPath(dashName, projectDir string) string {
	if cwd := cwdFromAnyTranscript(projectDir); cwd != "" {
		return cwd
	}
	return DecodeDashPath(dashName)
}

// cwdFromAnyTranscript opens *.jsonl files in projectDir until one
// yields a non-empty "cwd" field. Returns "" if none does.
func cwdFromAnyTranscript(projectDir string) string {
	entries, err := os.ReadDir(projectDir)
	if err != nil {
		return ""
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		if cwd := scanFileForCwd(filepath.Join(projectDir, e.Name())); cwd != "" {
			return cwd
		}
	}
	return ""
}

// scanFileForCwd reads up to the first 64 lines of a JSONL transcript
// looking for a "cwd" field. Returns "" if not found or unreadable.
// Bounded line count keeps this cheap on huge transcripts.
func scanFileForCwd(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var probe struct {
		Cwd string `json:"cwd"`
	}
	for i := 0; i < 64 && scanner.Scan(); i++ {
		probe.Cwd = ""
		if json.Unmarshal(scanner.Bytes(), &probe) == nil && probe.Cwd != "" {
			return probe.Cwd
		}
	}
	return ""
}

// TildePath abbreviates a home-prefixed absolute path with "~". Used
// for every display-facing path in the audit output so screenshots
// don't leak the user's home dir name.
func TildePath(absPath string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return absPath
	}
	if absPath == home {
		return "~"
	}
	if strings.HasPrefix(absPath, home+string(filepath.Separator)) {
		return "~" + absPath[len(home):]
	}
	return absPath
}

// DashNameCouldMatchScope reports whether dash-name COULD decode
// to a path equal to or under any of the given scope roots. It is
// the cheap pre-filter used by `enumerateClaudeProjects` when the
// user passed positional scope paths — it lets the enumerator skip
// the per-project transcript read for projects that obviously
// can't be in scope.
//
// Why "could": dash-decoding is lossy (every `-` in the dash-name
// might be a real `-` in the path or a `/` separator). So the
// match has to treat `-` and `/` as interchangeable at each
// position. A return value of `true` means "worth doing the full
// resolve to confirm"; `false` means "definitely not in scope, skip".
// False positives are harmless (the post-enumeration scope filter
// in run.go will drop them); false negatives would be bugs.
//
// scopeRoots must be absolute, filepath.Clean-ed paths. Empty
// scopeRoots returns true (no scope filter = everything matches).
func DashNameCouldMatchScope(dashName string, scopeRoots []string) bool {
	if len(scopeRoots) == 0 {
		return true
	}
	decoded := DecodeDashPath(dashName)
	for _, root := range scopeRoots {
		if dashAmbiguousPrefix(decoded, root) {
			return true
		}
	}
	return false
}

// dashAmbiguousPrefix is true when s could equal-or-be-under prefix
// under dash/slash ambiguity. See DashNameCouldMatchScope for why.
func dashAmbiguousPrefix(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	for i := 0; i < len(prefix); i++ {
		if s[i] == prefix[i] {
			continue
		}
		// Treat `-` and `/` as the same separator class.
		if (s[i] == '-' || s[i] == '/') && (prefix[i] == '-' || prefix[i] == '/') {
			continue
		}
		return false
	}
	if len(s) == len(prefix) {
		return true
	}
	// Beyond the prefix length, the next character of s must itself
	// be a separator, otherwise prefix matches a substring rather
	// than a path-component boundary. (E.g. prefix `/work/backend`
	// must NOT match `/work/backend-other`.)
	nx := s[len(prefix)]
	return nx == '-' || nx == '/'
}

// PathExists reports whether absPath is a directory that still
// exists on disk. Used to decide the Archived status.
func PathExists(absPath string) bool {
	if absPath == "" {
		return false
	}
	st, err := os.Stat(absPath)
	if err != nil {
		return false
	}
	return st.IsDir()
}
