package audit

import (
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
