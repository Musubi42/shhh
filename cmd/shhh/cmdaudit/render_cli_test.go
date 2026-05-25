package cmdaudit

import (
	"bytes"
	"strings"
	"testing"
	"time"

	auditpkg "github.com/Musubi42/shhh/internal/audit"
)

func sampleResult() *auditpkg.Result {
	t0 := time.Date(2026, 1, 12, 9, 30, 0, 0, time.UTC)
	tSeen := time.Date(2026, 3, 21, 14, 0, 0, 0, time.UTC)
	installed := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	return &auditpkg.Result{
		SchemaVersion: 1,
		Agent:         "claude-code",
		AuditTime:     time.Date(2026, 4, 14, 12, 45, 3, 0, time.UTC),
		ScanDuration:  1200 * time.Millisecond,
		Projects: []auditpkg.Project{
			{
				AbsPath:       "/Users/alice/work/backend",
				DisplayPath:   "~/work/backend",
				Status:        auditpkg.StatusUnprotected,
				OnDisk:        true,
				SessionsTotal: 18,
				FirstSeen:     t0,
				Leaked: []auditpkg.Finding{{
					Placeholder: "[STRIPE_LIVE_KEY:sk_live_...:a1b2]",
					Label:       "STRIPE_LIVE_KEY",
					Severity:    auditpkg.SevLeaked,
					Sources:     []string{"transcript", "paste-cache"},
					Occurrences: 4,
					FirstSeen:   tSeen,
					LastSeen:    tSeen,
					SessionIDs:  []string{"s1", "s2", "s3", "s4"},
				}},
				AtRisk: []auditpkg.Finding{{
					Placeholder: "[POSTGRES_CONNSTRING:admin@prod-db.internal:5432/myapp:e5f6]",
					Label:       "POSTGRES_CONNSTRING",
					Severity:    auditpkg.SevAtRisk,
					Locations:   []string{".env:4"},
				}},
			},
			{
				AbsPath:         "/Users/alice/work/safe",
				DisplayPath:     "~/work/safe",
				Status:          auditpkg.StatusProtected,
				OnDisk:          true,
				SessionsTotal:   5,
				FirstSeen:       t0,
				ShhhInstalledAt: &installed,
			},
		},
		Summary: auditpkg.Summary{
			ProjectsTotal:       2,
			ProjectsUnprotected: 1,
			ProjectsProtected:   1,
			SecretsLeaked:       1,
			SecretsAtRisk:       1,
			SecretsProtected:    0,
		},
	}
}

func TestRenderCLIBasic(t *testing.T) {
	r := sampleResult()
	var buf bytes.Buffer
	if err := RenderCLI(&buf, r, false); err != nil {
		t.Fatalf("RenderCLI: %v", err)
	}
	out := buf.String()

	wants := []string{
		"shhh audit",
		"Claude Code",
		"[STRIPE_LIVE_KEY:sk_live_...:a1b2]",
		"[POSTGRES_CONNSTRING:admin@prod-db.internal:5432/myapp:e5f6]",
		"~/work/backend",
		"~/work/safe",
		"UNPROTECTED",
		"PROTECTED",
	}
	for _, w := range wants {
		if !strings.Contains(out, w) {
			t.Errorf("expected output to contain %q", w)
		}
	}
	if strings.Contains(out, "\x1b[") {
		t.Errorf("expected no ANSI escapes when useColor=false; got: %q", out)
	}
}

func TestRenderCLIWithDelta(t *testing.T) {
	r := sampleResult()
	r.Delta = &auditpkg.Delta{
		Since:     time.Date(2026, 4, 10, 8, 12, 0, 0, time.UTC),
		Leaked:    auditpkg.DeltaCount{Before: 7, After: 4, Change: -3},
		AtRisk:    auditpkg.DeltaCount{Before: 6, After: 2, Change: -4},
		Protected: auditpkg.DeltaCount{Before: 0, After: 1, Change: 1},
	}
	var buf bytes.Buffer
	if err := RenderCLI(&buf, r, false); err != nil {
		t.Fatalf("RenderCLI: %v", err)
	}
	out := buf.String()

	for _, w := range []string{"Since last audit", "Leaked", "At risk", "Protected", "▼", "7 → 4"} {
		if !strings.Contains(out, w) {
			t.Errorf("expected %q in delta output", w)
		}
	}
}

func TestRenderCLIFirstAudit(t *testing.T) {
	r := sampleResult()
	r.Delta = nil
	var buf bytes.Buffer
	if err := RenderCLI(&buf, r, false); err != nil {
		t.Fatalf("RenderCLI: %v", err)
	}
	out := buf.String()
	for _, w := range []string{"First audit", "no previous snapshot"} {
		if !strings.Contains(out, w) {
			t.Errorf("expected %q in first-audit output", w)
		}
	}
}

func TestRenderCLICleanScan(t *testing.T) {
	r := &auditpkg.Result{
		SchemaVersion: 1,
		Agent:         "claude-code",
		AuditTime:     time.Now().UTC(),
		Projects: []auditpkg.Project{
			{
				AbsPath:     "/Users/alice/work/clean",
				DisplayPath: "~/work/clean",
				Status:      auditpkg.StatusClean,
			},
		},
		Summary: auditpkg.Summary{
			ProjectsTotal: 1,
			ProjectsClean: 1,
		},
	}
	var buf bytes.Buffer
	if err := RenderCLI(&buf, r, false); err != nil {
		t.Fatalf("RenderCLI: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "No secrets found") {
		t.Errorf("expected clean-scan sentinel; got: %s", out)
	}
	if strings.Contains(out, "📁") {
		t.Errorf("expected no project detail blocks in clean scan; got: %s", out)
	}
	if strings.Contains(out, "Rotation dashboards") {
		t.Errorf("expected no action block in clean scan")
	}
}

func TestRenderCLIColorCodes(t *testing.T) {
	r := sampleResult()
	var buf bytes.Buffer
	if err := RenderCLI(&buf, r, true); err != nil {
		t.Fatalf("RenderCLI: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "\x1b[31m") {
		t.Errorf("expected ANSI red escape for leaked content")
	}
}

func TestRenderCLIRotationDashboards(t *testing.T) {
	r := sampleResult()
	var buf bytes.Buffer
	if err := RenderCLI(&buf, r, false); err != nil {
		t.Fatalf("RenderCLI: %v", err)
	}
	out := buf.String()
	for _, w := range []string{"stripe", "dashboard.stripe.com"} {
		if !strings.Contains(out, w) {
			t.Errorf("expected rotation link for %q; got: %s", w, out)
		}
	}
}
