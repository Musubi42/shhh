package cmdinstall

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// RunInstall is the entry point for `shhh install [target] [paths...] [flags]`.
//
// Zero args enters the interactive installer. With a target:
//
//	shhh install claude-code                       # global
//	shhh install claude-code ~/work/billing        # project scope, single repo
//	shhh install claude-code ~/a ~/b ~/c           # project scope, N repos
//	shhh install claude-code --scope global        # explicit (the default)
//	shhh install claude-code --cwd ~/x             # back-compat alias for one positional path
//
// Positional paths imply --scope project. Passing both --scope global
// and positional paths is an error (the intent is ambiguous).
func RunInstall(args []string) error {
	if len(args) == 0 {
		return RunInteractive()
	}
	target := args[0]
	rest := args[1:]
	switch target {
	case "claude-code":
		scope, paths, err := parseInstallFlags("install", rest)
		if err != nil {
			return err
		}
		return installClaudeCode(scope, paths)
	default:
		return fmt.Errorf("install: unknown target %q (supported: claude-code, or omit for interactive)", target)
	}
}

// RunUninstall is the entry point for `shhh uninstall <target> [paths...] [flags]`.
// Accepts the same shape as install. Defaults to global when no path is given.
func RunUninstall(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("uninstall: missing target (try: shhh uninstall claude-code)")
	}
	target := args[0]
	rest := args[1:]
	switch target {
	case "claude-code":
		scope, paths, err := parseInstallFlags("uninstall", rest)
		if err != nil {
			return err
		}
		return uninstallClaudeCode(scope, paths)
	default:
		return fmt.Errorf("uninstall: unknown target %q (supported: claude-code)", target)
	}
}

// projectRootHint returns a one-line warning when target lacks any
// common project marker. Empty string means "looks like a project,
// no warning needed." Soft heuristic; the install proceeds either
// way.
func projectRootHint(target string) string {
	markers := []string{".git", "package.json", "go.mod", "pyproject.toml", "Cargo.toml", "pom.xml", "build.gradle", "Gemfile", "composer.json"}
	for _, m := range markers {
		if _, err := os.Stat(filepath.Join(target, m)); err == nil {
			return ""
		}
	}
	// No marker found. Distinguish "directory is empty/new" from
	// "directory has files but none are project markers" — the
	// former is fine (fresh repo init), the latter is suspicious.
	entries, err := os.ReadDir(target)
	if err != nil || len(entries) == 0 {
		return ""
	}
	return fmt.Sprintf("%s has no .git/package.json/go.mod/… — make sure this is the project root you meant", target)
}

// splitArgsByFlags pre-splits args into flag tokens and positional
// tokens so flag.Parse can be invoked on a flag-only slice. Unlike
// the simple bool-only variant in cmdaudit, this one consults the
// FlagSet to know when a flag is followed by a value token: a
// non-boolean flag (`--scope project`) consumes the next arg as its
// value rather than treating it as a positional.
//
// Recognized forms:
//   - `--flag` / `-flag` (bool)
//   - `--flag=value` / `-flag=value` (value embedded)
//   - `--flag value` / `-flag value` (next arg is value, only for non-bool flags)
//
// Anything else is positional. An unknown flag is treated as a bool
// (passed through to fs.Parse, which will raise the proper error).
func splitArgsByFlags(args []string, fs *flag.FlagSet) (flagArgs, positional []string) {
	i := 0
	for i < len(args) {
		a := args[i]
		if !strings.HasPrefix(a, "-") {
			positional = append(positional, a)
			i++
			continue
		}
		flagArgs = append(flagArgs, a)
		if strings.Contains(a, "=") {
			i++
			continue
		}
		name := strings.TrimLeft(a, "-")
		f := fs.Lookup(name)
		if f == nil {
			i++
			continue
		}
		if bf, ok := f.Value.(interface{ IsBoolFlag() bool }); ok && bf.IsBoolFlag() {
			i++
			continue
		}
		// Non-bool: take the next arg as the flag's value, if present.
		if i+1 < len(args) {
			flagArgs = append(flagArgs, args[i+1])
			i += 2
			continue
		}
		i++
	}
	return
}

