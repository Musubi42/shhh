package redactor

import (
	"strings"
	"testing"

	"github.com/musubi-sasu/shhh/internal/detector"
	"github.com/musubi-sasu/shhh/internal/session"
)

func newTestRedactor() *Redactor {
	return New(detector.New(), session.New())
}

func TestRedactBasic(t *testing.T) {
	r := newTestRedactor()
	input := `STRIPE_KEY=sk_live_4eC39HqLyjWDarjtT1zdp7dc
AWS=AKIA3EXAMPLE7XYZABC4
HOST=localhost`
	out, findings := r.RedactString(input)
	if len(findings) < 2 {
		t.Errorf("expected ≥2 findings, got %d", len(findings))
	}
	if strings.Contains(out, "sk_live_4eC39HqLyjWDarjtT1zdp7dc") {
		t.Error("raw Stripe key leaked in output")
	}
	if strings.Contains(out, "AKIA3EXAMPLE7XYZABC4") {
		t.Error("raw AWS key leaked in output")
	}
	if !strings.Contains(out, "STRIPE_LIVE_KEY") {
		t.Error("expected Stripe placeholder in output")
	}
	if !strings.Contains(out, "localhost") {
		t.Error("non-secret content should survive")
	}
}

func TestRedactRehydrateRoundTrip(t *testing.T) {
	r := newTestRedactor()
	original := `API_KEY=sk_live_4eC39HqLyjWDarjtT1zdp7dc
TOKEN=ghp_abcdefghijklmnopqrstuvwxyz0123456789`
	redacted, _ := r.RedactString(original)
	if redacted == original {
		t.Fatal("redact produced no change")
	}
	recovered := r.RehydrateString(redacted)
	if recovered != original {
		t.Errorf("round-trip mismatch:\n  orig:  %q\n  red:   %q\n  back:  %q", original, redacted, recovered)
	}
}

func TestRedactDeterministicWithinSession(t *testing.T) {
	r := newTestRedactor()
	value := "sk_live_4eC39HqLyjWDarjtT1zdp7dc"
	input := "A=" + value + "\nB=" + value
	out, _ := r.RedactString(input)
	// The same value should map to the same placeholder both times.
	lines := strings.Split(out, "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	a := strings.TrimPrefix(lines[0], "A=")
	b := strings.TrimPrefix(lines[1], "B=")
	if a != b {
		t.Errorf("same value should redact to same placeholder: %q vs %q", a, b)
	}
}

func TestRehydrateFailClosed(t *testing.T) {
	r := newTestRedactor()
	// Placeholder-shaped string never registered in the session.
	input := "nothing to see here [FAKE:xyz:deadbeef] nothing"
	out := r.RehydrateString(input)
	if out != input {
		t.Errorf("fail-closed rehydrate changed unknown placeholder: %q -> %q", input, out)
	}
}

func TestRehydrateOnlyKnownPlaceholders(t *testing.T) {
	r := newTestRedactor()
	// Register one value.
	original := "real secret: sk_live_4eC39HqLyjWDarjtT1zdp7dc"
	redacted, _ := r.RedactString(original)
	// Append a fake placeholder to the redacted text and rehydrate.
	mixed := redacted + " and an unknown [FAKE:xyz:deadbeef]"
	recovered := r.RehydrateString(mixed)
	if !strings.Contains(recovered, "sk_live_4eC39HqLyjWDarjtT1zdp7dc") {
		t.Error("known placeholder should be rehydrated")
	}
	if !strings.Contains(recovered, "[FAKE:xyz:deadbeef]") {
		t.Error("unknown placeholder should be preserved")
	}
}
