package cmdhook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Musubi42/shhh/internal/detector"
)

// runCursor is the entry point for `shhh hook cursor`.
//
// Cursor IDE shipped a native hook system in v1.7 (Oct 2025) that
// expanded in v2.4 (2026). The stdin payload is shaped like Claude
// Code's — snake_case fields with `tool_name`, `tool_input`,
// `cwd`, etc. — with two notable differences:
//
//  1. Session identifiers use `conversation_id` + `generation_id`
//     instead of `session_id`. `effectiveSessionID()` on hookInput
//     resolves the right one (we use conversation_id for Cursor —
//     it maps to the long-lived thread the way session_id does on
//     Claude Code).
//
//  2. The response envelope is FLAT and snake_case
//     (`{permission, updated_input, user_message, agent_message}`),
//     not wrapped in `hookSpecificOutput.permissionDecision` /
//     `updatedInput` / `additionalContext` the way Claude Code and
//     Codex are. cursor.go owns its own envelope and writers; the
//     redaction primitives (LoadRedactor, RedactedPath,
//     narrateRedactions) are agent-agnostic and reused as-is.
//
// What we intercept in v1:
//
//   - preToolUse on tool_name `Shell` — wrap the command through
//     `shhh redact --session` exactly like the Claude Code Bash
//     handler. Cursor's preToolUse output supports
//     `updated_input`, so this is a direct port.
//
//   - preToolUse on tool_name `Read` — rewrite
//     `tool_input.file_path` to a per-session redacted cache copy,
//     same as Claude Code. The narration that warns Claude to use
//     Bash when Edit fails goes in `agent_message` (Cursor's
//     equivalent of `additionalContext`). Whether Cursor has the
//     same Read→Edit ledger limitation as Claude Code (see
//     docs/dev/known-limitations.md §1) is unverified at the protocol
//     level — first user run will tell us. Until then, shipping
//     the narration is safe: if Edit works on Cursor it just
//     ignores the note; if Edit fails the user has the right
//     workaround in hand.
//
//   - SessionEnd → wipes the per-session redactor cache. Cursor
//     uses `stop` for the equivalent event in some versions;
//     accept both for forward compatibility.
//
// What we do NOT intercept in v1:
//
//   - beforeReadFile — Cursor's docs are explicit that
//     beforeReadFile's response shape supports allow/deny only,
//     with no field to return modified content. We cannot redact
//     via this event; preToolUse:Read is the only path.
//
//   - apply_patch / Write — at writing, the tool surface for
//     Cursor's edit path is not fully verified. preToolUse with a
//     matcher of "Shell|Read|Write" is the documented shape, so
//     a Write handler can be added in a follow-up once we
//     confirm payload semantics.
//
//   - beforeShellExecution — duplicate signal with
//     preToolUse:Shell; preToolUse fires earlier and supports
//     updated_input. Stick with one path.
//
// Best-effort-non-blocking, same contract as runClaudeCode: any
// failure writes the empty response and exits 0.
func runCursor(stdin io.Reader, stdout io.Writer) error {
	raw, err := io.ReadAll(stdin)
	if err != nil {
		writeEmpty(stdout)
		return nil
	}
	var in hookInput
	if err := json.Unmarshal(raw, &in); err != nil {
		writeEmpty(stdout)
		return nil
	}

	switch in.HookEventName {
	case "preToolUse":
		handleCursorPreToolUse(stdout, &in)
	case "SessionEnd", "stop":
		_ = WipeSession(in.effectiveSessionID())
		writeEmpty(stdout)
	default:
		writeEmpty(stdout)
	}
	return nil
}

func handleCursorPreToolUse(stdout io.Writer, in *hookInput) {
	switch in.ToolName {
	case "Shell":
		handleCursorShell(stdout, in)
	case "Read":
		handleCursorRead(stdout, in)
	default:
		writeEmpty(stdout)
	}
}

// cursorResponse is the flat snake_case response envelope Cursor's
// hook protocol expects. All fields are omitempty so we can emit
// minimal JSON when only `permission` and one of the optional
// fields are needed. See docs/dev/cursor-research-2026-05-27.md for
// the verbatim spec.
type cursorResponse struct {
	Permission   string          `json:"permission,omitempty"`
	UpdatedInput json.RawMessage `json:"updated_input,omitempty"`
	UserMessage  string          `json:"user_message,omitempty"`
	AgentMessage string          `json:"agent_message,omitempty"`
}

