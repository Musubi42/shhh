package cmdinstall

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// configVersion is the on-disk schema version. Bump when an incompatible
// change is made; readers refuse older+newer versions to avoid partial
// parsing.
const configVersion = 1

// Config is the shhh user config persisted to disk after a successful
// install. It captures the minimum state the installer needs to
// (a) know what was installed so uninstall can find it, (b) pre-fill
// the prompts on re-install, and (c) tell `shhh audit` which projects
// to include in its audit scope.
//
// Obfuscation level is intentionally absent — v0.2 ships one level
// (spec Q3 resolved α).
type Config struct {
	Version          int      `json:"version"`
	Scope            string   `json:"scope"`             // "global" or "project"
	Agents           []string `json:"agents"`            // ["claude-code", ...]
	Paths            []string `json:"installed_paths"`   // absolute settings.json paths touched
	SelectedProjects []string `json:"selected_projects"` // per-agent project dash-names to audit; empty = all
	IgnoredPaths     []string `json:"ignored_paths,omitempty"` // absolute project paths to skip in `shhh audit`
	// Engines is the ordered list of detection engines the hook +
	// scan + audit should drive. Order matters: when two engines
	// flag the same span, the first one wins for label attribution.
	// Empty means "use the default" (currently ["gitleaks"]).
	Engines []string `json:"engines,omitempty"`
}

// EffectiveEngines returns Engines if set, else the default selection
// ([]string{"gitleaks"}). Always returns a non-empty slice so callers
// can drive `detector.NewFromConfig` without an empty check.
func (c *Config) EffectiveEngines() []string {
	if c == nil || len(c.Engines) == 0 {
		return []string{"gitleaks"}
	}
	out := make([]string, len(c.Engines))
	copy(out, c.Engines)
	return out
}

// ConfigPath returns the absolute path to the shhh user config file.
// Respects $SHHH_CONFIG_DIR (used by tests and by users who want a
// non-default location). Default: ~/.shhh/config.json.
func ConfigPath() (string, error) {
	if dir := os.Getenv("SHHH_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "config.json"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve home dir: %w", err)
	}
	return filepath.Join(home, ".shhh", "config.json"), nil
}

// LoadConfig reads the shhh config file. Returns (nil, nil) if the file
// doesn't exist — that's a fresh install, not an error. Returns an error
// only if the file exists but is unreadable or corrupt.
func LoadConfig() (*Config, error) {
	path, err := ConfigPath()
	if err != nil {
		return nil, err
	}
	buf, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	var c Config
	if err := json.Unmarshal(buf, &c); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if c.Version != configVersion {
		return nil, fmt.Errorf("shhh config at %s has version %d, this binary expects %d", path, c.Version, configVersion)
	}
	return &c, nil
}

// SaveConfig writes the config atomically. Mode 0600 because the path
// list reveals install locations.
func SaveConfig(c *Config) error {
	path, err := ConfigPath()
	if err != nil {
		return err
	}
	c.Version = configVersion
	// Normalize for determinism across runs.
	sort.Strings(c.Agents)
	sort.Strings(c.Paths)

	out, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	out = append(out, '\n')

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".config-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, 0o600); err != nil {
		os.Remove(name)
		return err
	}
	return os.Rename(name, path)
}

// AddInstalledPath appends a settings.json path to the config if not
// already present. Used during Plan.Execute to record what was touched.
func (c *Config) AddInstalledPath(p string) {
	for _, existing := range c.Paths {
		if existing == p {
			return
		}
	}
	c.Paths = append(c.Paths, p)
}

// RemoveInstalledPath removes a settings.json path from the config.
// If the path list becomes empty, the Scope is cleared too so a future
// audit doesn't see a half-emptied "global install with zero paths".
// Used by `shhh uninstall` so its on-disk state matches reality.
func (c *Config) RemoveInstalledPath(p string) {
	kept := c.Paths[:0]
	for _, existing := range c.Paths {
		if existing != p {
			kept = append(kept, existing)
		}
	}
	c.Paths = kept
	if len(c.Paths) == 0 {
		c.Scope = ""
	}
}

// AddIgnoredPath records an absolute project path the user wants
// `shhh audit` to skip. Idempotent. Sorted on save.
func (c *Config) AddIgnoredPath(p string) {
	for _, existing := range c.IgnoredPaths {
		if existing == p {
			return
		}
	}
	c.IgnoredPaths = append(c.IgnoredPaths, p)
	sort.Strings(c.IgnoredPaths)
}

// RemoveIgnoredPath drops p from the ignore list. Returns true if it
// was present (used by the CLI to print "removed" vs "not in list").
func (c *Config) RemoveIgnoredPath(p string) bool {
	kept := c.IgnoredPaths[:0]
	found := false
	for _, existing := range c.IgnoredPaths {
		if existing == p {
			found = true
			continue
		}
		kept = append(kept, existing)
	}
	c.IgnoredPaths = kept
	return found
}
