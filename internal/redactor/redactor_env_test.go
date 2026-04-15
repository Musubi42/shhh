package redactor

import (
	"strings"
	"testing"

	"github.com/musubi-sasu/shhh/internal/detector"
	"github.com/musubi-sasu/shhh/internal/session"
)

func newEnvRedactor() *Redactor {
	return New(detector.New(), session.New())
}

func TestRedactEnvFile_ShortHighQualityToken(t *testing.T) {
	// 22-char all-unique token: entropy = log2(22) ≈ 4.459, below the
	// generic 4.5 gate. The env-aware pass must catch it via the
	// looser CheckEnvValue gate (entropy >= 3.0, len >= 12).
	in := []byte("INTERNAL_LEGACY_TOKEN=Mk9zPwXr7AqN4bVtC2yLhG\nAPP_ENV=production\n")
	r := newEnvRedactor()
	out, findings := r.RedactEnvFile(in)
	s := string(out)

	if strings.Contains(s, "Mk9zPwXr7AqN4bVtC2yLhG") {
		t.Errorf("raw token leaked:\n%s", s)
	}
	if !strings.Contains(s, "[INTERNAL_LEGACY_TOKEN:") {
		t.Errorf("expected [INTERNAL_LEGACY_TOKEN:...] placeholder:\n%s", s)
	}
	if !strings.Contains(s, "APP_ENV=production") {
		t.Errorf("APP_ENV should be untouched:\n%s", s)
	}
	// production is 10 chars → below CheckEnvValue's len>=12, stays raw.
	foundInternal := false
	for _, f := range findings {
		if f.Label == "INTERNAL_LEGACY_TOKEN" {
			foundInternal = true
		}
	}
	if !foundInternal {
		t.Errorf("no INTERNAL_LEGACY_TOKEN finding: %+v", findings)
	}
}

func TestRedactEnvFile_DoesNotDoubleRedactPatternMatches(t *testing.T) {
	// Stripe key should fire stage-1 rule. env pass must not re-redact
	// the same span into a second placeholder.
	in := []byte("STRIPE=sk_live_4eC39HqLyjWDarjtT1zdp7dc\n")
	r := newEnvRedactor()
	out, findings := r.RedactEnvFile(in)
	s := string(out)
	if strings.Count(s, "[STRIPE_LIVE_KEY:") != 1 {
		t.Errorf("expected exactly one STRIPE_LIVE_KEY placeholder:\n%s", s)
	}
	// There should be exactly one finding for this span.
	spans := 0
	for _, f := range findings {
		if f.Label == "STRIPE_LIVE_KEY" {
			spans++
		}
	}
	if spans != 1 {
		t.Errorf("expected 1 STRIPE_LIVE_KEY finding, got %d: %+v", spans, findings)
	}
}

func TestRedactEnvFile_IgnoresCommentsAndWeakValues(t *testing.T) {
	in := []byte(`# comment line
APP_ENV=production
APP_PORT=3000
EMPTY=
SHORT=abc
`)
	r := newEnvRedactor()
	out, _ := r.RedactEnvFile(in)
	if string(out) != string(in) {
		t.Errorf("weak/short/empty values should not be redacted:\n  got:  %q\n  want: %q", out, in)
	}
}

func TestRedactEnvFile_QuotedValue(t *testing.T) {
	in := []byte(`TOKEN="Mk9zPwXr7AqN4bVtC2yLhG"` + "\n")
	r := newEnvRedactor()
	out, _ := r.RedactEnvFile(in)
	if strings.Contains(string(out), "Mk9zPwXr7AqN4bVtC2yLhG") {
		t.Errorf("quoted token leaked:\n%s", out)
	}
	// The surrounding quotes should survive.
	if !strings.Contains(string(out), `="[TOKEN:`) {
		t.Errorf("quotes should be preserved:\n%s", out)
	}
}

func TestRedactEnvFile_ExportPrefix(t *testing.T) {
	in := []byte("export API_KEY=Mk9zPwXr7AqN4bVtC2yLhG\n")
	r := newEnvRedactor()
	out, _ := r.RedactEnvFile(in)
	if strings.Contains(string(out), "Mk9zPwXr7AqN4bVtC2yLhG") {
		t.Errorf("export-prefixed token leaked:\n%s", out)
	}
	if !strings.Contains(string(out), "[API_KEY:") {
		t.Errorf("expected [API_KEY:...]:\n%s", out)
	}
}

func TestRedactEnvFile_SkipsExistingPlaceholder(t *testing.T) {
	// A fixture or already-processed file: value is already a placeholder.
	// Env pass must not treat the placeholder itself as a high-entropy value.
	in := []byte("STRIPE=[STRIPE_LIVE_KEY:sk_live_...:deadbeef]\n")
	r := newEnvRedactor()
	out, findings := r.RedactEnvFile(in)
	if string(out) != string(in) {
		t.Errorf("placeholder should pass through unchanged:\n  got:  %q\n  want: %q", out, in)
	}
	if len(findings) != 0 {
		t.Errorf("no findings expected, got %+v", findings)
	}
}

func TestRedactEnvFile_SanitizesKeyForLabel(t *testing.T) {
	// Digits in the key should be stripped so the label matches
	// session.PlaceholderRe's [A-Z_]+ alphabet.
	in := []byte("SERVICE2_KEY=Mk9zPwXr7AqN4bVtC2yLhG\n")
	r := newEnvRedactor()
	out, _ := r.RedactEnvFile(in)
	// Expect SERVICE_KEY, not SERVICE2_KEY.
	if !strings.Contains(string(out), "[SERVICE_KEY:") {
		t.Errorf("digit should be stripped from label:\n%s", out)
	}
}
