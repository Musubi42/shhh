package cmdhook

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// runCursorT mirrors runClaude / runCodexT — same env isolation
// pattern (a throwaway SHHH_CONFIG_DIR pinning engines to
// shhh-native), same JSON marshal/unmarshal round trip. The
// payload uses Cursor's conventions: snake_case fields, no
// session_id (Cursor uses conversation_id + generation_id).
func runCursorT(t *testing.T, payload any) map[string]any {
	t.Helper()
	if os.Getenv("SHHH_CONFIG_DIR") == "" {
		cfgDir := t.TempDir()
		if err := os.WriteFile(filepath.Join(cfgDir, "config.json"),
			[]byte(`{"version":1,"engines":["shhh-native"]}`), 0o600); err != nil {
			t.Fatal(err)
		}
		t.Setenv("SHHH_CONFIG_DIR", cfgDir)
	}
	in, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runCursor(bytes.NewReader(in), &out); err != nil {
		t.Fatalf("runCursor: %v", err)
	}
	if out.Len() == 0 {
		t.Fatal("no output written")
	}
	var resp map[string]any
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response %q: %v", out.String(), err)
	}
	return resp
}

// TestCursorShellCommandWrapped is the v1 Shell happy path. The
// response shape is Cursor's flat envelope:
//
//	{ "permission": "allow", "updated_input": { "command": "..." } }
//
// — no `hookSpecificOutput` wrapper, no `permissionDecision`. Tool
// name on Cursor is `Shell`, not `Bash`.
func TestCursorShellCommandWrapped(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	resp := runCursorT(t, map[string]any{
		"conversation_id": "cursor-conv-01",
		"hook_event_name": "preToolUse",
		"tool_name":       "Shell",
		"tool_input": map[string]any{
			"command":           "cat .env",
			"working_directory": "/project",
		},
	})
	if resp["permission"] != "allow" {
		t.Errorf("permission: %v", resp["permission"])
	}
	ui, ok := resp["updated_input"].(map[string]any)
	if !ok {
		t.Fatalf("expected updated_input, got %v", resp)
	}
	cmd, _ := ui["command"].(string)
	for _, want := range []string{"cat .env", "redact --session", "cursor-conv-01"} {
		if !contains(cmd, want) {
			t.Errorf("wrapped command missing %q: %q", want, cmd)
		}
	}
	// working_directory pass-through preserved.
	if ui["working_directory"] != "/project" {
		t.Errorf("working_directory not passed through: %v", ui["working_directory"])
	}
}

// TestCursorReadRedactsSecret is the v1 Read happy path. The hook
// redirects tool_input.file_path to a per-session cache copy and
// emits agent_message with the redaction narration (Cursor's
// equivalent of Claude Code's additionalContext).
func TestCursorReadRedactsSecret(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	envPath := writeFixtureEnv(t, t.TempDir())

	resp := runCursorT(t, map[string]any{
		"conversation_id": "cursor-conv-02",
		"hook_event_name": "preToolUse",
		"tool_name":       "Read",
		"tool_input":      map[string]any{"file_path": envPath},
	})
	if resp["permission"] != "allow" {
		t.Errorf("permission: %v", resp["permission"])
	}
	ui, ok := resp["updated_input"].(map[string]any)
	if !ok {
		t.Fatalf("expected updated_input, got %v", resp)
	}
	newPath, _ := ui["file_path"].(string)
	if newPath == "" || newPath == envPath {
		t.Errorf("file_path not redirected: %q", newPath)
	}
	// The cache copy must exist and be different from the
	// original (we cannot string-compare against the fixture
	// secret because the fixture itself uses a placeholder-shaped
	// token to dogfood the detector — the redactor wraps it again
	// with a fresh placeholder rather than removing the
	// `sk_live_` substring).
	original, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("read original: %v", err)
	}
	body, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read redacted: %v", err)
	}
	if string(body) == string(original) {
		t.Errorf("redacted file is identical to original: %s", body)
	}
	if !contains(string(body), "[STRIPE_LIVE_KEY:") {
		t.Errorf("redacted file missing placeholder pattern: %s", body)
	}
	agentMsg, _ := resp["agent_message"].(string)
	for _, want := range []string{"STRIPE_LIVE_KEY", "shhh", "Bash"} {
		if !contains(agentMsg, want) {
			t.Errorf("agent_message missing %q: %s", want, agentMsg)
		}
	}
}

