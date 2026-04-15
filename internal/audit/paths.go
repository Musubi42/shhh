package audit

import (
	"fmt"
	"os"
	"path/filepath"
)

// ClaudeRoot returns the root of Claude Code's local state directory.
// Respects $CLAUDE_CONFIG_DIR for tests and non-default installs.
// Default: ~/.claude.
func ClaudeRoot() (string, error) {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".claude"), nil
}

// Subdirectories of the Claude root that each audit source cares
// about. Each helper returns an absolute path suitable for os.ReadDir
// and friends. Non-existence is not an error at this level — the
// caller decides what to do with a missing dir.
func ClaudeProjectsDir(root string) string   { return filepath.Join(root, "projects") }
func ClaudePasteCacheDir(root string) string { return filepath.Join(root, "paste-cache") }
func ClaudeHistoryFile(root string) string   { return filepath.Join(root, "history.jsonl") }
func ClaudeFileHistoryDir(root string) string { return filepath.Join(root, "file-history") }
func ClaudeSessionsDir(root string) string   { return filepath.Join(root, "sessions") }

// AuditDir returns the directory where snapshots are stored.
// Respects $SHHH_AUDIT_DIR for tests. Default: ~/.shhh/audits.
func AuditDir() (string, error) {
	if dir := os.Getenv("SHHH_AUDIT_DIR"); dir != "" {
		return dir, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".shhh", "audits"), nil
}
