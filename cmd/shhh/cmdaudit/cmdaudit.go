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
	"strings"
	"syscall"

	"github.com/Musubi42/shhh/cmd/shhh/cmdinstall"
	auditpkg "github.com/Musubi42/shhh/internal/audit"
)

// formatScopeLabel turns the resolved absolute scope paths back
// into a compact, tilde-abbreviated display string for the audit
// announcement. One path → "scope <p>"; two+ → "scope N paths".
// Keeps the live UI from wrapping when the user passed five repos.
func formatScopeLabel(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	home, _ := os.UserHomeDir()
	tilde := func(p string) string {
		if home != "" && p == home {
			return "~"
		}
		if home != "" && strings.HasPrefix(p, home+"/") {
			return "~" + p[len(home):]
		}
		return p
	}
	if len(paths) == 1 {
		return "scope " + tilde(paths[0])
	}
	return fmt.Sprintf("scope %d paths", len(paths))
}

// splitFlagsAndPositionals walks args and returns two slices: every
// arg starting with `-` (the flags) and everything else (the
// positionals). cmdaudit's flags are all booleans, so this split is
// unambiguous — no flag needs a value-token, so no flag can "steal"
// a following positional.
//
// The split exists because Go's stdlib `flag.Parse` stops at the
// first non-flag argument, which silently swallows any flag written
// after a positional. `shhh audit . --no-serve` would otherwise treat
// `--no-serve` as a path. Users (rightly) expect order to be
// irrelevant. If cmdaudit ever grows a string/int flag, this helper
// must learn to consume the following value token.
func splitFlagsAndPositionals(args []string) (flags, positionals []string) {
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
		} else {
			positionals = append(positionals, a)
		}
	}
	return
}

