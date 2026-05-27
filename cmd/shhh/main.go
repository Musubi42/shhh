// Command shhh is the CLI. v0.1 (milestone 1) ships scan, redact, hook,
// install, and uninstall. See docs/dev/implementation-roadmap.md.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/Musubi42/shhh/cmd/shhh/cmdallow"
	"github.com/Musubi42/shhh/cmd/shhh/cmdaudit"
	"github.com/Musubi42/shhh/cmd/shhh/cmdbench"
	"github.com/Musubi42/shhh/cmd/shhh/cmddoctor"
	"github.com/Musubi42/shhh/cmd/shhh/cmdhook"
	"github.com/Musubi42/shhh/cmd/shhh/cmdignore"
	"github.com/Musubi42/shhh/cmd/shhh/cmdinstall"
	"github.com/Musubi42/shhh/cmd/shhh/cmdlicenses"
	"github.com/Musubi42/shhh/cmd/shhh/cmdredact"
	"github.com/Musubi42/shhh/cmd/shhh/cmdscan"
)

// init wires the installer's scan hook to the real cmdscan. Kept out
// of the cmdinstall package to avoid an import cycle between
// cmdinstall and cmdscan.
func init() {
	cmdinstall.RunScanHook = func(out io.Writer, target string) error {
		// Route cmdscan's stdout into the installer's writer by
		// swapping os.Stdout. cmdscan writes to os.Stdout directly;
		// this is the minimum-invasive bridge.
		old := os.Stdout
		r, w, err := os.Pipe()
		if err != nil {
			return err
		}
		os.Stdout = w
		done := make(chan struct{})
		go func() {
			_, _ = io.Copy(out, r)
			close(done)
		}()
		runErr := cmdscan.Run([]string{target})
		w.Close()
		os.Stdout = old
		<-done
		return runErr
	}
}

const version = "0.3.0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "scan":
		err = cmdscan.Run(os.Args[2:])
	case "audit":
		err = cmdaudit.Run(os.Args[2:])
	case "redact":
		err = cmdredact.Run(os.Args[2:])
	case "hook":
		err = cmdhook.Run(os.Args[2:])
	case "install":
		err = cmdinstall.RunInstall(os.Args[2:])
	case "uninstall":
		err = cmdinstall.RunUninstall(os.Args[2:])
	case "doctor":
		err = cmddoctor.Run(os.Args[2:])
	case "bench":
		err = cmdbench.Run(os.Args[2:])
	case "ignore":
		err = cmdignore.Run(os.Args[2:])
	case "allow":
		err = cmdallow.Run(os.Args[2:])
	case "licenses":
		err = cmdlicenses.Run(os.Args[2:])
	case "version", "--version", "-v":
		fmt.Println("shhh", version)
		return
	case "help", "--help", "-h":
		usage()
		return
	default:
		fmt.Fprintf(os.Stderr, "shhh: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "shhh:", err)
		os.Exit(1)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `shhh — stop leaking secrets to AI agents

Usage:
  shhh install                        Interactive installer (recommended)
  shhh install claude-code            Non-interactive install — global by default
    --scope global|project             Default: global (~/.claude). Project = <cwd>/.claude
    --cwd <path>                       For --scope project, the project root (default: $PWD)
  shhh uninstall claude-code          Remove the shhh hook (mirrors install flags)

  shhh hook claude-code               Hook entry point (invoked by Claude Code, not you)

  shhh audit                          Forensic audit of what Claude Code has seen
    (interactive project picker by default — TTY only)
    --no-select                        Skip the picker, audit every non-ignored project
    --no-serve                         Terminal only, no HTML, no server
    --html-only                        Write HTML to disk, no server
    --open                             Default + launch browser
    --json                             Machine-readable JSON to stdout
  shhh audit ignore <abs-path>        Persist a project skip
  shhh audit unignore <abs-path>      Reverse the above
  shhh audit ignored                  Print the current ignore list

  shhh scan [path]                    Scan a directory for secrets (default: .)
    --show-details                     Show host/user details (local only)
    --format text|json|md              Output format (default: text)

  shhh redact <file>                  Redact a file, print to stdout
    --rehydrate                        Reverse operation
    --session <id>                     Use per-session placeholder store

  shhh doctor                         Health check (config, hooks, binary)
    --fix                              Drop stale installed_paths entries

  shhh bench <path>... [flags]        Compare detection engines on real files
    --engines=shhh-native,gitleaks     Subset to compare (default: both)
    --no-serve                         Write HTML to disk, no server
    --no-html                          Terminal only, no HTML
    --open                             Default + launch browser

  shhh ignore list                    List active .shhhignore rule cascade
  shhh ignore add <pattern>           Append a rule (default: project)
    --global                            Append to ~/.shhh/.shhhignore instead
  shhh ignore check <path>            Show which layer decides a path

  shhh allow                          Allow a placeholder name for the current Claude Code session
    --session-id <id>                  Session to scope under (defaults to $SHHH_SESSION_ID)
    --add NAME                         Placeholder name to allow (e.g. STRIPE_LIVE_KEY)
    --list                             List currently-allowed names

  shhh licenses                       Print shhh + third-party MIT notices

  shhh version                        Print version
  shhh help                           Show this message
`)
}
