package cmdinstall

import (
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	auditpkg "github.com/Musubi42/shhh/internal/audit"
)

// TestProjectScopeInstallAuditUninstallCycle exercises the full
// per-project install lifecycle end-to-end:
//
//  1. install --scope project writes a hook into <project>/.claude/settings.json
//  2. shhh config records the path under the "project" scope
//  3. audit's DecideStatus reports the project as Protected
//  4. uninstall removes the hook and updates config
//  5. audit now reports the project as Unprotected (findings present) or Clean
//
// This is the missing automated coverage for the 2026-05-25 MVP — the
// dryrun was verified manually and silently regressed once already.
func TestProjectScopeInstallAuditUninstallCycle(t *testing.T) {
	_, shhhDir, projectRoot := hermeticEnv(t)

	if err := installClaudeCode(ScopeProject, []string{projectRoot}); err != nil {
		t.Fatalf("installClaudeCode: %v", err)
	}

	settingsPath := filepath.Join(projectRoot, ".claude", "settings.json")
	buf, err := os.ReadFile(settingsPath)
	if err != nil {
		t.Fatalf("project settings.json missing after install: %v", err)
	}
	if !strings.Contains(string(buf), "hook claude-code") {
		t.Fatalf("settings.json missing hook entry:\n%s", buf)
	}

	cfg, err := LoadConfig()
	if err != nil || cfg == nil {
		t.Fatalf("LoadConfig after install: cfg=%+v err=%v", cfg, err)
	}
	if cfg.Scope != string(ScopeProject) {
		t.Errorf("config scope = %q after project install, want %q", cfg.Scope, ScopeProject)
	}
	if len(cfg.Paths) != 1 || cfg.Paths[0] != settingsPath {
		t.Errorf("config paths = %v, want [%s]", cfg.Paths, settingsPath)
	}

	// Audit: a project at projectRoot with a fake finding should be Protected.
	proj := &auditpkg.Project{
		AbsPath: projectRoot,
		OnDisk:  true,
		Leaked:  []auditpkg.Finding{{Placeholder: "[FAKE]"}},
	}
	status := auditpkg.DecideStatus(proj, cfg.Paths, cfg.Scope)
	if status != auditpkg.StatusProtected {
		t.Errorf("after install, project status = %q, want %q", status, auditpkg.StatusProtected)
	}

	// A SIBLING project outside the install root must NOT be reported as protected.
	sibling := &auditpkg.Project{
		AbsPath: filepath.Join(filepath.Dir(projectRoot), "other-project"),
		OnDisk:  true,
		Leaked:  []auditpkg.Finding{{Placeholder: "[FAKE]"}},
	}
	if got := auditpkg.DecideStatus(sibling, cfg.Paths, cfg.Scope); got == auditpkg.StatusProtected {
		t.Errorf("sibling project (outside install root) should not be Protected, got %q", got)
	}

	// Uninstall.
	if err := uninstallClaudeCode(ScopeProject, []string{projectRoot}); err != nil {
		t.Fatalf("uninstallClaudeCode: %v", err)
	}
	cfg2, _ := LoadConfig()
	if cfg2 == nil {
		t.Fatal("config gone after uninstall")
	}
	if len(cfg2.Paths) != 0 {
		t.Errorf("config paths = %v after uninstall, want empty", cfg2.Paths)
	}
	if cfg2.Scope != "" {
		t.Errorf("config scope = %q after uninstall, want empty", cfg2.Scope)
	}
	if status := auditpkg.DecideStatus(proj, cfg2.Paths, cfg2.Scope); status != auditpkg.StatusUnprotected {
		t.Errorf("after uninstall, status = %q, want %q", status, auditpkg.StatusUnprotected)
	}

	// .claude/ directory must still exist (we never delete it).
	if _, err := os.Stat(filepath.Join(projectRoot, ".claude")); err != nil {
		t.Errorf(".claude/ should be retained after uninstall: %v", err)
	}
	_ = shhhDir
}