// parseInstallFlags is shared between install and uninstall. It accepts
// any mix of positional paths and flags. Returns the resolved scope and
// the absolute project paths (empty slice for global scope).
func parseInstallFlags(verb string, args []string) (Scope, []string, error) {
	fs := flag.NewFlagSet(verb, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	scopeStr := fs.String("scope", "", "install scope: global or project (default: project if path(s) given, else global)")
	cwdFlag := fs.String("cwd", "", "deprecated alias for a single positional project path")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `shhh %s claude-code [paths...] [--scope global|project]

With no path, installs globally (~/.claude/settings.json).
With one or more paths, installs per-project into <path>/.claude/settings.json.
Paths are resolved against $PWD; can be relative or absolute.
`, verb)
		fs.PrintDefaults()
	}
	// Pre-split so flags work in any position: `shhh install
	// claude-code ~/repo --scope project` must respect --scope, but
	// Go's flag.Parse stops at the first non-flag argument. The
	// splitter consults the FlagSet to know which flags consume the
	// next token as a value (--scope, --cwd) vs which are bool. Same
	// bug class as cmdaudit's `splitFlagsAndPositionals` (fixed
	// 2026-05-26 dryrun); cmdinstall needs the value-aware variant.
	flagArgs, positional := splitArgsByFlags(args, fs)
	if err := fs.Parse(flagArgs); err != nil {
		return "", nil, err
	}

	// Collect positional paths and the --cwd alias.
	paths := append([]string{}, positional...)
	if *cwdFlag != "" {
		paths = append(paths, *cwdFlag)
	}

	// Resolve scope from explicit flag + presence of paths.
	scope := Scope(*scopeStr)
	switch scope {
	case "":
		if len(paths) > 0 {
			scope = ScopeProject
		} else {
			scope = ScopeGlobal
		}
	case ScopeGlobal:
		if len(paths) > 0 {
			return "", nil, fmt.Errorf("--scope global is incompatible with positional project paths")
		}
	case ScopeProject:
		if len(paths) == 0 {
			cwd, err := os.Getwd()
			if err != nil {
				return "", nil, fmt.Errorf("resolve cwd: %w", err)
			}
			paths = []string{cwd}
		}
	default:
		return "", nil, fmt.Errorf("--scope must be 'global' or 'project' (got %q)", *scopeStr)
	}

	// Normalize project paths to absolute form so the install target is
	// reproducible regardless of where the user ran `shhh` from.
	for i, p := range paths {
		if err := validateProjectPath(p); err != nil {
			return "", nil, err
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			return "", nil, fmt.Errorf("resolve path %q: %w", p, err)
		}
		paths[i] = abs
	}
	return scope, paths, nil
}

// validateProjectPath rejects paths that obviously aren't project
// roots, to stop accidental damage from typos. Lessons from the
// 2026-05-26 dryrun:
//   - A `--scope` token that slipped past flag parsing was happily
//     turned into a path and a `./--scope/.claude/` directory was
//     created. Reject `-`-prefixed inputs even when the flag parser
//     would otherwise let them through.
//   - A user typing `shhh install claude-code .claude` (typo for
//     `--scope project .`) would create `.claude/.claude/`. Reject
//     paths whose last segment is `.claude`.
//   - `..` in the middle of a path is legal but usually a sign that
//     the user pasted something half-typed. Reject and ask them to
//     resolve before passing.
//
// Validation runs BEFORE filepath.Abs so the error message echoes
// what the user actually typed.
func validateProjectPath(p string) error {
	if p == "" {
		return fmt.Errorf("install: empty project path")
	}
	if strings.HasPrefix(p, "-") {
		return fmt.Errorf("install: project path %q starts with '-' — looks like a misplaced flag", p)
	}
	base := filepath.Base(filepath.Clean(p))
	if base == ".claude" {
		return fmt.Errorf("install: project path %q ends in .claude — pass the project root, not its config dir", p)
	}
	parts := strings.Split(filepath.ToSlash(p), "/")
	for _, seg := range parts {
		if seg == ".." {
			return fmt.Errorf("install: project path %q contains '..' — resolve to an absolute path first", p)
		}
	}
	return nil
}

