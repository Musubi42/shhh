package cmdhook

import (
	"testing"
)

func TestLoadRedactorFreshAndPersist(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	const sid = "test-session-0001"

	// Fresh store: no file yet.
	r1, save1, err := LoadRedactor(sid)
	if err != nil {
		t.Fatalf("LoadRedactor fresh: %v", err)
	}
	input := []byte(`STRIPE=sk_live_4eC39HqLyjWDarjtT1zdp7dc`)
	out1, findings := r1.Redact(input)
	if len(findings) == 0 {
		t.Fatalf("expected findings on %q", input)
	}
	if string(out1) == string(input) {
		t.Fatalf("expected redaction to change input")
	}
	if err := save1(); err != nil {
		t.Fatalf("save1: %v", err)
	}

	// Reload: same input should produce identical placeholder (salt carried).
	r2, _, err := LoadRedactor(sid)
	if err != nil {
		t.Fatalf("LoadRedactor reload: %v", err)
	}
	out2, _ := r2.Redact(input)
	if string(out2) != string(out1) {
		t.Errorf("placeholder not stable across reload:\n  %s\n  %s", out1, out2)
	}

	// And resolving the placeholder yields the original value.
	placeholder := string(out2[len("STRIPE="):])
	got, ok := r2.Resolve(placeholder)
	if !ok {
		t.Fatalf("resolve miss on %q", placeholder)
	}
	if got != "sk_live_4eC39HqLyjWDarjtT1zdp7dc" {
		t.Errorf("resolve got %q", got)
	}
}

func TestLoadRedactorRejectsBadSessionID(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	for _, sid := range []string{"", "../etc/passwd", "with/slash", "with space"} {
		if _, _, err := LoadRedactor(sid); err == nil {
			t.Errorf("expected error for session_id %q", sid)
		}
	}
}

func TestWipeSession(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	const sid = "test-session-wipe"
	_, save, err := LoadRedactor(sid)
	if err != nil {
		t.Fatal(err)
	}
	if err := save(); err != nil {
		t.Fatal(err)
	}
	if err := WipeSession(sid); err != nil {
		t.Fatalf("wipe: %v", err)
	}
	// After wipe, loading again gives a fresh salt. Can't directly assert
	// "fresh" without introspection; settle for "load succeeds, save
	// succeeds, no error."
	_, save2, err := LoadRedactor(sid)
	if err != nil {
		t.Fatal(err)
	}
	if err := save2(); err != nil {
		t.Fatal(err)
	}
}
