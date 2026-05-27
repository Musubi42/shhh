package cmdinstall

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
)

// debugReadBuildInfo is a package-level indirection over
// runtime/debug.ReadBuildInfo so tests can stub it (otherwise
// build info during `go test` lacks gitleaks as a "dep" and the
// fallback path is the only one exercised).
var debugReadBuildInfo = debug.ReadBuildInfo

// RunInstall is the entry point for `shhh install [target] [paths...] [flags]`.
//
// Zero args enters the interactive installer. With a target:
//
//	shhh install claude-code                                       # global, default engines
//	shhh install claude-code ~/work/billing                        # project scope, single repo
//	shhh install claude-code ~/a ~/b ~/c                           # project scope, N repos
//	shhh install claude-code --scope global                        # explicit (the default)
//	shhh install claude-code --engines gitleaks,shhh-native        # explicit engines
//	shhh install claude-code --cwd ~/x                             # back-compat alias for one positional path
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
	case "claude-code", "codex", "cursor":
		scope, paths, engines, err := parseInstallFlags("install", rest)
		if err != nil {
			return err
		}
		return installAgent(target, scope, paths, engines)
	default:
		return fmt.Errorf("install: unknown target %q (supported: claude-code, codex, cursor, or omit for interactive)", target)
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
	case "claude-code", "codex", "cursor":
		scope, paths, _, err := parseInstallFlags("uninstall", rest)
		if err != nil {
			return err
		}
		return uninstallAgent(target, scope, paths)
	default:
		return fmt.Errorf("uninstall: unknown target %q (supported: claude-code, codex, cursor)", target)
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
// any mix of positional paths and flags. Returns the resolved scope,
// the absolute project paths (empty slice for global scope), and the
// validated engine selection (nil when --engines was not passed; the
// caller treats that as "preserve existing config").
func parseInstallFlags(verb string, args []string) (Scope, []string, []string, error) {
	fs := flag.NewFlagSet(verb, flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	scopeStr := fs.String("scope", "", "install scope: global or project (default: project if path(s) given, else global)")
	cwdFlag := fs.String("cwd", "", "deprecated alias for a single positional project path")
	enginesFlag := fs.String("engines", "", "comma-separated engines to enable (e.g. gitleaks,shhh-native). Empty = keep existing or use default.")
	fs.Usage = func() {
		fmt.Fprintf(os.Stderr, `shhh %s claude-code [paths...] [--scope global|project] [--engines gitleaks,shhh-native]

With no path, installs globally (~/.claude/settings.json).
With one or more paths, installs per-project into <path>/.claude/settings.json.
Paths are resolved against $PWD; can be relative or absolute.
--engines accepts a CSV; valid values: gitleaks, shhh-native.
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
		return "", nil, nil, err
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
			return "", nil, nil, fmt.Errorf("--scope global is incompatible with positional project paths")
		}
	case ScopeProject:
		if len(paths) == 0 {
			cwd, err := os.Getwd()
			if err != nil {
				return "", nil, nil, fmt.Errorf("resolve cwd: %w", err)
			}
			paths = []string{cwd}
		}
	default:
		return "", nil, nil, fmt.Errorf("--scope must be 'global' or 'project' (got %q)", *scopeStr)
	}

	// Normalize project paths to absolute form so the install target is
	// reproducible regardless of where the user ran `shhh` from.
	for i, p := range paths {
		if err := validateProjectPath(p); err != nil {
			return "", nil, nil, err
		}
		abs, err := filepath.Abs(p)
		if err != nil {
			return "", nil, nil, fmt.Errorf("resolve path %q: %w", p, err)
		}
		paths[i] = abs
	}

	// Parse + validate --engines if supplied. Empty value = no override;
	// caller preserves existing config.
	var engines []string
	if *enginesFlag != "" {
		for _, raw := range strings.Split(*enginesFlag, ",") {
			name := strings.TrimSpace(raw)
			if name == "" {
				continue
			}
			switch name {
			case "gitleaks", "shhh-native":
				engines = append(engines, name)
			default:
				return "", nil, nil, fmt.Errorf("--engines: unknown engine %q (want: gitleaks, shhh-native)", name)
			}
		}
		if len(engines) == 0 {
			return "", nil, nil, fmt.Errorf("--engines: at least one engine required")
		}
	}

	return scope, paths, engines, nil
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

// installAgent installs the shhh hook for one or more targets, against
// the given agent ("claude-code" or "codex"). For scope=global, paths
// is ignored (a single canonical settings path is used). For
// scope=project, paths is the list of project roots to install into;
// each one gets a per-agent config dir (<root>/.claude/settings.json or
// <root>/.codex/hooks.json).
//
// engines is the validated engine selection; nil means "preserve
// whatever is already in ~/.shhh/config.json (or take the default on
// first install)".
func installAgent(agent string, scope Scope, paths []string, engines []string) error {
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
		settingsPath, err := AgentSettingsPath(agent, scope, target)
		if err != nil {
			return err
		}

		// For project scope, the agent's config dir often doesn't
		// exist yet (both Claude Code and Codex create it lazily on
		// first session). Create it so the user can pre-install shhh
		// in a fresh repo. Mode 0700 matches what the agents
		// themselves use.
		createdConfigDir := false
		if scope == ScopeProject {
			// Soft heuristic warning, not a block: if the target
			// directory looks empty or lacks any project marker
			// (.git, package.json, go.mod, pyproject.toml, Cargo.toml),
			// surface a note. Helps catch "wrong directory" mistakes
			// without refusing legitimate brand-new repos.
			if note := projectRootHint(target); note != "" {
				fmt.Fprintf(os.Stderr, "shhh: warning: %s\n", note)
			}
			configDir := filepath.Dir(settingsPath)
			if _, statErr := os.Stat(configDir); os.IsNotExist(statErr) {
				if mkErr := os.MkdirAll(configDir, 0o700); mkErr != nil {
					return fmt.Errorf("create %s: %w", configDir, mkErr)
				}
				createdConfigDir = true
			}
		}

		d, err := Install(settingsPath, binary, agent)
		if err != nil {
			return err
		}

		syncConfigOnInstall(agent, scope, settingsPath, engines)

		if agent == "claude-code" {
			if _, err := installShhhAllowCommand(settingsPath); err != nil {
				fmt.Fprintf(os.Stderr, "shhh: warning: could not write /shhh-allow command file: %v\n", err)
			}
		}

		if d == "" {
			fmt.Printf("shhh: already installed in %s (no changes)\n", settingsPath)
			continue
		}
		if createdConfigDir {
			fmt.Printf("shhh: created %s\n", filepath.Dir(settingsPath))
		}
		fmt.Printf("shhh: installed hook into %s\n\n%s", settingsPath, d)
		if scope == ScopeProject {
			fmt.Printf("\nThis is a per-project install. Commit %s to your repo so teammates inherit shhh automatically.\n\n", settingsPath)
		}
	}
	printInstallSummary(agent, engines)
	return nil
}

// printInstallSummary prints the post-install footer: active engines
// with attribution + helpful one-liners pointing at follow-up
// subcommands. The gitleaks block names the upstream MIT license and
// links to the pinned `gitleaks.toml` so users can audit the
// inherited path allowlist without leaving their terminal.
//
// agent affects two strings: the restart hint at the bottom ("Restart
// any running `claude` sessions" vs "`codex` sessions") and, for codex,
// an explicit one-paragraph caveat about the upstream
// apply_patch/read_file hook gap.
func printInstallSummary(agent string, planEngines []string) {
	// Engines actually persisted may differ from planEngines (nil
	// means "preserve existing"); reload to print the truth.
	cfg, _ := LoadConfig()
	active := []string{"gitleaks"}
	if cfg != nil && len(cfg.Engines) > 0 {
		active = cfg.Engines
	} else if len(planEngines) > 0 {
		active = planEngines
	}

	fmt.Println()
	fmt.Printf("Engines active: %s\n", strings.Join(active, ", "))
	for _, e := range active {
		switch e {
		case "gitleaks":
			ver := gitleaksVersion()
			fmt.Printf("  • gitleaks %s — MIT, https://github.com/gitleaks/gitleaks\n", ver)
			fmt.Printf("    Default ignore rules: https://github.com/gitleaks/gitleaks/blob/%s/config/gitleaks.toml\n", ver)
		case "shhh-native":
			fmt.Println("  • shhh-native — env cross-reference + URL-structural redaction (first-party)")
		}
	}
	fmt.Println()
	fmt.Println("  • Inspect active ignore rules:  shhh ignore list")
	fmt.Println("  • Third-party notices:          shhh licenses")
	fmt.Println()

	switch agent {
	case "codex":
		fmt.Println("Codex coverage note: shhh intercepts Bash today (cat .env, rg, etc.). Codex's")
		fmt.Println("apply_patch and read_file tools do not yet fire PreToolUse upstream — track")
		fmt.Println("https://github.com/openai/codex/issues/18491. Until that ships, in-place edits")
		fmt.Println("via apply_patch can hand the model a raw secret. See docs/dev/known-limitations.md.")
		fmt.Println()
		fmt.Println("Restart any running `codex` sessions for the hook to take effect.")
	case "cursor":
		fmt.Println("Cursor coverage note: shhh wires into Cursor's native hook system (v1.7+).")
		fmt.Println("Shell + Read are intercepted; the Read→Edit ledger interaction is unverified")
		fmt.Println("on Cursor — if Edit fails on a redacted file, use Shell (sed/tee/python) as on")
		fmt.Println("Claude Code. See docs/dev/known-limitations.md §3.")
		fmt.Println()
		fmt.Println("Restart Cursor (close all windows) for the hook to take effect.")
	default:
		fmt.Println("Restart any running `claude` sessions for the hook to take effect.")
	}
}

// gitleaksVersion returns the gitleaks module version embedded in
// this build (e.g. "v8.30.1"). Falls back to "v8" when the build
// info doesn't expose module versions (unusual but possible for
// some `go build` invocations).
func gitleaksVersion() string {
	info, ok := debugReadBuildInfo()
	if !ok {
		return "v8"
	}
	for _, dep := range info.Deps {
		if dep.Path == "github.com/zricethezav/gitleaks/v8" {
			return dep.Version
		}
	}
	return "v8"
}

// uninstallAgent is the symmetric multi-target uninstall.
func uninstallAgent(agent string, scope Scope, paths []string) error {
	targets := []string{""}
	if scope == ScopeProject {
		targets = paths
	}

	for _, target := range targets {
		settingsPath, err := AgentSettingsPath(agent, scope, target)
		if err != nil {
			return err
		}
		d, err := Uninstall(settingsPath, agent)
		if err != nil {
			return err
		}

		if agent == "claude-code" {
			if _, err := uninstallShhhAllowCommand(settingsPath); err != nil {
				fmt.Fprintf(os.Stderr, "shhh: warning: could not remove /shhh-allow command file: %v\n", err)
			}
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

// syncConfigOnInstall adds the path to installed_paths, updates scope
// (highest scope wins, global > project), and appends agent to Agents
// if not already present. engines is the explicit selection from
// --engines (or nil to preserve the existing config / fall through
// to default).
func syncConfigOnInstall(agent string, scope Scope, path string, engines []string) {
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
		cfg.Agents = []string{agent}
	} else {
		found := false
		for _, a := range cfg.Agents {
			if a == agent {
				found = true
				break
			}
		}
		if !found {
			cfg.Agents = append(cfg.Agents, agent)
		}
	}
	if len(engines) > 0 {
		cfg.Engines = append([]string{}, engines...)
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

// inferScopeFromPaths returns "global" if any path matches a known
// global agent settings location (Claude Code or Codex), "project" if
// at least one project-scoped path remains, or "" if the list is
// empty.
func inferScopeFromPaths(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	globals := make([]string, 0, 3)
	if p, err := claudeSettingsPath(); err == nil && p != "" {
		globals = append(globals, filepath.Clean(p))
	}
	if p, err := codexHooksPath(); err == nil && p != "" {
		globals = append(globals, filepath.Clean(p))
	}
	if p, err := cursorHooksPath(); err == nil && p != "" {
		globals = append(globals, filepath.Clean(p))
	}
	for _, p := range paths {
		clean := filepath.Clean(p)
		for _, g := range globals {
			if clean == g {
				return "global"
			}
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

// AgentSettingsPath resolves the settings/hooks file path for a given
// agent and scope. For project scope, cwd is the directory the user ran
// `shhh install` from — its agent-specific config dir is the target.
// For global scope, cwd is ignored.
//
// Returns an error for unknown agents so the interactive installer can
// report them cleanly instead of silently defaulting.
//
// Per-agent layout:
//   - claude-code → ~/.claude/settings.json | <cwd>/.claude/settings.json
//   - codex       → ~/.codex/hooks.json     | <cwd>/.codex/hooks.json
//
// Codex's primary config is ~/.codex/config.toml; we use a dedicated
// ~/.codex/hooks.json instead because (a) shhh's install/uninstall
// logic is JSON-native, (b) Codex's hook discovery accepts hooks.json
// out of the box per developers.openai.com/codex/hooks, and (c) it
// keeps shhh's footprint isolated from the user's TOML config so
// uninstall is clean.
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
	case "codex":
		switch scope {
		case ScopeGlobal:
			return codexHooksPath()
		case ScopeProject:
			if cwd == "" {
				return "", fmt.Errorf("project scope requires a working directory")
			}
			abs, err := filepath.Abs(cwd)
			if err != nil {
				return "", fmt.Errorf("resolve cwd: %w", err)
			}
			return filepath.Join(abs, ".codex", "hooks.json"), nil
		default:
			return "", fmt.Errorf("unknown scope %q", scope)
		}
	case "cursor":
		switch scope {
		case ScopeGlobal:
			return cursorHooksPath()
		case ScopeProject:
			if cwd == "" {
				return "", fmt.Errorf("project scope requires a working directory")
			}
			abs, err := filepath.Abs(cwd)
			if err != nil {
				return "", fmt.Errorf("resolve cwd: %w", err)
			}
			return filepath.Join(abs, ".cursor", "hooks.json"), nil
		default:
			return "", fmt.Errorf("unknown scope %q", scope)
		}
	default:
		return "", fmt.Errorf("unknown agent %q", agent)
	}
}

// cursorHooksPath returns the global Cursor hooks file at
// ~/.cursor/hooks.json. Cursor IDE has no documented env override
// equivalent to $CLAUDE_CONFIG_DIR / $CODEX_HOME; if one ships
// later, plumb it here.
func cursorHooksPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".cursor", "hooks.json"), nil
}

// codexHooksPath returns the global Codex hooks file. Respects
// $CODEX_HOME if set (Codex's own env override).
func codexHooksPath() (string, error) {
	if dir := os.Getenv("CODEX_HOME"); dir != "" {
		return filepath.Join(dir, "hooks.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".codex", "hooks.json"), nil
}

// DetectInstalledAgents inspects well-known config locations and returns
// the agents that appear to be installed on this machine. Used by the
// interactive installer to pre-select options and hide unsupported
// agents. Detection is best-effort: if a config directory is present we
// assume the agent is, even if the user has never actually launched it.
func DetectInstalledAgents() []string {
	var out []string

	// Claude Code: $CLAUDE_CONFIG_DIR or ~/.claude.
	{
		dir := os.Getenv("CLAUDE_CONFIG_DIR")
		if dir == "" {
			if home, herr := os.UserHomeDir(); herr == nil {
				dir = filepath.Join(home, ".claude")
			}
		}
		if dir != "" {
			if st, err := os.Stat(dir); err == nil && st.IsDir() {
				out = append(out, "claude-code")
			}
		}
	}

	// Codex: $CODEX_HOME or ~/.codex.
	{
		dir := os.Getenv("CODEX_HOME")
		if dir == "" {
			if home, herr := os.UserHomeDir(); herr == nil {
				dir = filepath.Join(home, ".codex")
			}
		}
		if dir != "" {
			if st, err := os.Stat(dir); err == nil && st.IsDir() {
				out = append(out, "codex")
			}
		}
	}

	// Cursor: ~/.cursor (Cursor IDE's hook + MCP config dir).
	// macOS GUI Cursor also writes ~/Library/Application Support/Cursor
	// for general state, but the hook config lives under the dotfile
	// dir per developers.cursor.com/docs/hooks.
	{
		if home, herr := os.UserHomeDir(); herr == nil {
			dir := filepath.Join(home, ".cursor")
			if st, err := os.Stat(dir); err == nil && st.IsDir() {
				out = append(out, "cursor")
			}
		}
	}
	return out
}
