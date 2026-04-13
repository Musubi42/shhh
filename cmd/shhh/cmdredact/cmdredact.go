// Package cmdredact implements `shhh redact`.
//
// Redact reads a file (or stdin), runs the detector, replaces findings with
// session-deterministic placeholders, and prints the result to stdout. With
// --rehydrate it reverses the operation using a session map loaded from an
// environment variable. Phase 0 uses this for eval-suite plumbing; the real
// rehydration flow lives in the Phase 4 proxy.
package cmdredact

import (
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/musubi-sasu/shhh/internal/detector"
	"github.com/musubi-sasu/shhh/internal/redactor"
	"github.com/musubi-sasu/shhh/internal/session"
)

// Run is the entry point for `shhh redact`.
func Run(args []string) error {
	fs := flag.NewFlagSet("redact", flag.ContinueOnError)
	rehydrate := fs.Bool("rehydrate", false, "rehydrate placeholders back to real values (requires an active session)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	var in []byte
	var err error
	if fs.NArg() == 0 {
		in, err = io.ReadAll(os.Stdin)
	} else {
		in, err = os.ReadFile(fs.Arg(0))
	}
	if err != nil {
		return fmt.Errorf("read input: %w", err)
	}

	r := redactor.New(detector.New(), session.New())

	if *rehydrate {
		// Phase 0 note: rehydrate on a fresh session map is a no-op because
		// the map is empty. This command exists primarily so the eval harness
		// can exercise the rehydrate code path. Phase 4 wires it to a real
		// shared session map via the daemon socket.
		os.Stdout.Write(r.Rehydrate(in))
		return nil
	}

	out, _ := r.Redact(in)
	os.Stdout.Write(out)
	return nil
}
