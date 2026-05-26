// Package cmddoctor implements `shhh doctor` — a health check that
// surfaces stale install entries, missing hooks, and binary drift.
// Born from the 2026-05-26 dryrun where ~/.shhh/config.json claimed
// the global hook was installed while ~/.claude/settings.json had
// "hooks": {} empty — the audit's status badge was honest (it
// defensively re-reads each settings.json) but the config file
// itself stayed wrong until the user noticed.
//
// `shhh doctor` is the explicit way for the user to ask "is shhh
// healthy?", without mutating anything by default. `--fix` opts in
// to dropping stale entries from the config.
package cmddoctor

import (
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Musubi42/shhh/cmd/shhh/cmdinstall"
	"github.com/charmbracelet/x/term"
)

// lookPath is a thin alias so tests can stub the PATH lookup. The
// alias also keeps the import list short for the doctor's call site.
var lookPath = exec.LookPath

// Run is the entry point for `shhh doctor [--fix]`.
func Run(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	fix := fs.Bool("fix", false, "drop stale installed_paths entries from ~/.shhh/config.json (does not modify settings.json)")
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, `shhh doctor — health check for shhh's install state

Usage:
  shhh doctor          report on config + installs (read-only)
  shhh doctor --fix    drop installed_paths entries that no longer have a shhh hook

What it checks:
  • Binary on $PATH matches the one currently running
  • ~/.shhh/config.json loads cleanly
  • Each installed_paths entry: file exists AND has the shhh hook
  • Friendly reminder to restart Claude sessions if you just changed installs`)
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	c := pickColors(os.Stdout)
	report := runChecks()
	report.print(os.Stdout, c)

	if *fix && report.staleConfigEntries > 0 {
		dropped, err := healStaleEntries(report)
		if err != nil {
			return fmt.Errorf("doctor --fix: %w", err)
		}
		fmt.Fprintf(os.Stdout, "\n  %s✓%s --fix: dropped %d stale entr%s from config\n",
			c.green, c.reset, dropped, pluralY(dropped))
	}
	return nil
}

// checkResult is one line in the doctor's report.
type checkResult struct {
	level   level
	label   string   // primary one-line status
	details []string // optional indented follow-ups (suggestions, file paths)
}

type level int

const (
	levelOK level = iota
	levelWarn
	levelError
)

type doctorReport struct {
	results            []checkResult
	staleConfigEntries int
	// settingsPathsToHeal is the subset of installed_paths whose
	// referenced settings.json no longer contains a shhh hook —
	// `--fix` drops these from the config.
	settingsPathsToHeal []string
}

func runChecks() *doctorReport {
	r := &doctorReport{}

	// 1. Binary: where is `shhh` and is it what's running?
	r.results = append(r.results, checkBinary())

	// 2. Config file: loads and has a recognizable shape.
	cfg, cfgRes := checkConfig()
	r.results = append(r.results, cfgRes)

	// 3. Each installed_paths entry.
	if cfg != nil {
		entries, stale := checkInstalledPaths(cfg)
		r.results = append(r.results, entries...)
		r.staleConfigEntries = len(stale)
		r.settingsPathsToHeal = stale
	}

	// 4. Friendly reminder.
	r.results = append(r.results, checkResult{
		level: levelOK,
		label: "If you just installed or uninstalled, restart any running `claude` sessions",
		details: []string{
			"Claude Code reads settings.json at session start.",
			"Existing sessions won't pick up hook changes until restart.",
		},
	})

	return r
}

func checkBinary() checkResult {
	running, _ := os.Executable()
	running, _ = filepath.Abs(running)
	res := checkResult{
		level: levelOK,
		label: fmt.Sprintf("running binary at %s", tildePath(running)),
	}
	// Look up the binary that would be invoked from the user's shell.
	// On_path is the absolute path of `shhh` as resolved through $PATH,
	// independent of how this process was invoked.
	onPath, err := lookPath("shhh")
	if err != nil {
		res.level = levelWarn
		res.label = "running binary at " + tildePath(running) + " (no `shhh` found on $PATH)"
		res.details = append(res.details, "Future `shhh` invocations from a fresh shell may fail.")
		return res
	}
	onPath, _ = filepath.Abs(onPath)
	if onPath != running {
		res.level = levelWarn
		res.label = fmt.Sprintf("running binary differs from $PATH copy")
		res.details = append(res.details,
			"on $PATH: "+tildePath(onPath),
			"running:  "+tildePath(running),
			"Reinstall to align: `install -m 0755 bin/shhh $(dirname "+tildePath(onPath)+")/shhh`",
		)
	}
	return res
}

func checkConfig() (*cmdinstall.Config, checkResult) {
	cfg, err := cmdinstall.LoadConfig()
	if err != nil {
		return nil, checkResult{
			level:   levelError,
			label:   "~/.shhh/config.json failed to load",
			details: []string{err.Error()},
		}
	}
	if cfg == nil {
		return nil, checkResult{
			level: levelWarn,
			label: "~/.shhh/config.json does not exist yet",
			details: []string{
				"Run `shhh install claude-code` to create it.",
			},
		}
	}
	return cfg, checkResult{
		level: levelOK,
		label: fmt.Sprintf("~/.shhh/config.json valid (%d installed path%s)",
			len(cfg.Paths), plural(len(cfg.Paths))),
	}
}

