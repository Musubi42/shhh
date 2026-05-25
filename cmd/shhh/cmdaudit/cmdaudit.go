// Package cmdaudit implements `shhh audit`, the forensic audit
// subcommand. It orchestrates the internal/audit package (which does
// the actual scanning) with the CLI/HTML/JSON renderers and the
// ephemeral report server.
//
// The design decisions behind this command live in
// docs/design/implementation-plan.md. The visual reference is
// docs/design/mockups/. The terminal output format is
// docs/design/cli-output.md.
package cmdaudit

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"

	auditpkg "github.com/Musubi42/shhh/internal/audit"
)

// Run is the entry point for `shhh audit [flags]`. Returns an error
// that main.go will print to stderr and exit with.
func Run(args []string) error {
	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var (
		noServe  = fs.Bool("no-serve", false, "terminal output only; no HTML file, no local server, exit when done (for CI and scripts)")
		htmlOnly = fs.Bool("html-only", false, "render the HTML report to disk but do not start a server; prints the file path")
		openFlag = fs.Bool("open", false, "launch the default browser on the report URL in addition to the CLI output")
		jsonOut  = fs.Bool("json", false, "emit machine-readable JSON to stdout instead of the human report; implies --no-serve")
	)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, `shhh audit — forensic audit of what your coding agent has already seen

Usage:
  shhh audit                 terminal output + local HTML report server (default)
  shhh audit --no-serve      terminal only, no server, exit on done
  shhh audit --html-only     terminal + write HTML to disk, no server
  shhh audit --open          default mode + launch browser on the report URL
  shhh audit --json          machine-readable JSON to stdout (implies --no-serve)

The audit is strictly local. No data leaves this machine. Raw secret
values never appear in any output — findings are rendered as typed
placeholders from the shhh session map.`)
	}
	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("audit: unexpected positional arguments: %v", fs.Args())
	}

	// Mutually exclusive checks.
	if *jsonOut {
		*noServe = true
		*htmlOnly = false
		*openFlag = false
	}
	if *noServe && *openFlag {
		return fmt.Errorf("--no-serve and --open are mutually exclusive")
	}
	if *noServe && *htmlOnly {
		return fmt.Errorf("--no-serve and --html-only are mutually exclusive")
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Trap SIGINT/SIGTERM so the server's Stop func runs cleanly on
	// Ctrl-C and we return control to the user's terminal.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	// Scan progress preamble goes to stderr so it doesn't pollute
	// stdout when piping to jq or redirecting --json to a file.
	progress := os.Stderr
	fmt.Fprintln(progress, "🛡️  shhh audit — scanning...")

	progressFn := func(source string, count int) {
		fmt.Fprintf(progress, "  ▸ %-18s %d items\n", source, count)
	}

	result, err := auditpkg.Run(ctx, auditpkg.Config{
		Agent:    "claude-code",
		Progress: progressFn,
	})
	if err != nil {
		return fmt.Errorf("audit run failed: %w", err)
	}

	// Persist the snapshot so the next audit can compute a delta.
	// Failure to save is non-fatal — the user still gets the report.
	if auditDir, derr := auditpkg.AuditDir(); derr == nil {
		if _, serr := auditpkg.SaveSnapshot(auditDir, result); serr != nil {
			fmt.Fprintf(os.Stderr, "shhh audit: warning: snapshot save failed: %v\n", serr)
		}
	}

	// ----- JSON mode: stdout only, nothing else ----------------------------
	if *jsonOut {
		return RenderJSON(os.Stdout, result)
	}

	// ----- Terminal rendering (all non-JSON modes) -------------------------
	useColor := shouldUseColor(os.Stdout)
	if err := RenderCLI(os.Stdout, result, useColor); err != nil {
		return fmt.Errorf("render cli: %w", err)
	}

	// ----- --no-serve: stop here --------------------------------------------
	if *noServe {
		return nil
	}

	// ----- HTML rendering: everything else needs a dir to serve -------------
	htmlDir, err := prepareHTMLOutputDir(result)
	if err != nil {
		return fmt.Errorf("prepare html output dir: %w", err)
	}
	if err := RenderHTML(htmlDir, result); err != nil {
		return fmt.Errorf("render html: %w", err)
	}
	indexPath := filepath.Join(htmlDir, "index.html")

	// ----- --html-only: print path and exit --------------------------------
	if *htmlOnly {
		fmt.Fprintln(os.Stdout)
		fmt.Fprintf(os.Stdout, "📄 HTML report written to: %s\n", indexPath)
		return nil
	}

	// ----- Default + --open: start the ephemeral server --------------------
	url, stop, err := ServeReport(htmlDir)
	if err != nil {
		// Server failure is not fatal — the HTML file is on disk.
		fmt.Fprintln(os.Stdout)
		fmt.Fprintf(os.Stdout, "📄 HTML report written to: %s\n", indexPath)
		fmt.Fprintf(os.Stdout, "(could not start local server: %v)\n", err)
		return nil
	}
	defer stop()

	fmt.Fprintln(os.Stdout)
	fmt.Fprintf(os.Stdout, "🌐 Full interactive report: %s\n", url)
	fmt.Fprintln(os.Stdout, "   Press Ctrl-C to stop the report server.")

	if *openFlag {
		_ = openBrowser(url)
	}

	// Block until interrupted or the context is otherwise cancelled.
	select {
	case <-sigCh:
		fmt.Fprintln(os.Stderr)
		fmt.Fprintln(os.Stderr, "shhh audit: stopping report server...")
	case <-ctx.Done():
	}
	return nil
}

// prepareHTMLOutputDir returns the directory the HTML renderer should
// write into. v0.2 uses <AuditDir>/html-<timestamp>/ so each audit's
// rendered report is its own directory (no clobbering, no stale files
// from a previous run bleeding into the current one).
func prepareHTMLOutputDir(r *auditpkg.Result) (string, error) {
	base, err := auditpkg.AuditDir()
	if err != nil {
		return "", err
	}
	name := "html-" + r.AuditTime.UTC().Format("2006-01-02T15-04-05Z")
	dir := filepath.Join(base, name)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// shouldUseColor decides whether to emit ANSI colors based on the
// destination file (stdout TTY or not) and the NO_COLOR env var
// (https://no-color.org).
func shouldUseColor(f *os.File) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	// A character device implies a TTY — good enough for our purposes.
	return (fi.Mode() & os.ModeCharDevice) != 0
}

// openBrowser launches the OS-default browser on the given URL. Errors
// are swallowed: failing to open a browser should NOT fail the audit
// command, since the URL is already on the user's terminal and they
// can click it themselves.
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", url)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	return cmd.Start()
}

// SaveAndReport is a small helper extracted so tests can call it
// without going through the full Run flow. It takes an already-
// computed Result, persists a snapshot, and renders any of the three
// output forms into the provided writer. Not currently used by Run
// but kept as a future integration point.
func SaveAndReport(w io.Writer, r *auditpkg.Result) error {
	auditDir, err := auditpkg.AuditDir()
	if err != nil {
		return err
	}
	if _, err := auditpkg.SaveSnapshot(auditDir, r); err != nil {
		return err
	}
	return RenderCLI(w, r, false)
}