// installClaudeCode installs the hook for one or more targets. For
// scope=global, paths is ignored (a single canonical settings path is
// used). For scope=project, paths is the list of project roots to
// install into; each one gets <root>/.claude/settings.json.
func installClaudeCode(scope Scope, paths []string) error {
	binary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve shhh binary path: %w", err)
	}
	binary, _ = filepath.Abs(binary)

	targets := []string{""}
	if scope == ScopeProject {
		targets = paths
	}

	for _, target := range targets {
		settingsPath, err := AgentSettingsPath("claude-code", scope, target)
		if err != nil {
			return err
		}

		// For project scope, the `.claude/` directory often doesn't
		// exist yet (Claude Code creates it lazily on first session).
		// Create it so the user can pre-install shhh in a fresh repo.
		// Mode 0700 matches what Claude Code itself uses.
		createdClaudeDir := false
		if scope == ScopeProject {
			// Soft heuristic warning, not a block: if the target
			// directory looks empty or lacks any project marker
			// (.git, package.json, go.mod, pyproject.toml, Cargo.toml),
			// surface a note. Helps catch "wrong directory" mistakes
			// without refusing legitimate brand-new repos.
			if note := projectRootHint(target); note != "" {
				fmt.Fprintf(os.Stderr, "shhh: warning: %s\n", note)
			}
			claudeDir := filepath.Dir(settingsPath)
			if _, statErr := os.Stat(claudeDir); os.IsNotExist(statErr) {
				if mkErr := os.MkdirAll(claudeDir, 0o700); mkErr != nil {
					return fmt.Errorf("create %s: %w", claudeDir, mkErr)
				}
				createdClaudeDir = true
			}
		}

		d, err := Install(settingsPath, binary)
		if err != nil {
			return err
		}

		syncConfigOnInstall(scope, settingsPath)

		if d == "" {
			fmt.Printf("shhh: already installed in %s (no changes)\n", settingsPath)
			continue
		}
		if createdClaudeDir {
			fmt.Printf("shhh: created %s\n", filepath.Dir(settingsPath))
		}
		fmt.Printf("shhh: installed hook into %s\n\n%s", settingsPath, d)
		if scope == ScopeProject {
			fmt.Printf("\nThis is a per-project install. Commit %s to your repo so teammates inherit shhh automatically.\n\n", settingsPath)
		}
	}
	fmt.Println("Restart any running `claude` sessions for the hook to take effect.")
	return nil
}

// uninstallClaudeCode is the symmetric multi-target uninstall.
func uninstallClaudeCode(scope Scope, paths []string) error {
	targets := []string{""}
	if scope == ScopeProject {
		targets = paths
	}

	for _, target := range targets {
		settingsPath, err := AgentSettingsPath("claude-code", scope, target)
		if err != nil {
			return err
		}
		d, err := Uninstall(settingsPath)
		if err != nil {
			return err
		}

		// Always sync our own config.json — the on-disk state needs
		// to match reality even if the settings.json hadn't been
		// touched in a while. Without this, `shhh audit` would
		// happily report the project as PROTECTED based on stale
		// config (real bug observed during the v0.1 dryrun on
		// 2026-05-25).
		syncConfigOnUninstall(settingsPath)

		// Note: we intentionally do NOT remove an empty .claude/
		// directory even if shhh was the only thing in it. Claude
		// Code itself may create the directory and use it for
		// unrelated state in the future, and an empty .claude/ left
		// behind is harmless. The settings.json is similarly left in
		// place if it still has other hook entries; Uninstall() above
		// removes only shhh's entries.

		if d == "" {
			fmt.Printf("shhh: not installed in %s (no changes)\n", settingsPath)
			continue
		}
		fmt.Printf("shhh: removed hook from %s\n\n%s", settingsPath, d)
	}
	return nil
}

