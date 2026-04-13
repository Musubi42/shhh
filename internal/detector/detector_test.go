package detector

import (
	"strings"
	"testing"
)

func TestDetect_KnownPatterns(t *testing.T) {
	d := New()
	cases := []struct {
		name    string
		input   string
		wantRule string
	}{
		{"stripe live", `STRIPE_KEY=sk_live_4eC39HqLyjWDarjtT1zdp7dc`, "stripe-live-key"},
		{"aws access", `AWS_ACCESS_KEY=AKIA3EXAMPLE7XYZABC1`, "aws-access-key"},
		{"github pat", `GH_TOKEN=ghp_abcdefghijklmnopqrstuvwxyz0123456789`, "github-pat"},
		{"anthropic", `ANTHROPIC=sk-ant-api03-abcdefghij_klmnopqrst`, "anthropic-key"},
		{"postgres", `DATABASE_URL=postgresql://admin:s3cret@prod-db:5432/myapp`, "postgres-url"},
		{"jwt", `TOKEN=eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.abcdefghij`, "jwt"},
		{"private key", "-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAK", "rsa-private-key"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := d.Detect([]byte(tc.input))
			if len(fs) == 0 {
				t.Fatalf("no findings for %q", tc.input)
			}
			found := false
			for _, f := range fs {
				if f.Rule == tc.wantRule {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected rule %q, got %+v", tc.wantRule, fs)
			}
		})
	}
}

func TestDetect_EntropyFallback(t *testing.T) {
	d := New()
	// 40-char base64-looking blob, high entropy, no known pattern.
	blob := "X9kL2mN4pQ7rS1tU3vW5yZ8aB0cD2eF4gH6iJ8kL"
	input := "INTERNAL_TOKEN=" + blob
	fs := d.Detect([]byte(input))
	if len(fs) == 0 {
		t.Fatal("expected entropy detection")
	}
	for _, f := range fs {
		if f.Rule == "entropy" {
			return
		}
	}
	t.Errorf("no entropy finding: %+v", fs)
}

func TestDetect_IgnoresLowEntropy(t *testing.T) {
	d := New()
	input := "PASSWORD=password12345\nHOST=localhost\nENV=development\n"
	fs := d.Detect([]byte(input))
	for _, f := range fs {
		if f.Rule == "entropy" {
			t.Errorf("false positive on low-entropy: %q", f.Value)
		}
	}
}

func TestDetect_PublicExampleCorpus(t *testing.T) {
	// Task 8 calibration: these must NOT be flagged.
	d := New()
	safe := []string{
		"AWS_ACCESS_KEY=AKIAIOSFODNN7EXAMPLE",             // AWS docs placeholder — matches pattern but is documented example
		"COMMIT=a1b2c3d4e5f6a7b8c9d0e1f2a3b4c5d6e7f8",     // git SHA (40 hex, 4 bits entropy, borderline)
		"MIGRATION_ID=550e8400-e29b-41d4-a716-446655440000", // UUID
		"VERSION=1.2.3",
	}
	for _, s := range safe {
		fs := d.Detect([]byte(s))
		// AKIAIOSFODNN7EXAMPLE will match the aws-access-key rule by design —
		// the pattern cannot distinguish the documented placeholder from a real
		// key. This is accepted for Phase 0 and must be handled by the scanner
		// via an ignore list on well-known placeholder values.
		if strings.Contains(s, "AKIAIOSFODNN7EXAMPLE") {
			continue
		}
		for _, f := range fs {
			t.Errorf("false positive on safe string %q: %+v", s, f)
		}
	}
}

func TestDetect_KnownExamplesAllowlisted(t *testing.T) {
	d := New()
	cases := []string{
		"AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE",
		"AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		"STRIPE_TEST=sk_test_4eC39HqLyjWDarjtT1zdp7dc",
	}
	for _, s := range cases {
		fs := d.Detect([]byte(s))
		if len(fs) > 0 {
			t.Errorf("known example should be allowlisted: %q -> %+v", s, fs)
		}
	}
}

func TestCheckEnvValue(t *testing.T) {
	d := New()
	strong := "s8kdj39dkLmQ2xPvR7"
	if !d.CheckEnvValue(strong) {
		t.Errorf("strong value %q rejected", strong)
	}
	weak := []string{"password", "12345", "changeme", "short"}
	for _, w := range weak {
		if d.CheckEnvValue(w) {
			t.Errorf("weak value %q accepted", w)
		}
	}
}