func checkInstalledPaths(cfg *cmdinstall.Config) ([]checkResult, []string) {
	var results []checkResult
	var stale []string
	verified := 0
	for _, p := range cfg.Paths {
		switch reason := classifyInstallPath(p); reason {
		case "":
			verified++
		case "missing":
			results = append(results, checkResult{
				level: levelWarn,
				label: fmt.Sprintf("installed_paths entry: %s", tildePath(p)),
				details: []string{
					"settings.json file does not exist",
					"Suggested: `shhh doctor --fix` to drop the stale entry",
				},
			})
			stale = append(stale, p)
		case "no-hook":
			results = append(results, checkResult{
				level: levelWarn,
				label: fmt.Sprintf("installed_paths entry: %s", tildePath(p)),
				details: []string{
					"file exists but contains no shhh hook",
					"Suggested: `shhh install claude-code` (or `shhh doctor --fix` to forget this entry)",
				},
			})
			stale = append(stale, p)
		case "unreadable":
			results = append(results, checkResult{
				level: levelError,
				label: fmt.Sprintf("installed_paths entry: %s", tildePath(p)),
				details: []string{
					"cannot read file — permission denied?",
				},
			})
		}
	}
	if verified > 0 {
		results = append(results, checkResult{
			level: levelOK,
			label: fmt.Sprintf("%d install%s verified", verified, plural(verified)),
		})
	}
	return results, stale
}

// classifyInstallPath returns "" when the install at path is healthy,
// or one of "missing" / "no-hook" / "unreadable" otherwise.
func classifyInstallPath(path string) string {
	buf, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "missing"
		}
		return "unreadable"
	}
	// Same trick as internal/audit/run.go::settingsHasShhhHook: a
	// substring scan for the hook subcommand is enough and decouples
	// us from settings.json schema changes.
	if !strings.Contains(string(buf), "shhh hook") {
		return "no-hook"
	}
	return ""
}

func healStaleEntries(r *doctorReport) (int, error) {
	cfg, err := cmdinstall.LoadConfig()
	if err != nil || cfg == nil {
		return 0, fmt.Errorf("config load: %w", err)
	}
	toDrop := make(map[string]bool, len(r.settingsPathsToHeal))
	for _, p := range r.settingsPathsToHeal {
		toDrop[p] = true
	}
	kept := cfg.Paths[:0]
	for _, p := range cfg.Paths {
		if !toDrop[p] {
			kept = append(kept, p)
		}
	}
	cfg.Paths = kept
	// Re-derive scope from what remains.
	cfg.Scope = deriveScope(cfg.Paths)
	if err := cmdinstall.SaveConfig(cfg); err != nil {
		return 0, err
	}
	return len(toDrop), nil
}

// deriveScope mirrors cmdinstall.inferScopeFromPaths but is duplicated
// here to keep cmddoctor's dependency surface narrow (and because the
// install package's variant is unexported).
func deriveScope(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	home, _ := os.UserHomeDir()
	globalHint := ""
	if home != "" {
		globalHint = filepath.Join(home, ".claude", "settings.json")
	}
	for _, p := range paths {
		if globalHint != "" && filepath.Clean(p) == filepath.Clean(globalHint) {
			return "global"
		}
	}
	return "project"
}

// ---- printing -------------------------------------------------------------

func (r *doctorReport) print(w io.Writer, c ansi) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Checking shhh state...")
	issues := 0
	for _, res := range r.results {
		sym, col := symbolFor(res.level, c)
		fmt.Fprintf(w, "  %s%s%s %s\n", col, sym, c.reset, res.label)
		for _, d := range res.details {
			fmt.Fprintf(w, "      %s%s%s\n", c.dim, d, c.reset)
		}
		if res.level != levelOK {
			issues++
		}
	}
	fmt.Fprintln(w)
	if issues == 0 {
		fmt.Fprintf(w, "%sAll checks passed.%s\n", c.dim, c.reset)
		return
	}
	noun := "issue"
	if issues > 1 {
		noun = "issues"
	}
	fmt.Fprintf(w, "%d %s found.\n", issues, noun)
}

func symbolFor(l level, c ansi) (string, string) {
	switch l {
	case levelOK:
		return "✓", c.green
	case levelWarn:
		return "⚠", c.yellow
	case levelError:
		return "✗", c.red
	}
	return "·", ""
}

// ---- helpers --------------------------------------------------------------

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func pluralY(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

func tildePath(p string) string {
	if p == "" {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if strings.HasPrefix(p, home+"/") {
		return "~" + p[len(home):]
	}
	return p
}

// ---- ANSI colors ----------------------------------------------------------

// ansi is the local color set. Mirrors cmdinstall's so the doctor's
// output palette matches install/uninstall diffs. Kept inline rather
// than imported to keep cmddoctor's dependency surface narrow.
type ansi struct {
	green, yellow, red, dim, reset string
}

func pickColors(out *os.File) ansi {
	if os.Getenv("NO_COLOR") != "" {
		return ansi{}
	}
	if !term.IsTerminal(out.Fd()) {
		return ansi{}
	}
	return ansi{
		green:  "\x1b[32m",
		yellow: "\x1b[33m",
		red:    "\x1b[31m",
		dim:    "\x1b[2m",
		reset:  "\x1b[0m",
	}
}