// syncConfigOnInstall adds the path to installed_paths and updates
// scope using the "highest scope wins" rule (global > project).
func syncConfigOnInstall(scope Scope, path string) {
	cfg, _ := LoadConfig()
	if cfg == nil {
		cfg = &Config{}
	}
	cfg.AddInstalledPath(path)
	// Promote scope only when going to global. Project scope doesn't
	// downgrade an existing global.
	if scope == ScopeGlobal || cfg.Scope == "" {
		cfg.Scope = string(scope)
	}
	if cfg.Agents == nil {
		cfg.Agents = []string{"claude-code"}
	} else {
		// Add claude-code if not present
		found := false
		for _, a := range cfg.Agents {
			if a == "claude-code" {
				found = true
				break
			}
		}
		if !found {
			cfg.Agents = append(cfg.Agents, "claude-code")
		}
	}
	if err := SaveConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "shhh: warning: config update failed: %v\n", err)
	}
}

// syncConfigOnUninstall removes the path and re-derives scope from
// what remains. If no paths remain, scope is cleared.
func syncConfigOnUninstall(path string) {
	cfg, _ := LoadConfig()
	if cfg == nil {
		return
	}
	cfg.RemoveInstalledPath(path)
	// Re-derive scope from remaining paths: any global path wins.
	cfg.Scope = inferScopeFromPaths(cfg.Paths)
	if err := SaveConfig(cfg); err != nil {
		fmt.Fprintf(os.Stderr, "shhh: warning: config update failed: %v\n", err)
	}
}

// inferScopeFromPaths returns "global" if any path matches the global
// claude settings location, "project" if at least one project-scoped
// path remains, or "" if the list is empty.
func inferScopeFromPaths(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	global, _ := claudeSettingsPath()
	for _, p := range paths {
		if global != "" && filepath.Clean(p) == filepath.Clean(global) {
			return "global"
		}
	}
	return "project"
}

// claudeSettingsPath returns the path to the user's global Claude Code
// settings. Respects $CLAUDE_CONFIG_DIR if set (used for testing and for
// users with non-default Claude Code installs).
func claudeSettingsPath() (string, error) {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "settings.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".claude", "settings.json"), nil
}

// Scope is the install-target location for a single agent. Global writes
// into the user's home agent config; project writes into a .claude/ (or
// equivalent) directory inside a working directory.
type Scope string

const (
	ScopeGlobal  Scope = "global"
	ScopeProject Scope = "project"
)

// AgentSettingsPath resolves the settings.json path for a given agent and
// scope. For project scope, cwd is the directory the user ran `shhh
// install` from — its `.claude/settings.json` (or equivalent) is the
// target. For global scope, cwd is ignored.
//
// Returns an error for unknown agents so the interactive installer can
// report them cleanly instead of silently defaulting.
func AgentSettingsPath(agent string, scope Scope, cwd string) (string, error) {
	switch agent {
	case "claude-code":
		switch scope {
		case ScopeGlobal:
			return claudeSettingsPath()
		case ScopeProject:
			if cwd == "" {
				return "", fmt.Errorf("project scope requires a working directory")
			}
			abs, err := filepath.Abs(cwd)
			if err != nil {
				return "", fmt.Errorf("resolve cwd: %w", err)
			}
			return filepath.Join(abs, ".claude", "settings.json"), nil
		default:
			return "", fmt.Errorf("unknown scope %q", scope)
		}
	default:
		return "", fmt.Errorf("unknown agent %q", agent)
	}
}

// DetectInstalledAgents inspects well-known config locations and returns
// the agents that appear to be installed on this machine. Used by the
// interactive installer to pre-select options and hide unsupported
// agents. Detection is best-effort: if a config directory is present we
// assume the agent is, even if the user has never actually launched it.
func DetectInstalledAgents() []string {
	var out []string
	// Claude Code: $CLAUDE_CONFIG_DIR or ~/.claude.
	if _, err := claudeSettingsPath(); err == nil {
		dir := os.Getenv("CLAUDE_CONFIG_DIR")
		if dir == "" {
			home, herr := os.UserHomeDir()
			if herr == nil {
				dir = filepath.Join(home, ".claude")
			}
		}
		if dir != "" {
			if st, err := os.Stat(dir); err == nil && st.IsDir() {
				out = append(out, "claude-code")
			}
		}
	}
	// Codex, Cursor: not yet supported.
	return out
}