// TestValidateProjectPath covers the F2 dryrun rules: refuse paths
// that obviously aren't project roots so the install side-effects
// can't damage random directories.
func TestValidateProjectPath(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		wantErr string
	}{
		{"empty", "", "empty project path"},
		{"flag-typo single dash", "-foo", "starts with '-'"},
		{"flag-typo double dash", "--scope", "starts with '-'"},
		{"dotclaude literal", ".claude", "ends in .claude"},
		{"dotclaude trailing slash", "/tmp/foo/.claude/", "ends in .claude"},
		{"dotdot middle", "/tmp/../etc", "contains '..'"},
		{"plain abs ok", "/tmp/foo", ""},
		{"plain relative ok", "./foo", ""},
		{"tilde left for shell ok", "~/work", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateProjectPath(tc.path)
			if tc.wantErr == "" {
				if err != nil {
					t.Errorf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("want err containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

// TestSplitArgsByFlags locks the 2026-05-26 dryrun fix: install
// flags must work no matter where they sit relative to positional
// paths. The unique twist here (vs cmdaudit) is that --scope and
// --cwd take values, so the splitter must consult the FlagSet to
// know when to also consume the next arg.
func TestSplitArgsByFlags(t *testing.T) {
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	_ = fs.String("scope", "", "")
	_ = fs.String("cwd", "", "")
	_ = fs.Bool("verbose", false, "")

	cases := []struct {
		name           string
		args           []string
		wantFlags      []string
		wantPositional []string
	}{
		{
			"flags before",
			[]string{"--scope", "project", "~/a"},
			[]string{"--scope", "project"},
			[]string{"~/a"},
		},
		{
			"flags after (the regression)",
			[]string{"~/a", "--scope", "global"},
			[]string{"--scope", "global"},
			[]string{"~/a"},
		},
		{
			"flag with equals",
			[]string{"~/a", "--scope=project"},
			[]string{"--scope=project"},
			[]string{"~/a"},
		},
		{
			"bool flag does not steal next arg",
			[]string{"--verbose", "~/a"},
			[]string{"--verbose"},
			[]string{"~/a"},
		},
		{
			"interleaved with multiple positionals",
			[]string{"~/a", "--scope", "project", "~/b"},
			[]string{"--scope", "project"},
			[]string{"~/a", "~/b"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, p := splitArgsByFlags(tc.args, fs)
			if !reflect.DeepEqual(f, tc.wantFlags) {
				t.Errorf("flags = %v, want %v", f, tc.wantFlags)
			}
			if !reflect.DeepEqual(p, tc.wantPositional) {
				t.Errorf("positionals = %v, want %v", p, tc.wantPositional)
			}
		})
	}
}

// TestParseInstallFlags exercises the positional+flag CLI shape.
func TestParseInstallFlags(t *testing.T) {
	hermeticEnv(t)
	cases := []struct {
		name      string
		args      []string
		wantScope Scope
		wantPaths int
		wantErr   string
	}{
		{name: "no args = global", args: nil, wantScope: ScopeGlobal, wantPaths: 0},
		{name: "explicit global", args: []string{"--scope", "global"}, wantScope: ScopeGlobal, wantPaths: 0},
		{name: "positional path = project", args: []string{"/tmp/a"}, wantScope: ScopeProject, wantPaths: 1},
		{name: "multi positional", args: []string{"/tmp/a", "/tmp/b", "/tmp/c"}, wantScope: ScopeProject, wantPaths: 3},
		{name: "cwd alias", args: []string{"--cwd", "/tmp/x"}, wantScope: ScopeProject, wantPaths: 1},
		{name: "scope project no path uses cwd", args: []string{"--scope", "project"}, wantScope: ScopeProject, wantPaths: 1},
		{name: "global + path is error", args: []string{"--scope", "global", "/tmp/a"}, wantErr: "incompatible"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			scope, paths, err := parseInstallFlags("install", tc.args)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("want err containing %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if scope != tc.wantScope {
				t.Errorf("scope = %q, want %q", scope, tc.wantScope)
			}
			if len(paths) != tc.wantPaths {
				t.Errorf("paths = %v (n=%d), want n=%d", paths, len(paths), tc.wantPaths)
			}
			for _, p := range paths {
				if !filepath.IsAbs(p) {
					t.Errorf("path %q should have been resolved to absolute", p)
				}
			}
		})
	}
}

// TestPlanExecuteMultiProject verifies the picker's multi-select path:
// a single Plan with N ProjectPaths installs the hook into all N
// projects in one Execute call.
func TestPlanExecuteMultiProject(t *testing.T) {
	_, _, root := hermeticEnv(t)
	a := filepath.Join(filepath.Dir(root), "proj-a")
	b := filepath.Join(filepath.Dir(root), "proj-b")
	for _, d := range []string{a, b} {
		if err := os.MkdirAll(d, 0o700); err != nil {
			t.Fatal(err)
		}
	}

	plan := &Plan{
		Scope:        ScopeProject,
		Agents:       []string{"claude-code"},
		ProjectPaths: []string{a, b},
	}
	var out strings.Builder
	if err := plan.Execute("/opt/shhh/bin/shhh", &out); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	for _, dir := range []string{a, b} {
		path := filepath.Join(dir, ".claude", "settings.json")
		buf, err := os.ReadFile(path)
		if err != nil {
			t.Errorf("missing settings.json under %s: %v", dir, err)
			continue
		}
		if !strings.Contains(string(buf), "hook claude-code") {
			t.Errorf("hook entry missing in %s:\n%s", path, buf)
		}
	}

	cfg, _ := LoadConfig()
	if cfg == nil || len(cfg.Paths) != 2 {
		t.Errorf("expected 2 installed paths in config, got %+v", cfg)
	}
}
