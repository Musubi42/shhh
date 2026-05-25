// Command shhh is the CLI. v0.1 (milestone 1) ships scan, redact, hook,
// install, and uninstall. See docs/implementation-roadmap.md.
package main

import (
	"fmt"
	"io"
	"os"

	"github.com/Musubi42/shhh/cmd/shhh/cmdaudit"
	"github.com/Musubi42/shhh/cmd/shhh/cmdhook"
	"github.com/Musubi42/shhh/cmd/shhh/cmdinstall"
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

const version = "0.1.0-dev"

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
  shhh install claude-code            Non-interactive install into Claude Code
  shhh uninstall claude-code          Remove the shhh hook from Claude Code

  shhh hook claude-code               Hook entry point (invoked by Claude Code, not you)

  shhh audit                          Forensic audit of what Claude Code has seen
    --no-serve                         Terminal only, no HTML, no server
    --html-only                        Write HTML to disk, no server
    --open                             Default + launch browser
    --json                             Machine-readable JSON to stdout

  shhh scan [path]                    Scan a directory for secrets (default: .)
    --show-details                     Show host/user details (local only)
    --format text|json|md              Output format (default: text)

  shhh redact <file>                  Redact a file, print to stdout
    --rehydrate                        Reverse operation
    --session <id>                     Use per-session placeholder store

  shhh version                        Print version
  shhh help                           Show this message
`)
}
