package cmdinstall

import (
	"fmt"
	"os"
	"path/filepath"
)

// RunInstall is the entry point for `shhh install [target]`. Zero args
// enters the interactive installer (the default UX for `npx shhh
// install`). A single target argument falls through to the scripted,
// non-interactive path (used by CI and by the existing milestone 1
// invocation).
func RunInstall(args []string) error {
	if len(args) == 0 {
		return RunInteractive()
	}
	target := args[0]
	switch target {
	case "claude-code":
		return installClaudeCode()
	default:
		return fmt.Errorf("install: unknown target %q (supported: claude-code, or omit for interactive)", target)
	}
}

// RunUninstall is the entry point for `shhh uninstall <target>`.
func RunUninstall(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("uninstall: missing target (try: shhh uninstall claude-code)")
	}
	target := args[0]
	switch target {
	case "claude-code":
		return uninstallClaudeCode()
	default:
		return fmt.Errorf("uninstall: unknown target %q (supported: claude-code)", target)
	}
}

func installClaudeCode() error {
	path, err := claudeSettingsPath()
	if err != nil {
		return err
	}
	binary, err := os.Executable()
	if err != nil {
		return fmt.Errorf("resolve shhh binary path: %w", err)
	}
	binary, _ = filepath.Abs(binary)

	d, err := Install(path, binary)
	if err != nil {
		return err
	}
	if d == "" {
		fmt.Printf("shhh: already installed in %s (no changes)\n", path)
		return nil
	}
	fmt.Printf("shhh: installed hook into %s\n\n%s", path, d)
	fmt.Println("\nRestart any running `claude` sessions for the hook to take effect.")
	return nil
}

func uninstallClaudeCode() error {
	path, err := claudeSettingsPath()
	if err != nil {
		return err
	}
	d, err := Uninstall(path)
	if err != nil {
		return err
	}
	if d == "" {
		fmt.Printf("shhh: not installed in %s (no changes)\n", path)
		return nil
	}
	fmt.Printf("shhh: removed hook from %s\n\n%s", path, d)
	return nil
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
