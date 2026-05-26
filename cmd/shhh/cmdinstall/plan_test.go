package cmdinstall

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// hermeticEnv sets every env var that the installer and the config
// module read, so a test fully controls what the filesystem looks like
// to the subject code. Returns nothing; t.Setenv reverts after the test.
func hermeticEnv(t *testing.T) (claudeDir, shhhDir, cwd string) {
	t.Helper()
	root := t.TempDir()
	claudeDir = filepath.Join(root, "claude")
	shhhDir = filepath.Join(root, "shhh")
	cwd = filepath.Join(root, "project")
	for _, d := range []string{claudeDir, shhhDir, cwd} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("CLAUDE_CONFIG_DIR", claudeDir)
	t.Setenv("SHHH_CONFIG_DIR", shhhDir)
	return
}

func TestPlanValidate(t *testing.T) {
	cases := []struct {
		name    string
		plan    Plan
		wantErr string
	}{
		{
			name:    "empty",
			plan:    Plan{},
			wantErr: "invalid scope",
		},
		{
			name:    "unknown scope",
			plan:    Plan{Scope: "nope", Agents: []string{"claude-code"}},
			wantErr: "invalid scope",
		},
		{
			name:    "no agents",
			plan:    Plan{Scope: ScopeGlobal},
			wantErr: "no agents",
		},
		{
			name:    "unsupported agent",
			plan:    Plan{Scope: ScopeGlobal, Agents: []string{"windsurf"}},
			wantErr: "not supported",
		},
		{
			name:    "project without cwd",
			plan:    Plan{Scope: ScopeProject, Agents: []string{"claude-code"}},
			wantErr: "requires cwd",
		},
		{
			name: "valid global",
			plan: Plan{Scope: ScopeGlobal, Agents: []string{"claude-code"}},
		},
		{
			name: "valid project",
			plan: Plan{Scope: ScopeProject, Agents: []string{"claude-code"}, Cwd: "/tmp/x"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := tc.plan.Validate()
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q", tc.wantErr)
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestPlanExecuteGlobalInstall(t *testing.T) {
	claudeDir, shhhDir, _ := hermeticEnv(t)

	plan := &Plan{
		Scope:  ScopeGlobal,
		Agents: []string{"claude-code"},
	}
	var out bytes.Buffer
	if err := plan.Execute("/opt/shhh/bin/shhh", &out); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	// Claude settings.json should now contain our hook entry.
	buf, err := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	if err != nil {
		t.Fatalf("read settings.json: %v", err)
	}
	if !strings.Contains(string(buf), "/opt/shhh/bin/shhh hook claude-code") {
		t.Errorf("hook entry missing from settings.json:\n%s", buf)
	}

	// shhh config.json should be written with the expected shape.
	cfgBuf, err := os.ReadFile(filepath.Join(shhhDir, "config.json"))
	if err != nil {
		t.Fatalf("read shhh config.json: %v", err)
	}
	var cfg Config
	if err := json.Unmarshal(cfgBuf, &cfg); err != nil {
		t.Fatalf("parse shhh config: %v", err)
	}
	if cfg.Version != configVersion {
		t.Errorf("version = %d, want %d", cfg.Version, configVersion)
	}
	if cfg.Scope != string(ScopeGlobal) {
		t.Errorf("scope = %q, want %q", cfg.Scope, ScopeGlobal)
	}
	if len(cfg.Agents) != 1 || cfg.Agents[0] != "claude-code" {
		t.Errorf("agents = %v, want [claude-code]", cfg.Agents)
	}
	if len(cfg.Paths) != 1 || !strings.HasPrefix(cfg.Paths[0], claudeDir) {
		t.Errorf("installed_paths = %v, want one path under %q", cfg.Paths, claudeDir)
	}
}

func TestPlanExecuteProjectInstall(t *testing.T) {
	_, shhhDir, cwd := hermeticEnv(t)

	plan := &Plan{
		Scope:  ScopeProject,
		Agents: []string{"claude-code"},
		Cwd:    cwd,
	}
	var out bytes.Buffer
	if err := plan.Execute("/opt/shhh/bin/shhh", &out); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	projectSettings := filepath.Join(cwd, ".claude", "settings.json")
	buf, err := os.ReadFile(projectSettings)
	if err != nil {
		t.Fatalf("project settings.json missing: %v", err)
	}
	if !strings.Contains(string(buf), "hook claude-code") {
		t.Errorf("project settings.json missing hook entry:\n%s", buf)
	}

	// Global claude settings must NOT have been touched for a project install.
	if _, err := os.Stat(filepath.Join(os.Getenv("CLAUDE_CONFIG_DIR"), "settings.json")); err == nil {
		t.Error("project install must not write to the global Claude settings")
	}

	// shhh config reflects project scope.
	cfgBuf, _ := os.ReadFile(filepath.Join(shhhDir, "config.json"))
	var cfg Config
	if err := json.Unmarshal(cfgBuf, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Scope != string(ScopeProject) {
		t.Errorf("scope = %q, want project", cfg.Scope)
	}
	if len(cfg.Paths) != 1 || cfg.Paths[0] != projectSettings {
		t.Errorf("installed_paths = %v, want [%s]", cfg.Paths, projectSettings)
	}
}

func TestPlanExecuteIdempotent(t *testing.T) {
	hermeticEnv(t)
	plan := &Plan{
		Scope:  ScopeGlobal,
		Agents: []string{"claude-code"},
	}
	var out bytes.Buffer
	if err := plan.Execute("/opt/shhh/bin/shhh", &out); err != nil {
		t.Fatal(err)
	}
	// Second run must not error, must report "already configured."
	out.Reset()
	if err := plan.Execute("/opt/shhh/bin/shhh", &out); err != nil {
		t.Fatalf("second Execute: %v", err)
	}
	if !strings.Contains(out.String(), "already configured") {
		t.Errorf("expected 'already configured' on re-run, got:\n%s", out.String())
	}
}

func TestPlanExecuteRunsScanHook(t *testing.T) {
	hermeticEnv(t)
	called := 0
	var gotTarget string
	orig := RunScanHook
	defer func() { RunScanHook = orig }()
	RunScanHook = func(_ io.Writer, target string) error {
		called++
		gotTarget = target
		return nil
	}

	plan := &Plan{
		Scope:   ScopeGlobal,
		Agents:  []string{"claude-code"},
		RunScan: true,
	}
	var out bytes.Buffer
	if err := plan.Execute("/opt/shhh/bin/shhh", &out); err != nil {
		t.Fatal(err)
	}
	if called != 1 {
		t.Errorf("RunScanHook called %d times, want 1", called)
	}
	if gotTarget == "" {
		t.Errorf("scan target should be non-empty")
	}
}

func TestPlanExecuteRunScanOffPrintsFooter(t *testing.T) {
	hermeticEnv(t)
	plan := &Plan{
		Scope:   ScopeGlobal,
		Agents:  []string{"claude-code"},
		RunScan: false,
	}
	var out bytes.Buffer
	if err := plan.Execute("/opt/shhh/bin/shhh", &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "shhh scan") {
		t.Errorf("expected footer to suggest `shhh scan`, got:\n%s", out.String())
	}
}

func TestConfigRoundTrip(t *testing.T) {
	hermeticEnv(t)
	c := &Config{
		Scope:  string(ScopeProject),
		Agents: []string{"claude-code", "claude-code"}, // dupe
		Paths:  []string{"/a", "/b"},
	}
	if err := SaveConfig(c); err != nil {
		t.Fatal(err)
	}
	got, err := LoadConfig()
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("LoadConfig returned nil after save")
	}
	if got.Version != configVersion {
		t.Errorf("version lost: %d", got.Version)
	}
	if got.Scope != string(ScopeProject) {
		t.Errorf("scope = %q", got.Scope)
	}
	if len(got.Paths) != 2 {
		t.Errorf("paths = %v", got.Paths)
	}
}

func TestLoadConfigMissingIsNotAnError(t *testing.T) {
	hermeticEnv(t)
	got, err := LoadConfig()
	if err != nil {
		t.Errorf("missing config should not be an error: %v", err)
	}
	if got != nil {
		t.Errorf("missing config should return nil, got %+v", got)
	}
}

func TestAgentSettingsPathGlobal(t *testing.T) {
	hermeticEnv(t)
	p, err := AgentSettingsPath("claude-code", ScopeGlobal, "")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(p, "settings.json") {
		t.Errorf("global path = %q", p)
	}
}

func TestAgentSettingsPathProject(t *testing.T) {
	_, _, cwd := hermeticEnv(t)
	p, err := AgentSettingsPath("claude-code", ScopeProject, cwd)
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(cwd, ".claude", "settings.json")
	if p != want {
		t.Errorf("project path = %q, want %q", p, want)
	}
}

func TestAgentSettingsPathUnknownAgent(t *testing.T) {
	hermeticEnv(t)
	if _, err := AgentSettingsPath("windsurf", ScopeGlobal, ""); err == nil {
		t.Error("unknown agent should error")
	}
}
