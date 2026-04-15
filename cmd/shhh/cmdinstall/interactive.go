package cmdinstall

import (
	"fmt"
	"os"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/x/term"
)

// RunInteractive is the entry point for `shhh install` with no target.
// The v0.2 flow is agent-first: pick agents, then configure each one
// (v0.2 only knows how to configure Claude Code, but the shape scales
// to N agents). See docs/design/installer-tui.md for the wireframes
// this mirrors.
//
// Non-TTY environments (CI, piped stdin) return a clear error rather
// than hanging on a form that can never be submitted.
func RunInteractive() error {
	if !isInteractive() {
		return fmt.Errorf("shhh install requires an interactive terminal; for scripted installs use `shhh install claude-code` directly")
	}

	binary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve shhh binary: %w", err)
	}
	cwd, _ := os.Getwd() // only used for the Plan's Cwd field

	detected := DetectInstalledAgents()
	if len(detected) == 0 {
		return fmt.Errorf("no supported coding agents detected on this machine (looked for ~/.claude). Install Claude Code first, then re-run `shhh install`")
	}

	// Welcome banner printed once. Deliberately unstyled — huh will
	// overwrite with its own theme once the first group runs.
	fmt.Println()
	fmt.Println("    shhh — stop leaking secrets to AI agents")
	fmt.Println("    Everything runs locally. Nothing leaves your machine.")
	fmt.Println()

	plan := &Plan{
		Scope:   ScopeGlobal, // v0.2: always global from the interactive flow
		Agents:  nil,
		Cwd:     cwd,
		RunScan: false,
	}

	// --- Group 1: which agents? -------------------------------------
	agentForm := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Which coding agents do you use?").
				Description("shhh will install its hook into each selected agent.\nUnsupported agents are listed in the footer.").
				Options(agentOptions(detected)...).
				Value(&plan.Agents).
				Validate(func(sel []string) error {
					if len(sel) == 0 {
						return fmt.Errorf("pick at least one agent")
					}
					return nil
				}),
		),
	)
	if err := agentForm.Run(); err != nil {
		return fmt.Errorf("installer cancelled: %w", err)
	}

	// --- Group 2..N: per-agent configuration -------------------------
	for _, agent := range plan.Agents {
		switch agent {
		case "claude-code":
			if err := configureClaudeCode(plan); err != nil {
				return err
			}
		default:
			// Defensive: the multi-select only surfaces supported
			// agents, but if something ever slips through we want a
			// clear error instead of silent skipping.
			return fmt.Errorf("no configuration wizard for agent %q yet", agent)
		}
	}

	// --- Final group: confirm + optional scan ------------------------
	finalForm := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title("Run a forensic audit now?").
				Description(
					"shhh will read these LOCAL files to find secrets already sent to Claude:\n" +
						"  • ~/.claude/projects/**/*.jsonl    session transcripts\n" +
						"  • ~/.claude/paste-cache/*.txt      pastes into Claude\n" +
						"  • ~/.claude/history.jsonl          prompt history\n" +
						"  • ~/.claude/file-history/**        file edit history\n\n" +
						"Nothing is sent over the network. Findings are redacted in the output.",
				).
				Affirmative("Run the audit now").
				Negative("Skip — I'll run `shhh audit` later").
				Value(&plan.RunScan),
		),
	)
	if err := finalForm.Run(); err != nil {
		return fmt.Errorf("installer cancelled: %w", err)
	}

	// Footer: advertise the roadmap for agents we don't yet support.
	fmt.Println()
	fmt.Println("Coming soon: Codex, Cursor. Track progress in docs/mvp/.")
	fmt.Println()

	if err := plan.Validate(); err != nil {
		return err
	}

	if err := plan.Execute(binary, os.Stdout); err != nil {
		return err
	}
	fmt.Println()
	fmt.Println("Done. Start a new coding-agent session for the hook to take effect.")
	fmt.Println("To audit your existing sessions any time: `shhh audit`")
	return nil
}

// configureClaudeCode runs the Claude-Code-specific configuration
// group: project multi-select. Mutates plan.SelectedProjects in place.
func configureClaudeCode(plan *Plan) error {
	projects, err := ListClaudeProjects()
	if err != nil {
		// Non-fatal: we can still install the hook even if we can't
		// list projects for audit scoping. Leave SelectedProjects
		// empty which means "audit all" at run time.
		fmt.Fprintf(os.Stderr, "shhh: could not list Claude projects: %v (continuing)\n", err)
		return nil
	}
	if len(projects) == 0 {
		fmt.Println()
		fmt.Println("Claude Code: no past sessions found — shhh will still protect future ones.")
		fmt.Println()
		return nil
	}

	// Build a huh MultiSelect with one option per project. Default:
	// everything selected, consistent with the wireframe intent (the
	// user's action is to OPT OUT of projects, not opt in).
	opts := make([]huh.Option[string], 0, len(projects))
	for _, p := range projects {
		label := p.DisplayPath
		if !p.OnDisk {
			label += "  (folder gone · transcripts retained)"
		}
		opts = append(opts, huh.NewOption(label, p.DashName).Selected(true))
	}

	// Start with all dash-names selected so a "just press enter"
	// user ends up with the default-all behavior.
	selected := make([]string, 0, len(projects))
	for _, p := range projects {
		selected = append(selected, p.DashName)
	}
	plan.SelectedProjects = selected

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Claude Code — projects to audit").
				Description(fmt.Sprintf(
					"shhh found %d projects in ~/.claude/projects/. Uncheck any\n"+
						"project you do NOT want `shhh audit` to read.",
					len(projects),
				)).
				Options(opts...).
				Value(&plan.SelectedProjects).
				Filterable(true),
		),
	)
	if err := form.Run(); err != nil {
		return fmt.Errorf("claude-code configuration cancelled: %w", err)
	}
	return nil
}

// agentOptions builds the multi-select options list. Only agents that
// are both detected on this machine AND supported in this build appear.
// Unsupported agents are shown in a footer line, not as disabled
// options (huh doesn't offer per-option disable, and fake-selectable
// "coming soon" options would just frustrate the user).
func agentOptions(detected []string) []huh.Option[string] {
	want := map[string]bool{}
	for _, a := range detected {
		want[a] = true
	}
	opts := []huh.Option[string]{}
	if want["claude-code"] {
		opts = append(opts, huh.NewOption("Claude Code  (detected at ~/.claude)", "claude-code").Selected(true))
	}
	return opts
}

// isInteractive reports whether stdin is a real terminal. huh opens
// /dev/tty directly rather than reading stdin, so a simple stdin check
// isn't enough — we need to know whether a TTY is actually reachable.
// x/term.IsTerminal is the canonical answer, already pulled in as a
// transitive dep of huh.
func isInteractive() bool {
	return term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd())
}
