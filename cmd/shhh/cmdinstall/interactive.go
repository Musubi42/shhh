package cmdinstall

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

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
		Scope:   ScopeGlobal,
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

	// --- Group 1a: which detection engines? ---------------------------
	// gitleaks is the default (maintained third-party rules, MIT) +
	// shhh-native adds env cross-reference and structural URL handling.
	// At least one engine is required; the validator blocks submission
	// otherwise. See docs/engine-architecture.md §2.4.
	if err := chooseEngines(plan); err != nil {
		return err
	}

	// --- Group 1b: install scope -------------------------------------
	// Global is the default and matches what most users want. Project
	// scope is for users who want shhh active only in selected repos —
	// typically because they intend to commit .claude/settings.json so
	// teammates inherit shhh automatically. We source the candidate
	// project list from ~/.claude/projects/ (where the user has
	// actually run Claude Code) rather than asking them to type a path.
	if err := chooseScope(plan); err != nil {
		return err
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

// chooseEngines prompts for the detection-engine selection and
// stores it in plan.Engines. Defaults pre-select gitleaks, leaving
// shhh-native available as an additive. At least one engine must
// stay checked.
//
// Re-installs: if the persisted Config already has Engines, those
// are pre-selected. The validator still requires ≥1 after editing.
func chooseEngines(plan *Plan) error {
	preselect := []string{"gitleaks"}
	if cfg, _ := LoadConfig(); cfg != nil && len(cfg.Engines) > 0 {
		preselect = cfg.Engines
	}
	plan.Engines = append([]string{}, preselect...)
	wanted := map[string]bool{}
	for _, e := range preselect {
		wanted[e] = true
	}

	gitleaksOpt := huh.NewOption(
		"gitleaks  — ~222 rules, MIT (github.com/gitleaks)", "gitleaks").
		Selected(wanted["gitleaks"])
	nativeOpt := huh.NewOption(
		"shhh-native  — env cross-reference + URL-structural redaction", "shhh-native").
		Selected(wanted["shhh-native"])

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Detection engines (at least one required)").
				Description(
					"gitleaks ships a curated allowlist (lockfiles, vendor, binaries) so\n"+
						"shhh skips them automatically. shhh-native catches values copied\n"+
						"from .env into code and preserves DB-URL host/db visibility.").
				Options(gitleaksOpt, nativeOpt).
				Value(&plan.Engines).
				Validate(func(sel []string) error {
					if len(sel) == 0 {
						return fmt.Errorf("pick at least one engine")
					}
					return nil
				}),
		),
	)
	if err := form.Run(); err != nil {
		return fmt.Errorf("installer cancelled: %w", err)
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
	if want["codex"] {
		opts = append(opts, huh.NewOption("Codex  (detected at ~/.codex — Bash-only coverage, see docs)", "codex").Selected(true))
	}
	if want["cursor"] {
		opts = append(opts, huh.NewOption("Cursor IDE  (detected at ~/.cursor — restart Cursor after install)", "cursor").Selected(true))
	}
	return opts
}

