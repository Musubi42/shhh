package cmdhook

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const stripeKey = "sk_live_4eC39HqLyjWDarjtT1zdp7dc"

func runClaude(t *testing.T, payload any) map[string]any {
	t.Helper()
	in, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runClaudeCode(bytes.NewReader(in), &out); err != nil {
		t.Fatalf("runClaudeCode: %v", err)
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

func writeFixtureEnv(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(dir, ".env")
	content := "STRIPE_LIVE_KEY=" + stripeKey + "\nOTHER=plain\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestReadRedactsSecretToTempFile(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	envPath := writeFixtureEnv(t, t.TempDir())

	resp := runClaude(t, map[string]any{
		"session_id":      "sess-read-01",
		"hook_event_name": "PreToolUse",
		"tool_name":       "Read",
		"tool_input":      map[string]any{"file_path": envPath},
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
	newPath, _ := ui["file_path"].(string)
	if newPath == "" || newPath == envPath {
		t.Fatalf("updatedInput.file_path not rewritten: %q", newPath)
	}

	redacted, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("read redacted: %v", err)
	}
	if strings.Contains(string(redacted), stripeKey) {
		t.Errorf("raw key leaked into redacted file:\n%s", redacted)
	}
	if !strings.Contains(string(redacted), "[STRIPE_LIVE_KEY:") {
		t.Errorf("expected STRIPE_LIVE_KEY placeholder, got:\n%s", redacted)
	}
}

func TestReadRedactedPathEndsInOriginalBasename(t *testing.T) {
	// Transcript readability: the leaf of the rewritten file_path should
	// be the original basename, so the user sees ".env" at the tail
	// instead of a hex hash.
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	envPath := writeFixtureEnv(t, t.TempDir())
	resp := runClaude(t, map[string]any{
		"session_id":      "sess-read-path-01",
		"hook_event_name": "PreToolUse",
		"tool_name":       "Read",
		"tool_input":      map[string]any{"file_path": envPath},
	})
	newPath := resp["hookSpecificOutput"].(map[string]any)["updatedInput"].(map[string]any)["file_path"].(string)
	if filepath.Base(newPath) != ".env" {
		t.Errorf("redacted path leaf = %q, want %q", filepath.Base(newPath), ".env")
	}
}

func TestReadAdditionalContextNarration(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	envPath := writeFixtureEnv(t, t.TempDir())
	resp := runClaude(t, map[string]any{
		"session_id":      "sess-read-narr-01",
		"hook_event_name": "PreToolUse",
		"tool_name":       "Read",
		"tool_input":      map[string]any{"file_path": envPath},
	})
	hso := resp["hookSpecificOutput"].(map[string]any)
	ctx, _ := hso["additionalContext"].(string)
	if ctx == "" {
		t.Fatal("expected non-empty additionalContext on redacted Read")
	}
	// Must reference the ORIGINAL path (not the cache path) so Claude
	// can tell the user which file it actually read.
	if !strings.Contains(ctx, envPath) {
		t.Errorf("additionalContext should reference original path %q:\n%s", envPath, ctx)
	}
	// Must name shhh and the count, and list the redacted label.
	if !strings.Contains(ctx, "shhh") {
		t.Errorf("narration should name shhh:\n%s", ctx)
	}
	if !strings.Contains(ctx, "STRIPE_LIVE_KEY") {
		t.Errorf("narration should list redacted label STRIPE_LIVE_KEY:\n%s", ctx)
	}
	if !strings.Contains(ctx, "1 secret") {
		t.Errorf("narration should state the count:\n%s", ctx)
	}
	// .env files are line-preserving under redaction — Claude can already
	// read the variable→placeholder mapping from the file content itself,
	// so the narration MUST NOT also list "at line N" for env files. This
	// is a deliberate dedupe to keep the note tight.
	if strings.Contains(ctx, "at line") {
		t.Errorf("narration on env files should skip per-line listing:\n%s", ctx)
	}
}

func TestReadNarrationListsLineNumbersForProseFile(t *testing.T) {
	// On a non-env file (prose/source/config) the narration must carry
	// per-finding line numbers and the exact placeholder inline, so
	// Claude can tell the user exactly which secret sat where.
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	dir := t.TempDir()
	proseFile := filepath.Join(dir, "notes.md")
	content := "# Deploy notes\n\nThe current production key is " + stripeKey + " and it must be rotated monthly.\n"
	if err := os.WriteFile(proseFile, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	resp := runClaude(t, map[string]any{
		"session_id":      "sess-read-narr-prose-01",
		"hook_event_name": "PreToolUse",
		"tool_name":       "Read",
		"tool_input":      map[string]any{"file_path": proseFile},
	})
	hso, ok := resp["hookSpecificOutput"].(map[string]any)
	if !ok {
		t.Fatalf("expected hookSpecificOutput, got %v", resp)
	}
	ctx, _ := hso["additionalContext"].(string)
	if ctx == "" {
		t.Fatal("expected non-empty additionalContext on redacted Read")
	}
	if !strings.Contains(ctx, "at line 3") {
		t.Errorf("narration should pin STRIPE_LIVE_KEY to line 3:\n%s", ctx)
	}
	if !strings.Contains(ctx, "STRIPE_LIVE_KEY at line") {
		t.Errorf("narration should list label + line:\n%s", ctx)
	}
	// The exact placeholder must be inlined so Claude can correlate what
	// it sees in the file body to the per-finding row in the narration.
	if !strings.Contains(ctx, "placeholder: [STRIPE_LIVE_KEY:") {
		t.Errorf("narration should inline the placeholder:\n%s", ctx)
	}
}

func TestReadPreservesOffsetAndLimit(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	envPath := writeFixtureEnv(t, t.TempDir())

	resp := runClaude(t, map[string]any{
		"session_id":      "sess-read-02",
		"hook_event_name": "PreToolUse",
		"tool_name":       "Read",
		"tool_input": map[string]any{
			"file_path": envPath,
			"offset":    10,
			"limit":     100,
		},
	})
	ui := resp["hookSpecificOutput"].(map[string]any)["updatedInput"].(map[string]any)
	if ui["offset"] != float64(10) {
		t.Errorf("offset not preserved: %v", ui["offset"])
	}
	if ui["limit"] != float64(100) {
		t.Errorf("limit not preserved: %v", ui["limit"])
	}
}

func TestReadNoFindingsPassesThrough(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	dir := t.TempDir()
	clean := filepath.Join(dir, "clean.txt")
	if err := os.WriteFile(clean, []byte("hello world\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	resp := runClaude(t, map[string]any{
		"session_id":      "sess-read-03",
		"hook_event_name": "PreToolUse",
		"tool_name":       "Read",
		"tool_input":      map[string]any{"file_path": clean},
	})
	if _, ok := resp["hookSpecificOutput"]; ok {
		t.Errorf("expected empty response for no-findings file, got %v", resp)
	}
}

func TestReadMissingFilePassesThrough(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	resp := runClaude(t, map[string]any{
		"session_id":      "sess-read-04",
		"hook_event_name": "PreToolUse",
		"tool_name":       "Read",
		"tool_input":      map[string]any{"file_path": "/nonexistent/path/xyz"},
	})
	if len(resp) != 0 {
		t.Errorf("expected {}, got %v", resp)
	}
}

func TestReadBinaryFileSkipped(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	dir := t.TempDir()
	bin := filepath.Join(dir, "bin.dat")
	if err := os.WriteFile(bin, []byte{0x00, 0x01, 0x02, 'a', 'b'}, 0o600); err != nil {
		t.Fatal(err)
	}
	resp := runClaude(t, map[string]any{
		"session_id":      "sess-read-05",
		"hook_event_name": "PreToolUse",
		"tool_name":       "Read",
		"tool_input":      map[string]any{"file_path": bin},
	})
	if len(resp) != 0 {
		t.Errorf("expected {}, got %v", resp)
	}
}

func TestReadRewritesToStablePath(t *testing.T) {
	// Two reads of the same source file in the same session should
	// resolve to the same redacted path, so the cache amortizes.
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	envPath := writeFixtureEnv(t, t.TempDir())

	payload := map[string]any{
		"session_id":      "sess-read-06",
		"hook_event_name": "PreToolUse",
		"tool_name":       "Read",
		"tool_input":      map[string]any{"file_path": envPath},
	}
	r1 := runClaude(t, payload)
	r2 := runClaude(t, payload)
	p1 := r1["hookSpecificOutput"].(map[string]any)["updatedInput"].(map[string]any)["file_path"]
	p2 := r2["hookSpecificOutput"].(map[string]any)["updatedInput"].(map[string]any)["file_path"]
	if p1 != p2 {
		t.Errorf("redacted path not stable: %v vs %v", p1, p2)
	}
}

func TestBashCommandWrapped(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	resp := runClaude(t, map[string]any{
		"session_id":      "sess-bash-01",
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"tool_input":      map[string]any{"command": "cat .env"},
	})
	ui := resp["hookSpecificOutput"].(map[string]any)["updatedInput"].(map[string]any)
	cmd, _ := ui["command"].(string)
	if !strings.Contains(cmd, "cat .env") {
		t.Errorf("original command not preserved: %q", cmd)
	}
	if !strings.Contains(cmd, "redact --session") {
		t.Errorf("command not wrapped through shhh redact: %q", cmd)
	}
	if !strings.Contains(cmd, "sess-bash-01") {
		t.Errorf("session id not threaded: %q", cmd)
	}
}

func TestBashAlreadyWrappedPassesThrough(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	resp := runClaude(t, map[string]any{
		"session_id":      "sess-bash-02",
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"tool_input":      map[string]any{"command": "cat .env | shhh redact --session foo"},
	})
	if len(resp) != 0 {
		t.Errorf("expected {}, got %v", resp)
	}
}

func TestBashEmptyCommandPassesThrough(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	resp := runClaude(t, map[string]any{
		"session_id":      "sess-bash-03",
		"hook_event_name": "PreToolUse",
		"tool_name":       "Bash",
		"tool_input":      map[string]any{"command": "   "},
	})
	if len(resp) != 0 {
		t.Errorf("expected {}, got %v", resp)
	}
}

func TestUnknownEventPassesThrough(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	resp := runClaude(t, map[string]any{
		"session_id":      "sess-unk-01",
		"hook_event_name": "Notification",
	})
	if len(resp) != 0 {
		t.Errorf("expected {}, got %v", resp)
	}
}

func TestUnknownToolPassesThrough(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	resp := runClaude(t, map[string]any{
		"session_id":      "sess-unk-02",
		"hook_event_name": "PreToolUse",
		"tool_name":       "Glob",
		"tool_input":      map[string]any{"pattern": "*.go"},
	})
	if len(resp) != 0 {
		t.Errorf("expected {}, got %v", resp)
	}
}

func TestMalformedInputPassesThrough(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	var out bytes.Buffer
	if err := runClaudeCode(strings.NewReader("not json"), &out); err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.String()) != "{}" {
		t.Errorf("expected {}, got %q", out.String())
	}
}

func TestReadEnvFileCatchesShortToken(t *testing.T) {
	// Regression: a 22-char all-unique token sits below the generic
	// 4.5 entropy threshold, but a .env file should still get it via
	// the env-aware pass.
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	content := "STRIPE=" + stripeKey + "\nINTERNAL_LEGACY_TOKEN=Mk9zPwXr7AqN4bVtC2yLhG\nAPP_ENV=production\n"
	if err := os.WriteFile(envPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	resp := runClaude(t, map[string]any{
		"session_id":      "sess-env-short-01",
		"hook_event_name": "PreToolUse",
		"tool_name":       "Read",
		"tool_input":      map[string]any{"file_path": envPath},
	})
	newPath := resp["hookSpecificOutput"].(map[string]any)["updatedInput"].(map[string]any)["file_path"].(string)
	redacted, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatal(err)
	}
	s := string(redacted)
	if strings.Contains(s, "Mk9zPwXr7AqN4bVtC2yLhG") {
		t.Errorf("short token leaked from .env:\n%s", s)
	}
	if !strings.Contains(s, "[INTERNAL_LEGACY_TOKEN:") {
		t.Errorf("expected INTERNAL_LEGACY_TOKEN placeholder:\n%s", s)
	}
	if !strings.Contains(s, "[STRIPE_LIVE_KEY:") {
		t.Errorf("expected STRIPE_LIVE_KEY placeholder:\n%s", s)
	}
	if !strings.Contains(s, "APP_ENV=production") {
		t.Errorf("non-secret APP_ENV should survive:\n%s", s)
	}
}

func TestReadNonEnvFileKeepsStrictGate(t *testing.T) {
	// On a regular source file, the short token is NOT a secret — the
	// env-aware pass must not fire.
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	dir := t.TempDir()
	srcPath := filepath.Join(dir, "config.go")
	content := `package config

const LegacyToken = "Mk9zPwXr7AqN4bVtC2yLhG"
`
	if err := os.WriteFile(srcPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	resp := runClaude(t, map[string]any{
		"session_id":      "sess-nonenv-01",
		"hook_event_name": "PreToolUse",
		"tool_name":       "Read",
		"tool_input":      map[string]any{"file_path": srcPath},
	})
	// Nothing to redact → empty response, Claude reads the file unmodified.
	if len(resp) != 0 {
		t.Errorf("non-env file should pass through: got %v", resp)
	}
}

func TestSessionEndWipesCache(t *testing.T) {
	cacheDir := t.TempDir()
	t.Setenv("SHHH_CACHE_DIR", cacheDir)
	const sid = "sess-end-01"

	// Prime the cache.
	envPath := writeFixtureEnv(t, t.TempDir())
	runClaude(t, map[string]any{
		"session_id":      sid,
		"hook_event_name": "PreToolUse",
		"tool_name":       "Read",
		"tool_input":      map[string]any{"file_path": envPath},
	})
	dir, _ := sessionDir(sid)
	if _, err := os.Stat(dir); err != nil {
		t.Fatalf("session dir should exist before SessionEnd: %v", err)
	}

	// Fire SessionEnd.
	runClaude(t, map[string]any{
		"session_id":      sid,
		"hook_event_name": "SessionEnd",
	})
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Errorf("session dir should be gone after SessionEnd, got err=%v", err)
	}
}
