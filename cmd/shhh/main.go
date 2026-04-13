// Command shhh is the Phase 0 CLI. It exposes `scan` and `redact` only.
// Hooks, proxy, MCP server, and TUI installer are intentionally absent —
// those ship in Phase 3+.
package main

import (
	"fmt"
	"os"

	"github.com/musubi-sasu/shhh/cmd/shhh/cmdredact"
	"github.com/musubi-sasu/shhh/cmd/shhh/cmdscan"
)

const version = "0.0.1-phase0"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "scan":
		if err := cmdscan.Run(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "shhh:", err)
			os.Exit(1)
		}
	case "redact":
		if err := cmdredact.Run(os.Args[2:]); err != nil {
			fmt.Fprintln(os.Stderr, "shhh:", err)
			os.Exit(1)
		}
	case "version", "--version", "-v":
		fmt.Println("shhh", version)
	case "help", "--help", "-h":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "shhh: unknown command %q\n\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprint(os.Stderr, `shhh — stop leaking secrets to AI agents (Phase 0)

Usage:
  shhh scan [path]                    Scan a directory for secrets (default: .)
    --show-details                     Show host/user details (local only — avoid in screenshots)
    --format text|json|md              Output format (default: text)

  shhh redact <file>                  Redact a file, print to stdout
    --rehydrate                        Reverse operation: replace placeholders with real values

  shhh version                        Print version
  shhh help                           Show this message

Phase 0 ships scan and redact only. Hooks, proxy, and MCP come later.
`)
}
