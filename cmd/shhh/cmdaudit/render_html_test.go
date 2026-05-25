package cmdaudit

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	auditpkg "github.com/Musubi42/shhh/internal/audit"
)

// mustReadFile reads a file or fails the test.
func mustReadFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(data)
}

func baseResult() *auditpkg.Result {
	t0 := time.Date(2026, 4, 14, 12, 45, 3, 0, time.UTC)
	installed := t0.Add(-24 * time.Hour)
	return &auditpkg.Result{
		SchemaVersion: 1,
		Agent:         "claude-code",
		AuditTime:     t0,
		ScanDuration:  1200 * time.Millisecond,
		Projects: []auditpkg.Project{
			{
				AbsPath:       "/Users/alice/work/backend",
				DisplayPath:   "~/work/backend",
				DashName:      "-Users-alice-work-backend",
				Status:        auditpkg.StatusUnprotected,
				OnDisk:        true,
				SessionsTotal: 18,
				FirstSeen:     t0.Add(-90 * 24 * time.Hour),
				LastSessionAt: t0.Add(-1 * 24 * time.Hour),
				Leaked: []auditpkg.Finding{
					{
						Placeholder: "[STRIPE_LIVE_KEY:sk_live_...:a1b2]",
						Label:       "STRIPE_LIVE_KEY",
						Severity:    auditpkg.SevLeaked,
						Sources:     []string{"transcript", "paste-cache"},
						Occurrences: 4,
						FirstSeen:   t0.Add(-24 * 24 * time.Hour),
						LastSeen:    t0.Add(-4 * 24 * time.Hour),
						SessionIDs:  []string{"s1", "s2", "s3", "s4"},
					},
				},
				AtRisk: []auditpkg.Finding{
					{
						Placeholder: "[POSTGRES_CONNSTRING:admin@prod-db.internal:5432/myapp:e5f6]",
						Label:       "POSTGRES_URL",
						Severity:    auditpkg.SevAtRisk,
						Locations:   []string{".env:4"},
					},
				},
			},
			{
				AbsPath:         "/Users/alice/Documents/Musubi42/shhh",
				DisplayPath:     "~/Documents/Musubi42/shhh",
				DashName:        "-Users-alice-Documents-Musubi42-shhh",
				Status:          auditpkg.StatusProtected,
				OnDisk:          true,
				SessionsTotal:   12,
				FirstSeen:       t0.Add(-30 * 24 * time.Hour),
				LastSessionAt:   t0.Add(-1 * 24 * time.Hour),
				ShhhInstalledAt: &installed,
				AtRisk: []auditpkg.Finding{
					{
						Placeholder: "[STRIPE_LIVE_KEY:sk_live_...:k1l2]",
						Label:       "STRIPE_LIVE_KEY",
						Severity:    auditpkg.SevAtRisk,
						Locations:   []string{"testdata/fixtures/leaky-project/.env:2"},
					},
				},
			},
		},
		Summary: auditpkg.Summary{
			ProjectsTotal:       2,
			ProjectsUnprotected: 1,
			ProjectsProtected:   1,
			SecretsLeaked:       1,
			SecretsAtRisk:       2,
			SecretsProtected:    0,
		},
		Delta: &auditpkg.Delta{
			Since:     t0.Add(-4 * 24 * time.Hour),
			Leaked:    auditpkg.DeltaCount{Before: 4, After: 1, Change: -3},
			AtRisk:    auditpkg.DeltaCount{Before: 5, After: 2, Change: -3},
			Protected: auditpkg.DeltaCount{Before: 0, After: 0, Change: 0},
		},
	}
}

func TestRenderHTMLBasic(t *testing.T) {
	outDir := t.TempDir()
	r := baseResult()

	if err := RenderHTML(outDir, r); err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}

	indexPath := filepath.Join(outDir, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		t.Fatalf("index.html missing: %v", err)
	}

	idx := mustReadFile(t, indexPath)
	wants := []string{
		"shhh",
		"CLAUDE CODE",
		"~/work/backend",
		"~/Documents/Musubi42/shhh",
		"[STRIPE_LIVE_KEY:sk_live_...:a1b2]",
		"[POSTGRES_CONNSTRING:admin@prod-db.internal:5432/myapp:e5f6]",
		"UNPROTECTED", // from badge label uppercase CSS; raw HTML is "Unprotected"
		"PROTECTED",
	}
	for _, w := range wants {
		// Badge labels are uppercased via CSS; raw HTML contains capitalized forms.
		check := w
		if w == "UNPROTECTED" {
			check = "Unprotected"
		}
		if w == "PROTECTED" {
			check = "Protected"
		}
		if !strings.Contains(idx, check) {
			t.Errorf("index.html missing %q", check)
		}
	}

	// Projects directory should contain two html files (both projects have findings).
	projectsDir := filepath.Join(outDir, "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		t.Fatalf("readdir projects: %v", err)
	}
	count := 0
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".html") {
			count++
		}
	}
	if count != 2 {
		t.Errorf("expected 2 project html files, got %d", count)
	}

	// Each detail page should contain its project path and at least one placeholder.
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".html") {
			continue
		}
		content := mustReadFile(t, filepath.Join(projectsDir, e.Name()))
		if !strings.Contains(content, "~/") {
			t.Errorf("project page %s missing display path prefix", e.Name())
		}
		if !strings.Contains(content, "[") || !strings.Contains(content, "]") {
			t.Errorf("project page %s missing placeholder brackets", e.Name())
		}
		if !strings.Contains(content, "../index.html") {
			t.Errorf("project page %s missing back link", e.Name())
		}
	}
}