// TestCursorReadCleanFilePassesThrough — clean files take no
// rewrite, so the hook emits the empty response and Cursor reads
// the file unmodified.
func TestCursorReadCleanFilePassesThrough(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	dir := t.TempDir()
	clean := filepath.Join(dir, "clean.txt")
	if err := os.WriteFile(clean, []byte("hello world\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resp := runCursorT(t, map[string]any{
		"conversation_id": "cursor-conv-03",
		"hook_event_name": "preToolUse",
		"tool_name":       "Read",
		"tool_input":      map[string]any{"file_path": clean},
	})
	if len(resp) != 0 {
		t.Errorf("clean file should pass through, got %v", resp)
	}
}

// TestCursorWritePassesThrough — the v1 hook does not handle
// Write yet; it falls through to writeEmpty so the tool call
// proceeds verbatim. Documents the gap for a future handler.
func TestCursorWritePassesThrough(t *testing.T) {
	resp := runCursorT(t, map[string]any{
		"conversation_id": "cursor-conv-04",
		"hook_event_name": "preToolUse",
		"tool_name":       "Write",
		"tool_input":      map[string]any{"file_path": "/tmp/x", "content": "y"},
	})
	if len(resp) != 0 {
		t.Errorf("Write should pass through (no handler in v1), got %v", resp)
	}
}

// TestCursorMCPToolPassesThrough — MCP tools fire preToolUse on
// Cursor (same as on Codex/Claude Code), but shhh has no per-tool
// redaction for them.
func TestCursorMCPToolPassesThrough(t *testing.T) {
	resp := runCursorT(t, map[string]any{
		"conversation_id": "cursor-conv-05",
		"hook_event_name": "preToolUse",
		"tool_name":       "mcp__example__do",
		"tool_input":      map[string]any{"arg": "v"},
	})
	if len(resp) != 0 {
		t.Errorf("MCP tool should pass through, got %v", resp)
	}
}

// TestCursorSessionEndWipes confirms the cleanup hook fires on
// either `SessionEnd` or `stop` — Cursor's docs use both names
// across versions; we accept both.
func TestCursorSessionEndWipes(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	for _, eventName := range []string{"SessionEnd", "stop"} {
		resp := runCursorT(t, map[string]any{
			"conversation_id": "cursor-conv-06",
			"hook_event_name": eventName,
		})
		if len(resp) != 0 {
			t.Errorf("%s should return empty response, got %v", eventName, resp)
		}
	}
}

// TestCursorMalformedInputFailsClosed — garbage stdin must not
// block the tool call. Mirrors Claude Code / Codex behaviour:
// emit `{}` and exit 0 (fail-open by design, see the
// best-effort-non-blocking contract in cursor.go).
func TestCursorMalformedInputFailsClosed(t *testing.T) {
	var out bytes.Buffer
	if err := runCursor(bytes.NewReader([]byte("not json")), &out); err != nil {
		t.Fatalf("runCursor: %v", err)
	}
	if out.String() != "{}\n" {
		t.Errorf("expected empty response on malformed input, got %q", out.String())
	}
}

// TestCursorConversationIDIsThreadedThroughShellWrap — the
// conversation_id (Cursor's equivalent of session_id) must reach
// the wrapped Shell command via `redact --session`.
func TestCursorConversationIDIsThreadedThroughShellWrap(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	resp := runCursorT(t, map[string]any{
		"conversation_id": "abc-def-123",
		"hook_event_name": "preToolUse",
		"tool_name":       "Shell",
		"tool_input":      map[string]any{"command": "echo hi"},
	})
	ui := resp["updated_input"].(map[string]any)
	if !contains(ui["command"].(string), `--session "abc-def-123"`) {
		t.Errorf("conversation_id not threaded to redact: %q", ui["command"])
	}
}
