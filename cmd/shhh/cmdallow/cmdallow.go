// Package cmdallow implements `shhh allow`, the CLI surface that
// backs the /shhh-allow slash command in Claude Code.
//
// It is intentionally minimal: one flag set, one effect. The slash
// command file dropped by `shhh install` invokes:
//
//	shhh allow --session-id "$SHHH_SESSION_ID" --add NAME
//
// where SHHH_SESSION_ID is exported by the SessionStart hook (via
// CLAUDE_ENV_FILE) so that downstream Bash invocations launched by
// Claude can reach it.
package cmdallow

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/Musubi42/shhh/cmd/shhh/cmdhook"
)

// Run is the entry point for `shhh allow`.
func Run(args []string) error {
	return run(args, os.Stdout, os.Stderr)
}

func run(args []string, stdout, stderr io.Writer) error {
	fs := flag.NewFlagSet("allow", flag.ContinueOnError)
	fs.SetOutput(stderr)
	sessionID := fs.String("session-id", os.Getenv("SHHH_SESSION_ID"), "session id to scope the allow under (defaults to $SHHH_SESSION_ID)")
	add := fs.String("add", "", "placeholder name to allow for this session (e.g. STRIPE_LIVE_KEY)")
	list := fs.Bool("list", false, "list currently-allowed names for this session")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if *sessionID == "" {
		return fmt.Errorf("--session-id is required (or set SHHH_SESSION_ID — usually wired by shhh's SessionStart hook)")
	}

	switch {
	case *add != "":
		if err := cmdhook.AddAllow(*sessionID, *add); err != nil {
			return fmt.Errorf("allow add: %w", err)
		}
		fmt.Fprintf(stdout, "Allowed %s for this session (max 24h or until session ends).\n", *add)
		fmt.Fprintln(stdout, "Press ↑ to recall your previous prompt and re-submit.")
		return nil
	case *list:
		entries, err := cmdhook.LoadAllow(*sessionID)
		if err != nil {
			return fmt.Errorf("allow list: %w", err)
		}
		if len(entries) == 0 {
			fmt.Fprintln(stdout, "(no names allowed for this session)")
			return nil
		}
		for name := range entries {
			fmt.Fprintln(stdout, name)
		}
		return nil
	default:
		return fmt.Errorf("nothing to do: pass --add NAME or --list")
	}
}