// Run is the entry point for `shhh audit [flags]`. Returns an error
// that main.go will print to stderr and exit with.
func Run(args []string) error {
	// Subcommands for the ignore list. Handled before flag parsing
	// so their arguments aren't interpreted as audit flags.
	if len(args) >= 1 {
		switch args[0] {
		case "ignore":
			return runIgnore(args[1:], true)
		case "unignore":
			return runIgnore(args[1:], false)
		case "ignored":
			return runIgnoredList()
		}
	}

	fs := flag.NewFlagSet("audit", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var (
		noServe  = fs.Bool("no-serve", false, "terminal output only; no HTML file, no local server, exit when done (for CI and scripts)")
		htmlOnly = fs.Bool("html-only", false, "render the HTML report to disk but do not start a server; prints the file path")
		htmlAlt  = fs.Bool("html", false, "alias for --html-only")
		openFlag = fs.Bool("open", false, "launch the default browser on the report URL in addition to the CLI output")
		jsonOut  = fs.Bool("json", false, "emit machine-readable JSON to stdout instead of the human report; implies --no-serve")
		noSelect = fs.Bool("no-select", false, "skip the interactive project picker and audit every non-ignored project (implied in non-TTY contexts)")
	)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, `shhh audit — forensic audit of what your coding agent has already seen

Usage:
  shhh audit                       interactive project picker, then default terminal + HTML report
  shhh audit .                     scope to the current directory (any positional path works)
  shhh audit <path> [<path>...]    scope to the given project root(s); recursive match
  shhh audit --no-select           skip the picker (audit every non-ignored project)
  shhh audit --no-serve            terminal only, no server, exit on done
  shhh audit --html-only, --html   terminal + write HTML to disk, no server
  shhh audit --open                default mode + launch browser on the report URL
  shhh audit --json                machine-readable JSON to stdout (implies --no-serve and --no-select)

  shhh audit ignore <path>         skip a project on future audits (also editable via the picker)
  shhh audit unignore <path>       put it back in scope
  shhh audit ignored               list ignored projects

The audit is strictly local. No data leaves this machine. Raw secret
values never appear in any output — findings are rendered as typed
placeholders from the shhh session map.`)
	}
	flagArgs, positionalArgs := splitFlagsAndPositionals(args)
	if err := fs.Parse(flagArgs); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	// Positional args are project-path scope filters. Each is resolved
	// to absolute form so `shhh audit .` works no matter where the
	// user invoked it from. Filter is applied inside auditpkg.Run via
	// Config.ScopePaths (step 2a').
	var scopePaths []string
	for _, p := range positionalArgs {
		abs, err := filepath.Abs(p)
		if err != nil {
			return fmt.Errorf("audit: resolve %q: %w", p, err)
		}
		scopePaths = append(scopePaths, abs)
	}

	// --html is an alias for --html-only. Folding here keeps the rest
	// of the function unaware of the alternative spelling.
	if *htmlAlt {
		*htmlOnly = true
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

	// Trap SIGINT/SIGTERM. Two responsibilities:
	//
	//   1. While the audit is running (possibly minutes on a busy
	//      machine), a Ctrl-C must cancel the context promptly. We
	//      spawn a watcher goroutine that calls cancel() on the first
	//      signal — without it, signal.Notify swallows Ctrl-C and the
	//      user has to kill the terminal.
	//   2. After the audit finishes and the HTML server is up, the
	//      same channel is used to shut it down cleanly.
	//
	// A second Ctrl-C exits hard (os.Exit) so an unresponsive shutdown
	// doesn't trap the user.
	sigCh := make(chan os.Signal, 2)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(sigCh)

	auditDone := make(chan struct{})
	go func() {
		select {
		case <-sigCh:
			fmt.Fprintln(os.Stderr)
			fmt.Fprintln(os.Stderr, "shhh audit: stopping (Ctrl-C again to force quit)...")
			cancel()
			// On a second signal, exit hard. Don't try to clean up —
			// the user has told us twice that they want out.
			<-sigCh
			fmt.Fprintln(os.Stderr, "shhh audit: forced exit.")
			os.Exit(130) // 128 + SIGINT
		case <-auditDone:
			// Audit completed; the post-audit select below takes over.
		}
	}()

	// Live progress goes to stderr so it doesn't pollute stdout when
	// piping to jq or redirecting --json to a file. TTY detection here
	// (not on stdout) because the live UI lives on the same stream as
	// the progress, not the final report.
	progress := os.Stderr
	progressTTY := isTTY(progress)
	renderer := newProgressRenderer(progress, progressTTY)
	if len(scopePaths) > 0 {
		renderer = renderer.withScope(formatScopeLabel(scopePaths))
	}

	// Pull the user's persistent ignore list from ~/.shhh/config.json.
	// Audit honors it before any scanning happens. Failure to load is
	// non-fatal: empty list == audit everything.
	var ignored []string
	if cfg, lerr := cmdinstall.LoadConfig(); lerr == nil && cfg != nil {
		ignored = cfg.IgnoredPaths
	}

	// Interactive project picker: default behavior on TTY. The picker
	// shows every project (incl. folder-gone) pre-checked except those
	// already in the ignore list. Unchecking a project persists it to
	// the ignore list; re-checking removes it. --no-select skips this
	// (CI/scripts), and we auto-skip when stderr (the picker's output)
	// is not a terminal.
	// Skip the picker when the user already narrowed scope via
	// positional paths — the picker would just confirm what they
	// typed.
	if !*noSelect && !*jsonOut && len(scopePaths) == 0 && isTTY(os.Stderr) {
		updated, perr := runProjectPicker()
		if perr != nil {
			return fmt.Errorf("project picker: %w", perr)
		}
		ignored = updated
	}

	result, err := auditpkg.Run(ctx, auditpkg.Config{
		Agent:        "claude-code",
		IgnoredPaths: ignored,
		ScopePaths:   scopePaths,
		OnProgress:   renderer.Handle,
	})
	// Audit is over (success, error, or cancelled). Release the
	// watcher goroutine so the post-audit select below owns sigCh.
	close(auditDone)
	if err != nil {
		if ctx.Err() != nil {
			// User-initiated cancel via Ctrl-C — don't surface a
			// scary "audit run failed" message.
			return nil
		}
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
	return isTTY(f)
}

// isTTY reports whether f is connected to a terminal. Used by both
// the color toggle and the live progress renderer.
func isTTY(f *os.File) bool {
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
