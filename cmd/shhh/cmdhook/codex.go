package cmdhook

import (
	"encoding/json"
	"io"
)

// runCodex is the entry point for `shhh hook codex`.
//
// The Codex CLI hook payload is the same shape as Claude Code's (per
// docs/codex-research-2026-05-26.md): `session_id`, `tool_name`,
// `tool_input`, `hook_event_name`, plus Codex-only fields like
// `turn_id` and `permission_mode` that we don't need and ignore.
// The response envelope is also identical
// (`hookSpecificOutput.{hookEventName, permissionDecision, updatedInput,
// additionalContext}`).
//
// What differs from Claude Code:
//
//  1. Codex has no first-class `Read` tool. File reads happen as
//     shell commands (`cat .env`, `rg STRIPE_LIVE_KEY`, …) so they
//     fire as `Bash`. We dispatch only Bash here; everything else
//     (Read, apply_patch, MCP tools, internal grep) falls through
//     to writeEmpty.
//
//  2. As of Codex v0.117 (2026-05), `PreToolUse` fires reliably only
//     for `Bash`. `apply_patch`, `read_file`, and `grep` are tracked
//     in https://github.com/openai/codex/issues/18491 but not yet
//     shipped. Until upstream fixes that, in-place edits via
//     apply_patch can hand the model a raw secret before shhh sees
//     it. README and docs/known-limitations.md document this gap.
//     When upstream ships the fix, the v1 hook auto-improves —
//     handleBash already redacts on Bash, but new tool dispatches
//     here will need new handlers.
//
// Best-effort-non-blocking, same contract as runClaudeCode: any
// failure → writeEmpty + exit 0 rather than dropping the tool call.
func runCodex(stdin io.Reader, stdout io.Writer) error {
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
		handleCodexPreToolUse(stdout, &in)
	case "SessionEnd":
		_ = WipeSession(in.effectiveSessionID())
		writeEmpty(stdout)
	default:
		writeEmpty(stdout)
	}
	return nil
}

// handleCodexPreToolUse routes Codex PreToolUse events. Only Bash is
// intercepted in v1 — see runCodex's comment for why other tools fall
// through unmodified.
func handleCodexPreToolUse(stdout io.Writer, in *hookInput) {
	switch in.ToolName {
	case "Bash":
		handleBash(stdout, in)
	default:
		writeEmpty(stdout)
	}
}
