package cmdhook

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
)

// Run is the entry point for `shhh hook <target>`. Supported targets:
// "claude-code" and "codex".
//
// Redaction is best-effort-non-blocking: any failure path falls through to
// an empty JSON response and exit 0, so the tool call proceeds unmodified
// rather than the hook getting in the user's way. Genuine programmer errors
// (unknown target, bad arg count) are the only non-zero exits.
func Run(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("hook: missing target (try: shhh hook claude-code)")
	}
	target := args[0]
	switch target {
	case "claude-code":
		return runClaudeCode(os.Stdin, os.Stdout)
	case "codex":
		return runCodex(os.Stdin, os.Stdout)
	case "cursor":
		return runCursor(os.Stdin, os.Stdout)
	default:
		return fmt.Errorf("hook: unknown target %q (supported: claude-code, codex, cursor)", target)
	}
}

// hookInput is the subset of an agent's hook stdin payload we care
// about. Claude Code and Codex use `session_id`; Cursor splits the
// notion into `conversation_id` (long-lived thread) and
// `generation_id` (per-LLM-call). We accept both shapes and let
// `effectiveSessionID` resolve which one to use. Unknown fields pass
// through JSON unmarshal unchanged.
type hookInput struct {
	SessionID      string          `json:"session_id"`
	ConversationID string          `json:"conversation_id"`
	HookEventName  string          `json:"hook_event_name"`
	ToolName       string          `json:"tool_name"`
	ToolInput      json.RawMessage `json:"tool_input"`
	// Prompt is populated on UserPromptSubmit events. Claude Code sends
	// the user's literal prompt text here, before it reaches the model.
	Prompt string `json:"prompt"`
}

// effectiveSessionID returns the agent-appropriate identifier used to
// scope the session cache and the placeholder map. Prefers
// `session_id` when present (Claude Code, Codex) and falls back to
// `conversation_id` (Cursor). Empty when neither is set — callers
// should writeEmpty in that case.
func (in *hookInput) effectiveSessionID() string {
	if in.SessionID != "" {
		return in.SessionID
	}
	return in.ConversationID
}

func runClaudeCode(stdin io.Reader, stdout io.Writer) error {
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
	case "PreToolUse":
		handlePreToolUse(stdout, &in)
	case "UserPromptSubmit":
		handleUserPromptSubmit(stdout, &in)
	case "SessionStart":
		handleSessionStart(stdout, &in)
	case "SessionEnd":
		_ = WipeSession(in.effectiveSessionID())
		writeEmpty(stdout)
	default:
		writeEmpty(stdout)
	}
	return nil
}

func handlePreToolUse(stdout io.Writer, in *hookInput) {
	switch in.ToolName {
	case "Read":
		handleRead(stdout, in)
	case "Bash":
		handleBash(stdout, in)
	default:
		writeEmpty(stdout)
	}
}

// writeEmpty emits a no-op response. Claude Code treats `{}` as "no
// modification, proceed."
func writeEmpty(w io.Writer) {
	_, _ = w.Write([]byte("{}\n"))
}

// writeJSON marshals v and writes it; on marshal error falls back to empty.
func writeJSON(w io.Writer, v any) {
	out, err := json.Marshal(v)
	if err != nil {
		writeEmpty(w)
		return
	}
	_, _ = w.Write(out)
	_, _ = w.Write([]byte("\n"))
}

// hookSpecificOutput is the PreToolUse response envelope. We always emit
// permissionDecision=allow — this hook never blocks, it only rewrites the
// input on its way through. additionalContext is a free-text note that
// Claude Code appends to the model's context for this tool call; shhh
// uses it to tell Claude what was redacted and to ask Claude to mention
// the protection when answering the user.
type hookSpecificOutput struct {
	HookEventName            string          `json:"hookEventName"`
	PermissionDecision       string          `json:"permissionDecision,omitempty"`
	PermissionDecisionReason string          `json:"permissionDecisionReason,omitempty"`
	UpdatedInput             json.RawMessage `json:"updatedInput,omitempty"`
	AdditionalContext        string          `json:"additionalContext,omitempty"`
}

type hookResponse struct {
	HookSpecificOutput *hookSpecificOutput `json:"hookSpecificOutput,omitempty"`
}