// chooseScope asks the user whether to install globally or only in a
// selected set of projects. For the project case it presents the
// multiselect of known Claude-Code project directories (from
// ~/.claude/projects/) so the user picks from a concrete list rather
// than typing paths. Mutates plan.Scope and plan.ProjectPaths.
func chooseScope(plan *Plan) error {
	const (
		choiceGlobal  = "global"
		choiceProject = "project"
	)
	scopeChoice := choiceGlobal
	scopeForm := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Title("Where to install shhh?").
				Description("Global protects every Claude Code session on this machine.\n" +
					"Per-project writes <project>/.claude/settings.json — commit it\n" +
					"to a repo so teammates inherit shhh automatically.").
				Options(
					huh.NewOption("Global  (every Claude Code session on this machine)", choiceGlobal),
					huh.NewOption("This project only  (pick from your Claude history)", choiceProject),
				).
				Value(&scopeChoice),
		),
	)
	if err := scopeForm.Run(); err != nil {
		return fmt.Errorf("installer cancelled: %w", err)
	}
	if scopeChoice == choiceGlobal {
		plan.Scope = ScopeGlobal
		return nil
	}

	// Project scope: source candidates from the user's Claude history.
	// Only OnDisk projects are eligible — installing into a directory
	// that doesn't exist anymore would create a stray .claude/.
	projects, err := ListClaudeProjects()
	if err != nil {
		return fmt.Errorf("list claude projects: %w", err)
	}
	var eligible []ClaudeProject
	for _, p := range projects {
		if p.OnDisk {
			eligible = append(eligible, p)
		}
	}

	// The "custom path" sentinel sits at the top of the list so users
	// can install in a repo Claude has never been opened in (fresh
	// clone, brand-new project). Picking it triggers a follow-up Input
	// prompt that accepts one path per loop until empty.
	const customSentinel = "__custom_path__"
	opts := []huh.Option[string]{
		huh.NewOption("✍ Type a custom path... (for repos not in your Claude history)", customSentinel),
	}
	for _, p := range eligible {
		opts = append(opts, huh.NewOption(p.DisplayPath, p.AbsPath))
	}

	var selected []string
	description := fmt.Sprintf(
		"Found %d project director%s in your Claude history. shhh will\n"+
			"write <project>/.claude/settings.json in each one you pick.",
		len(eligible), pluralY(len(eligible)))
	if len(eligible) == 0 {
		description = "No project directories found in your Claude history.\n" +
			"Use the 'Type a custom path...' option to install in an arbitrary repo."
	}
	projForm := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title("Which projects to install shhh into?").
				Description(description).
				Options(opts...).
				Value(&selected).
				Filterable(true).
				Validate(func(sel []string) error {
					if len(sel) == 0 {
						return fmt.Errorf("pick at least one project (or restart and choose Global)")
					}
					return nil
				}),
		),
	)
	if err := projForm.Run(); err != nil {
		return fmt.Errorf("installer cancelled: %w", err)
	}

	// Separate custom-path sentinel from real selections.
	var chosen []string
	wantsCustom := false
	for _, s := range selected {
		if s == customSentinel {
			wantsCustom = true
			continue
		}
		chosen = append(chosen, s)
	}

	if wantsCustom {
		customPaths, err := promptCustomPaths()
		if err != nil {
			return err
		}
		chosen = append(chosen, customPaths...)
	}

	if len(chosen) == 0 {
		return fmt.Errorf("no project paths selected")
	}
	plan.Scope = ScopeProject
	plan.ProjectPaths = chosen
	return nil
}

// promptCustomPaths repeatedly asks the user for a project path until
// they submit an empty line. Each entered path is resolved to its
// absolute form and validated as an existing directory; rejected
// inputs re-prompt rather than aborting the whole installer.
func promptCustomPaths() ([]string, error) {
	var paths []string
	for {
		var raw string
		input := huh.NewForm(
			huh.NewGroup(
				huh.NewInput().
					Title("Path to project").
					Description("Absolute or relative path. Leave empty and press enter to finish.").
					Value(&raw).
					Validate(func(s string) error {
						s = strings.TrimSpace(s)
						if s == "" {
							return nil // empty = done
						}
						abs, err := filepath.Abs(expandTilde(s))
						if err != nil {
							return fmt.Errorf("invalid path: %w", err)
						}
						info, err := os.Stat(abs)
						if err != nil {
							return fmt.Errorf("%s: %w", abs, err)
						}
						if !info.IsDir() {
							return fmt.Errorf("%s is not a directory", abs)
						}
						return nil
					}),
			),
		)
		if err := input.Run(); err != nil {
			return nil, fmt.Errorf("installer cancelled: %w", err)
		}
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return paths, nil
		}
		abs, _ := filepath.Abs(expandTilde(raw))
		paths = append(paths, abs)
		fmt.Printf("  ✓ %s\n", abs)
	}
}

// expandTilde replaces a leading ~ with the user's home directory.
// Paths typed by hand commonly use ~ ; filepath.Abs does not expand it
// since the shell normally does that before exec.
func expandTilde(p string) string {
	if !strings.HasPrefix(p, "~") {
		return p
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	if p == "~" {
		return home
	}
	if strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[2:])
	}
	return p
}

func pluralY(n int) string {
	if n == 1 {
		return "y"
	}
	return "ies"
}

// isInteractive reports whether stdin is a real terminal. huh opens
// /dev/tty directly rather than reading stdin, so a simple stdin check
// isn't enough — we need to know whether a TTY is actually reachable.
// x/term.IsTerminal is the canonical answer, already pulled in as a
// transitive dep of huh.
func isInteractive() bool {
	return term.IsTerminal(os.Stdin.Fd()) && term.IsTerminal(os.Stdout.Fd())
}
