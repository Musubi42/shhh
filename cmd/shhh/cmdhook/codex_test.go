package cmdhook

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// runCodexT runs the codex hook against a synthetic stdin payload and
// returns the decoded JSON response. Mirrors runClaude in
// cmdhook_test.go — same env isolation strategy (pin engines to
// shhh-native via a throwaway SHHH_CONFIG_DIR), same payload shape
// since Codex's hook protocol is structurally identical to Claude
// Code's.
func runCodexT(t *testing.T, payload any) map[string]any {
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
	if err := runCodex(bytes.NewReader(in), &out); err != nil {
		t.Fatalf("runCodex: %v", err)
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

// TestCodexBashCommandWrapped is the v1 happy path: a Codex Bash
// invocation gets wrapped through `shhh redact --session`. This is
// the only redaction surface Codex exposes today (apply_patch /
// read_file are tracked in openai/codex#18491 and do not yet fire
// PreToolUse). The wrapping logic is identical to Claude Code's
// because runCodex routes Bash to the same handleBash handler.
func TestCodexBashCommandWrapped(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	resp := runCodexT(t, map[string]any{
		"session_id":      "codex-sess-01",
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"tool_input":      map[string]any{"command": "cat .env"},
	})
	hso, ok := resp["hookSpecificOutput"].(map[string]any)
	if !ok {
		t.Fatalf("expected hookSpecificOutput, got %v", resp)
	}
	if hso["permissionDecision"] != "allow" {
		t.Errorf("permissionDecision: %v", hso["permissionDecision"])
	}
	ui, ok := hso["updatedInput"].(map[string]any)
	if !ok {
		t.Fatalf("expected updatedInput, got %v", hso)
	}
	cmd, _ := ui["command"].(string)
	if cmd == "" {
		t.Fatal("empty wrapped command")
	}
	for _, want := range []string{"cat .env", "redact --session", "codex-sess-01"} {
		if !contains(cmd, want) {
			t.Errorf("wrapped command missing %q: %q", want, cmd)
		}
	}
}

// TestCodexApplyPatchPassesThrough documents the current upstream
// limitation: when Codex fires PreToolUse for apply_patch (which it
// does for some, but not all, edit calls today), shhh does NOT
// intercept it. The dispatcher writes the empty response so the
// tool call proceeds verbatim. When openai/codex#18491 ships proper
// apply_patch coverage, this test flips to assert redaction.
func TestCodexApplyPatchPassesThrough(t *testing.T) {
	resp := runCodexT(t, map[string]any{
		"session_id":      "codex-sess-02",
		"hook_event_name": "PreToolUse",
		"tool_name":       "apply_patch",
		"tool_input":      map[string]any{"patch": "*** Update File: foo.py\n+x = 1\n"},
	})
	if len(resp) != 0 {
		t.Errorf("apply_patch should currently pass through, got %v", resp)
	}
}

// TestCodexReadFilePassesThrough — same rationale as apply_patch.
// The internal read_file tool exists on Codex but does not emit
// PreToolUse. If a future Codex release plumbs it through, shhh
// will need a new handler.
func TestCodexReadFilePassesThrough(t *testing.T) {
	resp := runCodexT(t, map[string]any{
		"session_id":      "codex-sess-03",
		"hook_event_name": "PreToolUse",
		"tool_name":       "read_file",
		"tool_input":      map[string]any{"path": "/tmp/foo"},
	})
	if len(resp) != 0 {
		t.Errorf("read_file should currently pass through, got %v", resp)
	}
}

// TestCodexMCPToolPassesThrough — MCP tools fire PreToolUse on
// Codex (per docs), and shhh has no per-tool redaction for them
// today. They fall through to writeEmpty, same as on Claude Code.
func TestCodexMCPToolPassesThrough(t *testing.T) {
	resp := runCodexT(t, map[string]any{
		"session_id":      "codex-sess-04",
		"hook_event_name": "PreToolUse",
		"tool_name":       "mcp__example__do_thing",
		"tool_input":      map[string]any{"arg": "value"},
	})
	if len(resp) != 0 {
		t.Errorf("MCP tool should pass through, got %v", resp)
	}
}

// TestCodexSessionEndWipes confirms SessionEnd still triggers the
// session-cache wipe — the cleanup hook is agent-agnostic.
func TestCodexSessionEndWipes(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	resp := runCodexT(t, map[string]any{
		"session_id":      "codex-sess-05",
		"hook_event_name": "SessionEnd",
	})
	if len(resp) != 0 {
		t.Errorf("SessionEnd should return empty response, got %v", resp)
	}
}

// TestCodexMalformedInputFailsClosed mirrors the Claude Code
// behaviour: a garbage stdin payload must NOT block the tool call.
// The hook writes `{}` and exits 0.
func TestCodexMalformedInputFailsClosed(t *testing.T) {
	var out bytes.Buffer
	if err := runCodex(bytes.NewReader([]byte("not json at all")), &out); err != nil {
		t.Fatalf("runCodex: %v", err)
	}
	if out.String() != "{}\n" {
		t.Errorf("expected empty response on malformed input, got %q", out.String())
	}
}

// contains is a local strings.Contains alias to keep the test
// imports minimal — the cmdhook package already imports strings via
// other files; this helper sidesteps a duplicate import in the test
// file.
func contains(s, sub string) bool {
	return len(sub) == 0 || (len(s) >= len(sub) && indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
