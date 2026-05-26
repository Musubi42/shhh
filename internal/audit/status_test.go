package audit

import "testing"

func TestDecideStatus(t *testing.T) {
	t.Parallel()

	mkFindings := func(n int) []Finding {
		out := make([]Finding, n)
		for i := range out {
			out[i] = Finding{Label: "X"}
		}
		return out
	}

	cases := []struct {
		name           string
		onDisk         bool
		leaked         int
		atRisk         int
		scope          string
		installedPaths []string
		projectAbs     string
		want           Status
	}{
		{
			name:           "archived-no-matter-what",
			onDisk:         false,
			leaked:         1,
			scope:          "global",
			installedPaths: []string{"/Users/alice/.claude/settings.json"},
			projectAbs:     "/Users/alice/gone",
			want:           StatusArchived,
		},
		{
			name:           "global-protected",
			onDisk:         true,
			scope:          "global",
			installedPaths: []string{"/Users/alice/.claude/settings.json"},
			projectAbs:     "/Users/alice/work/backend",
			want:           StatusProtected,
		},
		{
			name:           "global-protected-with-findings",
			onDisk:         true,
			leaked:         2,
			atRisk:         1,
			scope:          "global",
			installedPaths: []string{"/Users/alice/.claude/settings.json"},
			projectAbs:     "/Users/alice/work/backend",
			want:           StatusProtected,
		},
		{
			name:           "project-protected-matching",
			onDisk:         true,
			scope:          "project",
			installedPaths: []string{"/work/backend/.claude/settings.json"},
			projectAbs:     "/work/backend",
			want:           StatusProtected,
		},
		{
			name:           "project-protected-findings",
			onDisk:         true,
			leaked:         1,
			scope:          "project",
			installedPaths: []string{"/work/backend/.claude/settings.json"},
			projectAbs:     "/work/backend",
			want:           StatusProtected,
		},
		{
			name:           "project-not-matching",
			onDisk:         true,
			leaked:         1,
			scope:          "project",
			installedPaths: []string{"/other/.claude/settings.json"},
			projectAbs:     "/work/backend",
			want:           StatusUnprotected,
		},
		{
			name:       "unprotected-with-leaked",
			onDisk:     true,
			leaked:     1,
			scope:      "",
			projectAbs: "/work/backend",
			want:       StatusUnprotected,
		},
		{
			name:       "unprotected-with-at-risk",
			onDisk:     true,
			atRisk:     1,
			scope:      "",
			projectAbs: "/work/backend",
			want:       StatusUnprotected,
		},
		{
			name:       "clean",
			onDisk:     true,
			scope:      "",
			projectAbs: "/work/backend",
			want:       StatusClean,
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			p := &Project{
				AbsPath: tc.projectAbs,
				OnDisk:  tc.onDisk,
				Leaked:  mkFindings(tc.leaked),
				AtRisk:  mkFindings(tc.atRisk),
			}
			got := DecideStatus(p, tc.installedPaths, tc.scope)
			if got != tc.want {
				t.Errorf("DecideStatus = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestApplyStatusMutates(t *testing.T) {
	t.Parallel()
	p := &Project{
		AbsPath: "/work/backend",
		OnDisk:  true,
		Leaked:  []Finding{{Label: "X"}},
	}
	if p.Status != "" {
		t.Fatalf("precondition: expected empty status, got %q", p.Status)
	}
	got := ApplyStatus(p, nil, "")
	if got != StatusUnprotected {
		t.Errorf("returned status = %q, want %q", got, StatusUnprotected)
	}
	if p.Status != StatusUnprotected {
		t.Errorf("p.Status = %q, want %q", p.Status, StatusUnprotected)
	}
}

func TestCoversPathSubdirectory(t *testing.T) {
	t.Parallel()
	installed := []string{"/work/backend/.claude/settings.json"}
	if !coversPath(installed, "/work/backend/sub/dir") {
		t.Error("expected /work/backend/sub/dir to be covered")
	}
	if coversPath(installed, "/work/backend-other") {
		t.Error("expected /work/backend-other to NOT be covered (prefix trap)")
	}
}

func TestPathUnderAny(t *testing.T) {
	t.Parallel()
	roots := []string{"/work/backend", "/home/me/repo"}
	cases := []struct {
		path string
		want bool
	}{
		{"/work/backend", true},               // exact
		{"/work/backend/sub/file", true},      // descendant
		{"/work/backend-other", false},        // prefix trap
		{"/work", false},                      // parent of root, not under it
		{"/home/me/repo/deep/nested", true},   // second root match
		{"/home/me/repository", false},        // sibling-like
		{"/work/backend/", true},              // trailing slash
	}
	for _, tc := range cases {
		if got := pathUnderAny(tc.path, roots); got != tc.want {
			t.Errorf("pathUnderAny(%q) = %v, want %v", tc.path, got, tc.want)
		}
	}
}
