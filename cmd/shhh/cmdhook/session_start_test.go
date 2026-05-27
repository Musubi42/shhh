package cmdhook

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionStart_WritesSessionIDToClaudeEnvFile(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	envFile := filepath.Join(t.TempDir(), "env")
	t.Setenv("CLAUDE_ENV_FILE", envFile)

	runClaude(t, map[string]any{
		"session_id":      "sess-export",
		"hook_event_name": "SessionStart",
	})

	body, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("read env file: %v", err)
	}
	if !strings.Contains(string(body), "SHHH_SESSION_ID=sess-export") {
		t.Errorf("env file missing SHHH_SESSION_ID assignment, got: %q", body)
	}
}

func TestSessionStart_NoEnvFileIsSilentNoop(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	t.Setenv("CLAUDE_ENV_FILE", "")

	resp := runClaude(t, map[string]any{
		"session_id":      "sess-noenv",
		"hook_event_name": "SessionStart",
	})
	if len(resp) != 0 {
		t.Errorf("expected empty response when CLAUDE_ENV_FILE unset, got %v", resp)
	}
}

func TestSessionStart_MissingSessionIDIsNoop(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	envFile := filepath.Join(t.TempDir(), "env")
	t.Setenv("CLAUDE_ENV_FILE", envFile)

	runClaude(t, map[string]any{
		"hook_event_name": "SessionStart",
	})
	// No file should have been created.
	if _, err := os.Stat(envFile); !os.IsNotExist(err) {
		t.Errorf("env file should not be created without session_id, stat err = %v", err)
	}
}

func TestSessionStart_AppendsRatherThanOverwrites(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	envFile := filepath.Join(t.TempDir(), "env")
	// Simulate another hook having already written to the env file.
	if err := os.WriteFile(envFile, []byte("OTHER_VAR=pre-existing\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CLAUDE_ENV_FILE", envFile)

	runClaude(t, map[string]any{
		"session_id":      "sess-append",
		"hook_event_name": "SessionStart",
	})
	body, err := os.ReadFile(envFile)
	if err != nil {
		t.Fatalf("read env file: %v", err)
	}
	got := string(body)
	if !strings.Contains(got, "OTHER_VAR=pre-existing") {
		t.Errorf("pre-existing content was clobbered: %q", got)
	}
	if !strings.Contains(got, "SHHH_SESSION_ID=sess-append") {
		t.Errorf("session id not appended: %q", got)
	}
}
