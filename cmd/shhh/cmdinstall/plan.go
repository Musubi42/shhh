package cmdinstall

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// Plan captures the outcome of the interactive installer's prompts and
// is the unit the test suite exercises. Separating Plan from the TUI
// means the installer's behavior can be verified without a TTY: tests
// build a Plan directly and call Execute. The TUI is a thin adapter
// that builds a Plan from user answers.
type Plan struct {
	// Scope is either ScopeGlobal or ScopeProject. Global writes into
	// ~/.<agent>/settings.json; project writes into <cwd>/.<agent>/settings.json.
	//
	// The v0.2 interactive TUI always sets this to ScopeGlobal (the
	// global/project choice was dropped as a useless prompt). The
	// scripted `shhh install claude-code` path still honors both for
	// power users, which is why the field stays on Plan.
	Scope Scope

	// Agents is the list of agents to hook into. Order does not matter.
	// Unknown agents cause Execute to fail with a clear error.
	Agents []string

	// Cwd is the directory used when Scope == ScopeProject. Ignored
	// otherwise. Must be an absolute path when the caller wants
	// reproducible behavior; Execute takes it as-is.
	Cwd string

	// RunScan, when true, runs `shhh scan <cwd-or-.>` after the install
	// step completes. Currently defaults to false (installer opt-in).
	RunScan bool

	// SelectedProjects is the per-agent opt-in list of project dash-
	// names to include in audit runs. Empty means "all projects the
	// agent knows about." The TUI presents the full list and lets the
	// user uncheck projects they don't want audited; the selected
	// subset is persisted into Config so `shhh audit` honors it.
	//
	// Dash-names are the raw directory names under ~/.claude/projects/
	// (e.g. "-Users-alice-work-backend"). The audit layer decodes
	// them back to absolute paths when it needs to.
	SelectedProjects []string
}

// Validate reports any reasons this plan cannot be executed. Used by the
// TUI to block submission and by tests to assert error paths. Returns
// nil when the plan is actionable.
func (p *Plan) Validate() error {
	switch p.Scope {
	case ScopeGlobal, ScopeProject:
	default:
		return fmt.Errorf("plan: invalid scope %q (want global or project)", p.Scope)
	}
	if len(p.Agents) == 0 {
		return fmt.Errorf("plan: no agents selected")
	}
	for _, a := range p.Agents {
		switch a {
		case "claude-code":
		default:
			return fmt.Errorf("plan: agent %q is not supported yet", a)
		}
	}
	if p.Scope == ScopeProject && p.Cwd == "" {
		return fmt.Errorf("plan: project scope requires cwd")
	}
	return nil
}

// Execute applies the plan. For each selected agent it resolves the
// target settings.json path (global vs project), calls the existing
// Install merger, appends the path to the persisted user config, and
// writes the config back. Returns a human-readable report describing
// what was done — the interactive installer prints this to the user.
//
// binary is the absolute path to the currently-running shhh binary,
// used as the hook command in each agent's settings.json. The caller
// is expected to have resolved it via os.Executable().
//
// Execute is intentionally side-effectful on disk: it's the place where
// the installer actually commits. Tests point SHHH_CONFIG_DIR and
// CLAUDE_CONFIG_DIR at temp directories to keep it hermetic.
func (p *Plan) Execute(binary string, out io.Writer) error {
	if err := p.Validate(); err != nil {
		return err
	}

	// Load existing config so re-installs don't clobber history.
	cfg, err := LoadConfig()
	if err != nil {
		return err
	}
	if cfg == nil {
		cfg = &Config{}
	}
	cfg.Scope = string(p.Scope)
	cfg.Agents = uniqStrings(append(cfg.Agents, p.Agents...))
	// SelectedProjects is replaced (not merged) so unchecking a
	// project in a re-run actually drops it from the audit scope.
	cfg.SelectedProjects = append([]string{}, p.SelectedProjects...)

	for _, agent := range p.Agents {
		path, err := AgentSettingsPath(agent, p.Scope, p.Cwd)
		if err != nil {
			return fmt.Errorf("resolve %s settings path: %w", agent, err)
		}
		diff, err := Install(path, binary)
		if err != nil {
			return fmt.Errorf("install %s: %w", agent, err)
		}
		cfg.AddInstalledPath(path)
		fmt.Fprintf(out, "✓ %s: %s\n", agent, path)
		if diff == "" {
			fmt.Fprintf(out, "  (already configured — no changes)\n")
		}
	}

	if err := SaveConfig(cfg); err != nil {
		return fmt.Errorf("save shhh config: %w", err)
	}
	cfgPath, _ := ConfigPath()
	fmt.Fprintf(out, "✓ shhh config: %s\n", cfgPath)

	// RunScan is wired here so the installer's "run a scan now?" prompt
	// can be honored without the installer caller having to know about
	// cmdscan. Scan failures are non-fatal — the install already
	// succeeded, the scan is just a demo step.
	if p.RunScan {
		scanTarget := p.Cwd
		if scanTarget == "" {
			scanTarget = "."
		}
		fmt.Fprintf(out, "\n")
		if err := RunScanHook(out, scanTarget); err != nil {
			fmt.Fprintf(out, "(scan failed: %v — install itself succeeded)\n", err)
		}
	} else {
		fmt.Fprintf(out, "\n")
		fmt.Fprintf(out, "You can run a scan any time with:\n")
		fmt.Fprintf(out, "    shhh scan %s\n", defaultScanTarget(p.Cwd))
	}

	return nil
}

// defaultScanTarget picks a sensible path to suggest in the "you can run
// a scan any time" footer. For project scope, the cwd; for global, ".".
func defaultScanTarget(cwd string) string {
	if cwd != "" {
		return cwd
	}
	return "."
}

// RunScanHook is the install-time bridge to `shhh scan`. The main
// binary wires this to cmdscan.Run at init time; cmdinstall cannot
// import cmdscan directly without an import cycle (cmdscan imports
// nothing from here, but the cmdredact package pulls cmdinstall via
// cmdhook). Tests replace this with a stub.
var RunScanHook = func(out io.Writer, target string) error {
	fmt.Fprintf(out, "scan: (not wired in this build)\n")
	_ = target
	return nil
}

// uniqStrings returns s with duplicates removed, preserving first-seen
// order.
func uniqStrings(s []string) []string {
	seen := make(map[string]struct{}, len(s))
	out := make([]string, 0, len(s))
	for _, v := range s {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// DescribeScope returns a human-friendly label for the TUI prompt that
// includes the resolved absolute path so the user sees exactly where
// the install is about to land.
func DescribeScope(scope Scope, cwd string) string {
	switch scope {
	case ScopeGlobal:
		path, err := claudeSettingsPath()
		if err != nil {
			return "Everywhere on this machine"
		}
		return fmt.Sprintf("Everywhere on this machine (%s)", tildePath(path))
	case ScopeProject:
		if cwd == "" {
			cwd, _ = os.Getwd()
		}
		return fmt.Sprintf("Only in the current directory (%s)", tildePath(cwd))
	default:
		return string(scope)
	}
}

// tildePath replaces the user's home directory prefix with ~ for
// display. Purely cosmetic; the underlying paths handed to the install
// logic remain absolute.
func tildePath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if strings.HasPrefix(p, home+string(filepath.Separator)) {
		return "~" + p[len(home):]
	}
	return p
}
