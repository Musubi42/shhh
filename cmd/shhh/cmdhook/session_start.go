package cmdhook

import (
	"fmt"
	"io"
	"os"
)

// handleSessionStart exports the current Claude Code session id as
// the SHHH_SESSION_ID environment variable for the rest of the
// session, so that Bash commands launched by Claude — including the
// `shhh allow` invocation behind the /shhh-allow slash command —
// know which session to scope under.
//
// Claude Code's SessionStart hook receives a path in CLAUDE_ENV_FILE
// where the hook can append KEY=VALUE lines that get loaded into the
// session's environment. See:
// https://code.claude.com/docs/en/hooks
//
// Best-effort: any error writing the env file is swallowed and the
// hook returns a no-op response. shhh's other hooks still work
// without SHHH_SESSION_ID (the user just cannot use /shhh-allow
// without explicitly passing --session-id).
func handleSessionStart(stdout io.Writer, in *hookInput) {
	defer writeEmpty(stdout)

	sid := in.effectiveSessionID()
	if sid == "" {
		return
	}
	envFile := os.Getenv("CLAUDE_ENV_FILE")
	if envFile == "" {
		return
	}

	f, err := os.OpenFile(envFile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintf(f, "SHHH_SESSION_ID=%s\n", sid)
}
