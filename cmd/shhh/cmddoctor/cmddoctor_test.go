package cmddoctor

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Musubi42/shhh/cmd/shhh/cmdinstall"
)

// hermeticEnv mirrors cmdinstall's: redirects CLAUDE_CONFIG_DIR and
// SHHH_CONFIG_DIR at temp dirs so the doctor sees a controlled world.
func hermeticEnv(t *testing.T) (claudeDir, shhhDir string) {
	t.Helper()
	root := t.TempDir()
	claudeDir = filepath.Join(root, "claude")
	shhhDir = filepath.Join(root, "shhh")
	for _, d := range []string{claudeDir, shhhDir} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("CLAUDE_CONFIG_DIR", claudeDir)
	t.Setenv("SHHH_CONFIG_DIR", shhhDir)
	return
}

func TestClassifyInstallPath(t *testing.T) {
	tmp := t.TempDir()
	withHook := filepath.Join(tmp, "with-hook.json")
	if err := os.WriteFile(withHook, []byte(`{"hooks":{"PreToolUse":[{"hooks":[{"command":"/x/shhh hook claude-code"}]}]}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	noHook := filepath.Join(tmp, "no-hook.json")
	if err := os.WriteFile(noHook, []byte(`{"hooks":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name string
		path string
		want string
	}{
		{"with hook", withHook, ""},
		{"file exists no hook", noHook, "no-hook"},
		{"missing", filepath.Join(tmp, "ghost.json"), "missing"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := classifyInstallPath(tc.path); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// stubLookPathToRunning makes the binary-check pass by pretending
// the running test binary IS what's on $PATH. Lets us assert on the
// other checks without binary noise.
func stubLookPathToRunning(t *testing.T) {
	t.Helper()
	prev := lookPath
	t.Cleanup(func() { lookPath = prev })
	lookPath = func(string) (string, error) {
		return os.Executable()
	}
}

func TestRunChecksHealthy(t *testing.T) {
	stubLookPathToRunning(t)
	claudeDir, _ := hermeticEnv(t)
	// Plant a valid global settings.json with a shhh hook.
	settings := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settings,
		[]byte(`{"hooks":{"PreToolUse":[{"hooks":[{"command":"/x/shhh hook claude-code"}]}]}}`),
		0o600); err != nil {
		t.Fatal(err)
	}
	// Plant a matching config.
	cfg := &cmdinstall.Config{
		Scope:   "global",
		Agents:  []string{"claude-code"},
		Paths:   []string{settings},
	}
	if err := cmdinstall.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}

	r := runChecks()
	if r.staleConfigEntries != 0 {
		t.Errorf("expected 0 stale entries, got %d", r.staleConfigEntries)
	}
	// Render to a buffer and look for the verified message.
	var buf bytes.Buffer
	r.print(&buf, ansi{})
	if !strings.Contains(buf.String(), "1 install verified") {
		t.Errorf("expected verified message, got:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "All checks passed") {
		t.Errorf("expected 'All checks passed' for clean state:\n%s", buf.String())
	}
}

func TestRunChecksStaleEntry(t *testing.T) {
	claudeDir, _ := hermeticEnv(t)
	// Settings.json exists but has NO shhh hook (the F3 dryrun bug).
	settings := filepath.Join(claudeDir, "settings.json")
	if err := os.WriteFile(settings, []byte(`{"hooks":{}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg := &cmdinstall.Config{
		Scope:   "global",
		Agents:  []string{"claude-code"},
		Paths:   []string{settings},
	}
	if err := cmdinstall.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}

	r := runChecks()
	if r.staleConfigEntries != 1 {
		t.Errorf("expected 1 stale entry, got %d", r.staleConfigEntries)
	}

	// Heal it.
	dropped, err := healStaleEntries(r)
	if err != nil {
		t.Fatal(err)
	}
	if dropped != 1 {
		t.Errorf("dropped %d, want 1", dropped)
	}
	// Verify config now has empty paths and empty scope.
	cfg2, _ := cmdinstall.LoadConfig()
	if cfg2 == nil || len(cfg2.Paths) != 0 {
		t.Errorf("paths not pruned: %+v", cfg2)
	}
	if cfg2.Scope != "" {
		t.Errorf("scope not cleared after dropping last entry: %q", cfg2.Scope)
	}
}

func TestRunChecksMissingFile(t *testing.T) {
	claudeDir, _ := hermeticEnv(t)
	// Config references a settings.json that no longer exists.
	cfg := &cmdinstall.Config{
		Scope:   "project",
		Agents:  []string{"claude-code"},
		Paths:   []string{filepath.Join(claudeDir, "ghost", "settings.json")},
	}
	if err := cmdinstall.SaveConfig(cfg); err != nil {
		t.Fatal(err)
	}
	r := runChecks()
	if r.staleConfigEntries != 1 {
		t.Errorf("expected 1 stale entry for missing file, got %d", r.staleConfigEntries)
	}
	var buf bytes.Buffer
	r.print(&buf, ansi{})
	if !strings.Contains(buf.String(), "does not exist") {
		t.Errorf("expected 'does not exist' note, got:\n%s", buf.String())
	}
}

func TestDeriveScopeAfterHeal(t *testing.T) {
	home, _ := os.UserHomeDir()
	globalSettings := filepath.Join(home, ".claude", "settings.json")
	cases := []struct {
		name  string
		paths []string
		want  string
	}{
		{"empty", nil, ""},
		{"only global", []string{globalSettings}, "global"},
		{"only project", []string{"/tmp/x/.claude/settings.json"}, "project"},
		{"global + project", []string{globalSettings, "/tmp/x/.claude/settings.json"}, "global"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := deriveScope(tc.paths); got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}

// readRawConfig is a tiny test helper for cases where we want to
// inspect the on-disk JSON shape directly rather than go through
// LoadConfig. Used in failure-mode tests where Load might choke.
func readRawConfig(t *testing.T, path string) map[string]any {
	t.Helper()
	buf, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read raw config: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(buf, &m); err != nil {
		t.Fatalf("parse raw config: %v", err)
	}
	return m
}

var _ = readRawConfig // keep helper available for future tests