// handleCursorShell wraps a Shell command through `shhh redact
// --session`. Same logic as handleBash, different response shape.
func handleCursorShell(stdout io.Writer, in *hookInput) {
	var ti bashToolInput
	if err := json.Unmarshal(in.ToolInput, &ti); err != nil || strings.TrimSpace(ti.Command) == "" {
		writeEmpty(stdout)
		return
	}
	// Avoid recursive wrapping if the command already contains a
	// shhh redact call — same guard as the Claude Code Bash handler.
	if strings.Contains(ti.Command, " shhh redact") || strings.Contains(ti.Command, "shhh redact ") {
		writeEmpty(stdout)
		return
	}
	sid := in.effectiveSessionID()
	if !sessionIDRe.MatchString(sid) {
		writeEmpty(stdout)
		return
	}
	shhh := shhhExecutable()
	wrapped := fmt.Sprintf(
		`set -o pipefail; { %s; } 2>&1 | %q redact --session %q`,
		ti.Command, shhh, sid,
	)
	updated := map[string]any{"command": wrapped}
	if len(ti.Description) > 0 {
		updated["description"] = json.RawMessage(ti.Description)
	}
	if len(ti.Timeout) > 0 {
		updated["timeout"] = json.RawMessage(ti.Timeout)
	}
	// Cursor's Shell tool_input may also carry `working_directory`,
	// per the docs example. Pass it through if present.
	var passthrough struct {
		WorkingDirectory json.RawMessage `json:"working_directory,omitempty"`
	}
	if err := json.Unmarshal(in.ToolInput, &passthrough); err == nil && len(passthrough.WorkingDirectory) > 0 {
		updated["working_directory"] = json.RawMessage(passthrough.WorkingDirectory)
	}
	updatedJSON, err := json.Marshal(updated)
	if err != nil {
		writeEmpty(stdout)
		return
	}
	writeJSON(stdout, cursorResponse{
		Permission:   "allow",
		UpdatedInput: updatedJSON,
	})
}

// handleCursorRead rewrites tool_input.file_path to a per-session
// redacted cache copy, and emits agent_message with the
// redaction narration. Same shape as handleRead but with a
// Cursor-flavored response envelope.
//
// Whether Cursor has the same Read→Edit ledger limitation as
// Claude Code is unverified at the protocol level. The narration
// includes the "if Edit fails, use Shell" guidance defensively;
// if Cursor does not have the bug, the note is harmless prose
// the agent ignores.
func handleCursorRead(stdout io.Writer, in *hookInput) {
	var ti readToolInput
	if err := json.Unmarshal(in.ToolInput, &ti); err != nil || ti.FilePath == "" {
		writeEmpty(stdout)
		return
	}
	abs, err := filepath.Abs(ti.FilePath)
	if err != nil {
		writeEmpty(stdout)
		return
	}
	f, err := os.Open(abs)
	if err != nil {
		writeEmpty(stdout)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() > maxRedactFileSize {
		writeEmpty(stdout)
		return
	}
	content, err := io.ReadAll(f)
	if err != nil {
		writeEmpty(stdout)
		return
	}
	if bytes.IndexByte(content, 0) >= 0 {
		writeEmpty(stdout)
		return
	}
	sid := in.effectiveSessionID()
	if !sessionIDRe.MatchString(sid) {
		writeEmpty(stdout)
		return
	}
	red, save, err := LoadRedactor(sid)
	if err != nil {
		writeEmpty(stdout)
		return
	}
	var (
		out      []byte
		findings []detector.Finding
	)
	envMode := isEnvLikePath(abs)
	if envMode {
		out, findings = red.RedactEnvFile(content)
	} else {
		out, findings = red.Redact(content)
	}
	if len(findings) == 0 {
		writeEmpty(stdout)
		return
	}
	destPath, err := RedactedPath(sid, abs)
	if err != nil {
		writeEmpty(stdout)
		return
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o700); err != nil {
		writeEmpty(stdout)
		return
	}
	if err := os.WriteFile(destPath, out, 0o600); err != nil {
		writeEmpty(stdout)
		return
	}
	if err := save(); err != nil {
		// Placeholder map didn't persist; current firing still
		// goes through, future firings lose continuity. Non-fatal.
	}
	updated := map[string]any{"file_path": destPath}
	if len(ti.Offset) > 0 {
		updated["offset"] = json.RawMessage(ti.Offset)
	}
	if len(ti.Limit) > 0 {
		updated["limit"] = json.RawMessage(ti.Limit)
	}
	updatedJSON, err := json.Marshal(updated)
	if err != nil {
		writeEmpty(stdout)
		return
	}
	writeJSON(stdout, cursorResponse{
		Permission:   "allow",
		UpdatedInput: updatedJSON,
		AgentMessage: narrateRedactions(ti.FilePath, findings, content, red, envMode),
	})
}
