// Package cmdredact implements `shhh redact`.
//
// Redact reads a file (or stdin), runs the detector, replaces findings with
// session-deterministic placeholders, and prints the result to stdout. With
// --rehydrate it reverses the operation using a session map loaded from an
// environment variable. Phase 0 uses this for eval-suite plumbing; the real
// rehydration flow lives in the Phase 4 proxy.
package cmdredact

import (
	"bytes"
	"flag"
	"fmt"
	"io"
	"os"

	"github.com/Musubi42/shhh/cmd/shhh/cmdhook"
	"github.com/Musubi42/shhh/internal/detector"
	"github.com/Musubi42/shhh/internal/redactor"
	"github.com/Musubi42/shhh/internal/session"
)

// Run is the entry point for `shhh redact`.
func Run(args []string) error {
	fs := flag.NewFlagSet("redact", flag.ContinueOnError)
	rehydrate := fs.Bool("rehydrate", false, "rehydrate placeholders back to real values (requires an active session)")
	sessionID := fs.String("session", "", "per-session store ID (used by the Claude Code Bash hook wrapper to share placeholders across tool calls)")
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

	var (
		r    *redactor.Redactor
		save func() error
	)
	if *sessionID != "" {
		r, save, err = cmdhook.LoadRedactor(*sessionID)
		if err != nil {
			// Fall back to an ephemeral redactor rather than failing
			// the pipeline — hook-adjacent code is best-effort.
			r = redactor.New(detector.New(), session.New())
			save = nil
		}
	} else {
		r = redactor.New(detector.New(), session.New())
	}

	if *rehydrate {
		// Phase 0 note: rehydrate on a fresh session map is a no-op because
		// the map is empty. This command exists primarily so the eval harness
		// can exercise the rehydrate code path. Phase 4 wires it to a real
		// shared session map via the daemon socket.
		os.Stdout.Write(r.Rehydrate(in))
		return nil
	}

	out, findings := r.Redact(in)
	os.Stdout.Write(out)
	// When invoked from the Bash hook (i.e. --session is set) and shhh
	// actually redacted something, append a narration footer so Claude
	// sees a per-finding map of label → output line → placeholder.
	// Intentionally gated on --session: standalone `shhh redact` use (eval
	// harness, manual CLI) must stay byte-exact on stdout.
	if *sessionID != "" && len(findings) > 0 {
		os.Stdout.Write(buildBashNarration(in, findings, r))
	}
	if save != nil {
		_ = save()
	}
	return nil
}

// buildBashNarration emits the same kind of per-finding listing the Read
// hook produces, but framed as a trailing note on command output rather
// than a separate additionalContext field. The Bash hook has no
// PostToolUse pathway to inject narration elsewhere, so the footer
// piggybacks on the redacted stdout that Claude reads anyway.
func buildBashNarration(original []byte, findings []detector.Finding, r *redactor.Redactor) []byte {
	var b bytes.Buffer
	// Leading newline ensures the marker starts on its own line even if
	// the command output didn't end in one.
	if len(original) > 0 && original[len(original)-1] != '\n' {
		b.WriteByte('\n')
	}
	b.WriteString("\n--- shhh (local secret-redaction tool) ")
	if len(findings) == 1 {
		b.WriteString("redacted 1 secret")
	} else {
		fmt.Fprintf(&b, "redacted %d secrets", len(findings))
	}
	b.WriteString(" from this command's output. Real values never reached you and never left the user's machine.\n")
	for _, f := range findings {
		line := lineNumberAt(original, f.Start)
		placeholder := r.PlaceholderForFinding(f)
		fmt.Fprintf(&b, "  - %s at output line %d (placeholder: %s)\n", f.Label, line, placeholder)
	}
	b.WriteString("---\n")
	return b.Bytes()
}

// lineNumberAt returns the 1-based line number of byte offset off in
// content. Duplicated here (rather than shared with cmdhook/read.go) to
// avoid exporting a near-trivial helper across packages.
func lineNumberAt(content []byte, off int) int {
	if off < 0 {
		off = 0
	}
	if off > len(content) {
		off = len(content)
	}
	return 1 + bytes.Count(content[:off], []byte{'\n'})
}
