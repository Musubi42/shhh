package cmdaudit

import (
	"bytes"
	"encoding/json"
	"regexp"
	"strings"
	"testing"
	"time"

	auditpkg "github.com/Musubi42/shhh/internal/audit"
)

func TestRenderJSONShape(t *testing.T) {
	r := sampleResult()
	r.Delta = &auditpkg.Delta{
		Since:     time.Date(2026, 4, 10, 8, 12, 0, 0, time.UTC),
		Leaked:    auditpkg.DeltaCount{Before: 7, After: 4, Change: -3},
		AtRisk:    auditpkg.DeltaCount{Before: 6, After: 2, Change: -4},
		Protected: auditpkg.DeltaCount{Before: 0, After: 1, Change: 1},
	}

	var buf bytes.Buffer
	if err := RenderJSON(&buf, r); err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}

	var m map[string]interface{}
	if err := json.Unmarshal(buf.Bytes(), &m); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if got := m["schema_version"]; got != float64(1) {
		t.Errorf("schema_version = %v, want 1", got)
	}
	if got := m["agent"]; got != "claude-code" {
		t.Errorf("agent = %v", got)
	}
	if got := m["scan_duration_ms"]; got != float64(1200) {
		t.Errorf("scan_duration_ms = %v, want 1200", got)
	}

	summary, ok := m["summary"].(map[string]interface{})
	if !ok {
		t.Fatalf("summary missing or wrong type")
	}
	if summary["secrets_leaked"] != float64(1) {
		t.Errorf("secrets_leaked = %v, want 1", summary["secrets_leaked"])
	}
	if summary["secrets_at_risk"] != float64(1) {
		t.Errorf("secrets_at_risk = %v, want 1", summary["secrets_at_risk"])
	}

	delta, ok := m["delta"].(map[string]interface{})
	if !ok {
		t.Fatalf("delta missing or wrong type")
	}
	leaked := delta["leaked"].(map[string]interface{})
	if leaked["before"] != float64(7) || leaked["after"] != float64(4) || leaked["change"] != float64(-3) {
		t.Errorf("delta.leaked = %v", leaked)
	}
	if _, ok := m["delta_since"]; !ok {
		t.Errorf("delta_since missing")
	}

	projects, ok := m["projects"].([]interface{})
	if !ok || len(projects) != 2 {
		t.Fatalf("projects missing or wrong length: %v", projects)
	}
	p0 := projects[0].(map[string]interface{})
	if p0["status"] != "unprotected" {
		t.Errorf("p0.status = %v", p0["status"])
	}
	leakedList := p0["leaked"].([]interface{})
	if len(leakedList) != 1 {
		t.Fatalf("expected 1 leaked finding")
	}
	lf := leakedList[0].(map[string]interface{})
	if lf["placeholder"] != "[STRIPE_LIVE_KEY:sk_live_...:a1b2]" {
		t.Errorf("leaked placeholder = %v", lf["placeholder"])
	}
	if lf["session_count"] != float64(4) {
		t.Errorf("session_count = %v", lf["session_count"])
	}

	atRisk := p0["at_risk"].([]interface{})
	if len(atRisk) != 1 {
		t.Fatalf("expected 1 at-risk finding")
	}
	ar := atRisk[0].(map[string]interface{})
	if ar["location"] != ".env:4" {
		t.Errorf("at_risk location = %v", ar["location"])
	}
	if _, has := ar["rule"]; has {
		t.Errorf("at_risk.rule should be omitted in v0.2")
	}
}

func TestRenderJSONOmitsDelta(t *testing.T) {
	r := sampleResult()
	r.Delta = nil
	var buf bytes.Buffer
	if err := RenderJSON(&buf, r); err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	s := buf.String()
	if strings.Contains(s, "\"delta\"") {
		t.Errorf("expected no delta key; got: %s", s)
	}
	if strings.Contains(s, "delta_since") {
		t.Errorf("expected no delta_since key; got: %s", s)
	}
}

func TestRenderJSONNoRawSecrets(t *testing.T) {
	r := sampleResult()
	var buf bytes.Buffer
	if err := RenderJSON(&buf, r); err != nil {
		t.Fatalf("RenderJSON: %v", err)
	}
	s := buf.String()

	if !strings.Contains(s, "[STRIPE_LIVE_KEY:sk_live_...:a1b2]") {
		t.Errorf("expected verbatim placeholder in JSON output")
	}
	// A real stripe key would be "sk_live_" followed by alphanumerics.
	// The placeholder contains "sk_live_..." which is safe — the ...
	// is the redaction marker. Assert no "sk_live_" followed by
	// 8+ alphanumerics (i.e. a plausible raw key).
	rawKey := regexp.MustCompile(`sk_live_[A-Za-z0-9]{8,}`)
	if rawKey.MatchString(s) {
		t.Errorf("JSON output appears to contain a raw Stripe key: %s", s)
	}
}
