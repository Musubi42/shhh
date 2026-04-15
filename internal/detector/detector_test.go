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
		{"aws access", `AWS_ACCESS_KEY=AKIA3EXAMPLE7XYZABC4`, "aws-access-key"},
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

// TestDetect_IgnoresPathLikeTokens pins the fix for a CSS-header false
// positive. A comment listing source files tripped HIGH_ENTROPY on
// docs/design/vault/mockups/DESIGN-NOTES because / is in the entropy
// token charset and the path cleared 4.5 bit/char. The guard skips
// tokens with 3+ slashes: rare in real base64 secrets, common in
// filesystem paths.
func TestDetect_IgnoresPathLikeTokens(t *testing.T) {
	d := New()
	input := `/* Eurio prototype -- design tokens
 *   docs/design/onboarding/mockups/DESIGN-NOTES.md
 *   docs/design/scan/mockups/DESIGN-NOTES.md
 *   docs/design/coin-detail/mockups/DESIGN-NOTES.md
 *   docs/design/vault/mockups/DESIGN-NOTES.md
 *   docs/design/profile/mockups/DESIGN-NOTES.md
 */`
	fs := d.Detect([]byte(input))
	for _, f := range fs {
		if f.Rule == "entropy" {
			t.Errorf("false positive on path-like token: %q", f.Value)
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

// TestDetect_GitleaksDerivedRules pins the rules transcribed from
// gitleaks in entry 10. One fixture per rule. If any fixture stops
// matching its rule, the rule was broken by a later edit.
func TestDetect_GitleaksDerivedRules(t *testing.T) {
	d := New()
	cases := []struct {
		name  string
		input string
		rule  string
	}{
		{"github-app-token", "GITHUB=ghs_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "github-app-token"},
		{"github-refresh-token", "GHR=ghr_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "github-refresh-token"},
		{"github-fine-grained-pat", "GHPT=github_pat_" + strings.Repeat("A", 82), "github-fine-grained-pat"},
		{"gitlab-pat", "GL=glpat-aaaaaaaaaaaaaaaaaaaa", "gitlab-pat"},
		{"gitlab-ptt", "GLPTT=glptt-" + strings.Repeat("a", 40), "gitlab-ptt"},
		{"slack-app-token", "SLACK=xapp-1-ABCDEFG-12345-abcdefghijklmn", "slack-app-token"},
		{"slack-webhook-url", "URL=https://hooks.slack.com/services/" + strings.Repeat("A", 44), "slack-webhook-url"},
		{"1password-secret-key", "OP=A3-ABCDEF-ABCDEFGHIJK-ABCDE-ABCDE-ABCDE", "1password-secret-key"},
		{"age-secret-key", "AGE=AGE-SECRET-KEY-1" + strings.Repeat("Q", 58), "age-secret-key"},
		{"npm-access-token", "NPM=npm_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa", "npm-access-token"},
		{"pypi-upload-token", "PYPI=pypi-AgEIcHlwaS5vcmc" + strings.Repeat("a", 60), "pypi-upload-token"},
		{"sendgrid-api-token", "SG=SG." + strings.Repeat("A", 22) + "." + strings.Repeat("B", 43), "sendgrid-api-token"},
		{"databricks-api-token", "DB=dapi" + strings.Repeat("a", 32), "databricks-api-token"},
		{"digitalocean-pat", "DO=dop_v1_" + strings.Repeat("a", 64), "digitalocean-pat"},
		{"digitalocean-access-token", "DO=doo_v1_" + strings.Repeat("a", 64), "digitalocean-access-token"},
		{"doppler-api-token", "DOPPLER=dp.pt." + strings.Repeat("a", 43), "doppler-api-token"},
		{"huggingface-access-token", "HF=hf_" + strings.Repeat("a", 34), "huggingface-access-token"},
		{"linear-api-key", "LIN=lin_api_" + strings.Repeat("a", 40), "linear-api-key"},
		{"notion-api-token", "NOT=ntn_" + strings.Repeat("1", 11) + strings.Repeat("A", 35), "notion-api-token"},
		{"postman-api-token", "PM=PMAK-" + strings.Repeat("a", 24) + "-" + strings.Repeat("b", 34), "postman-api-token"},
		{"shopify-access-token", "SH=shpat_" + strings.Repeat("a", 32), "shopify-access-token"},
		{"shopify-shared-secret", "SH=shpss_" + strings.Repeat("a", 32), "shopify-shared-secret"},
		{"vault-service-token", "V=hvs." + strings.Repeat("a", 100), "vault-service-token"},
		{"grafana-service-account", "GF=glsa_" + strings.Repeat("a", 32) + "_abcdef12", "grafana-service-account-token"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := d.Detect([]byte(tc.input))
			if len(fs) == 0 {
				t.Fatalf("no findings for %q", tc.input)
			}
			for _, f := range fs {
				if f.Rule == tc.rule {
					return
				}
			}
			t.Errorf("expected rule %q, got %+v", tc.rule, fs)
		})
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
