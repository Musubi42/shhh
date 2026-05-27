package cmdinstall

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// shhAllowCommandBody is the markdown body of the /shhh-allow slash
// command that ships with `shhh install claude-code`.
//
// Claude Code interprets a slash-command markdown file as instructions
// for Claude itself: when the user types `/shhh-allow STRIPE_LIVE_KEY`,
// Claude reads this file, substitutes $ARGUMENTS with the user's
// argument, and runs the embedded Bash via its Bash tool.
//
// SHHH_SESSION_ID is populated for the current session by shhh's
// SessionStart hook (see cmd/shhh/cmdhook/session_start.go).
const shhhAllowCommandBody = `---
description: Allow a secret placeholder name for this Claude Code session (max 24h)
---

The user wants shhh to stop redacting a specific placeholder name for the
remainder of this Claude Code session (or 24h, whichever comes first).

Run exactly this command, with $ARGUMENTS as the placeholder name the user
provided (e.g. ` + "`STRIPE_LIVE_KEY`" + `, ` + "`ANTHROPIC_API_KEY`" + `):

` + "```" + `bash
shhh allow --session-id "$SHHH_SESSION_ID" --add "$ARGUMENTS"
` + "```" + `

Then tell the user, in one short sentence, that you've allowed $ARGUMENTS for
this session and they can press ↑ to recall their previous prompt and
re-submit. If ` + "`shhh allow`" + ` errored, paste the error verbatim and stop
— don't try to "fix" it yourself.
`

// shhhAllowCommandPath returns the absolute path of the /shhh-allow
// slash command file derived from a settings.json path. For a
// settings file at ~/.claude/settings.json the command file lives at
// ~/.claude/commands/shhh-allow.md; for <project>/.claude/settings.json
// it lives at <project>/.claude/commands/shhh-allow.md.
func shhhAllowCommandPath(settingsPath string) string {
	dir := filepath.Dir(settingsPath)
	return filepath.Join(dir, "commands", "shhh-allow.md")
}

// installShhhAllowCommand writes the slash command file alongside the
// settings.json shhh just touched. Idempotent: if the file is already
// present with the current body, this is a no-op. Returns true when
// the file was created or updated, false when it was already current.
func installShhhAllowCommand(settingsPath string) (bool, error) {
	path := shhhAllowCommandPath(settingsPath)
	existing, err := os.ReadFile(path)
	if err == nil && string(existing) == shhhAllowCommandBody {
		return false, nil
	}
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, err
	}
	if err := os.WriteFile(path, []byte(shhhAllowCommandBody), 0o600); err != nil {
		return false, err
	}
	return true, nil
}

// uninstallShhhAllowCommand removes the slash command file written by
// installShhhAllowCommand. Returns true when a file was removed, false
// when nothing was there. Errors other than not-exist are surfaced.
func uninstallShhhAllowCommand(settingsPath string) (bool, error) {
	path := shhhAllowCommandPath(settingsPath)
	if err := os.Remove(path); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
