package cmdhook

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

// bashToolInput is the subset of Claude Code's Bash tool_input we touch.
type bashToolInput struct {
	Command     string          `json:"command"`
	Description json.RawMessage `json:"description,omitempty"`
	Timeout     json.RawMessage `json:"timeout,omitempty"`
}

// shhhExecutable resolves the path to the currently-running binary so we
// can reference it from the rewritten shell command. Falls back to the
// plain name "shhh" if the lookup fails (which is graceful: if shhh isn't
// on PATH the wrapped command fails cleanly at runtime rather than this
// hook silently dropping the rewrite).
func shhhExecutable() string {
	if p, err := os.Executable(); err == nil {
		return p
	}
	return "shhh"
}

func handleBash(stdout io.Writer, in *hookInput) {
	var ti bashToolInput
	if err := json.Unmarshal(in.ToolInput, &ti); err != nil || strings.TrimSpace(ti.Command) == "" {
		writeEmpty(stdout)
		return
	}

	// Avoid recursive wrapping if the command is already a shhh hook call
	// (e.g. the user manually wrapped it, or a previous firing's rewrite
	// somehow round-trips). Cheap guard: if the command already contains
	// the shhh binary path or the `shhh redact` subcommand string, leave
	// it alone.
	if strings.Contains(ti.Command, " shhh redact") || strings.Contains(ti.Command, "shhh redact ") {
		writeEmpty(stdout)
		return
	}

	shhh := shhhExecutable()
	sid := in.SessionID
	if !sessionIDRe.MatchString(sid) {
		writeEmpty(stdout)
		return
	}

	// Wrap: run the original command, pipe combined output through
	// `shhh redact --session <id>`. The subshell preserves multi-statement
	// commands and exit-status propagation via `set -o pipefail`.
	//
	// The 2>&1 captures stderr too — a lot of real-world secret leaks
	// come from curl's -v output or aws-cli errors that echo keys.
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
	updatedJSON, err := json.Marshal(updated)
	if err != nil {
		writeEmpty(stdout)
		return
	}

	writeJSON(stdout, hookResponse{
		HookSpecificOutput: &hookSpecificOutput{
			HookEventName:      "PreToolUse",
			PermissionDecision: "allow",
			UpdatedInput:       updatedJSON,
		},
	})
}