func TestRenderHTMLNoFindings(t *testing.T) {
	outDir := t.TempDir()
	t0 := time.Date(2026, 4, 14, 12, 0, 0, 0, time.UTC)
	r := &auditpkg.Result{
		SchemaVersion: 1,
		Agent:         "claude-code",
		AuditTime:     t0,
		ScanDuration:  500 * time.Millisecond,
		Projects: []auditpkg.Project{
			{
				AbsPath:       "/Users/alice/clean",
				DisplayPath:   "~/clean",
				Status:        auditpkg.StatusClean,
				OnDisk:        true,
				SessionsTotal: 3,
				FirstSeen:     t0.Add(-10 * 24 * time.Hour),
			},
		},
		Summary: auditpkg.Summary{ProjectsTotal: 1, ProjectsClean: 1},
	}

	if err := RenderHTML(outDir, r); err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}

	idx := mustReadFile(t, filepath.Join(outDir, "index.html"))
	if !strings.Contains(idx, "No findings") && !strings.Contains(idx, "clean") {
		t.Errorf("index.html should indicate clean state")
	}

	// No project detail pages should exist.
	if entries, err := os.ReadDir(filepath.Join(outDir, "projects")); err == nil {
		count := 0
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".html") {
				count++
			}
		}
		if count != 0 {
			t.Errorf("expected no project pages, got %d", count)
		}
	}
}

func TestRenderHTMLFirstAudit(t *testing.T) {
	outDir := t.TempDir()
	r := baseResult()
	r.Delta = nil

	if err := RenderHTML(outDir, r); err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	idx := mustReadFile(t, filepath.Join(outDir, "index.html"))
	if !strings.Contains(idx, "first audit") {
		t.Errorf("first-audit index.html should contain 'first audit', got:\n%s", idx[:min(500, len(idx))])
	}
	if strings.Contains(idx, "Since last audit") {
		t.Errorf("first-audit index.html should not contain 'Since last audit'")
	}
	if strings.Contains(idx, "Progress since last audit") {
		t.Errorf("first-audit index.html should not contain 'Progress since last audit'")
	}
}

func TestRenderHTMLOutputIsValidHTML(t *testing.T) {
	outDir := t.TempDir()
	r := baseResult()
	if err := RenderHTML(outDir, r); err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}
	idx := mustReadFile(t, filepath.Join(outDir, "index.html"))
	for _, tag := range []string{"<html", "<body", "</body>", "</html>"} {
		if !strings.Contains(idx, tag) {
			t.Errorf("index.html missing %q", tag)
		}
	}
	// Spot check a project page too.
	entries, _ := os.ReadDir(filepath.Join(outDir, "projects"))
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".html") {
			continue
		}
		content := mustReadFile(t, filepath.Join(outDir, "projects", e.Name()))
		for _, tag := range []string{"<html", "<body", "</body>", "</html>"} {
			if !strings.Contains(content, tag) {
				t.Errorf("project page %s missing %q", e.Name(), tag)
			}
		}
		break
	}
}

func TestRenderHTMLNoRawSecrets(t *testing.T) {
	outDir := t.TempDir()
	r := baseResult()
	if err := RenderHTML(outDir, r); err != nil {
		t.Fatalf("RenderHTML: %v", err)
	}

	// Regex for anything that looks like a real sk_live_* secret (> 5 chars after prefix).
	reStripe := regexp.MustCompile(`sk_live_[A-Za-z0-9]{6,}`)
	// Regex for a real AWS access key.
	reAWS := regexp.MustCompile(`AKIA[A-Z0-9]{15,}`)

	files := []string{filepath.Join(outDir, "index.html")}
	if entries, err := os.ReadDir(filepath.Join(outDir, "projects")); err == nil {
		for _, e := range entries {
			if strings.HasSuffix(e.Name(), ".html") {
				files = append(files, filepath.Join(outDir, "projects", e.Name()))
			}
		}
	}

	for _, f := range files {
		content := mustReadFile(t, f)
		if reStripe.MatchString(content) {
			m := reStripe.FindString(content)
			t.Errorf("%s contains a real-looking stripe secret: %q", f, m)
		}
		if reAWS.MatchString(content) {
			m := reAWS.FindString(content)
			t.Errorf("%s contains a real-looking AWS secret: %q", f, m)
		}
	}
}

func TestRenderHTMLSlugDeterministic(t *testing.T) {
	r1 := baseResult()
	r2 := baseResult()
	slugs1 := buildSlugs(r1.Projects)
	slugs2 := buildSlugs(r2.Projects)
	for k, v := range slugs1 {
		if slugs2[k] != v {
			t.Errorf("slug non-deterministic: %q got %q then %q", k, v, slugs2[k])
		}
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
