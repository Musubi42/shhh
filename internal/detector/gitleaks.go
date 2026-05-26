package detector

import (
	"fmt"
	"regexp"
	"strings"

	gitleaksconfig "github.com/zricethezav/gitleaks/v8/config"
	"github.com/zricethezav/gitleaks/v8/detect"
	"github.com/zricethezav/gitleaks/v8/report"
)

// GitleaksBackend is the gitleaks-driven detection backend. It wraps
// gitleaks' library API and reshapes its `report.Finding` into our
// `Finding`. The mapping covers two responsibilities:
//
//  1. RuleID → shhh `Label` (the placeholder tag the agent sees).
//     A small explicit table maps gitleaks rules we recognize onto
//     our existing labels; everything else falls back to a derived
//     uppercase form.
//  2. Match → `Value` + `PublicPrefix`. Gitleaks tells us WHAT
//     matched; the public-prefix slice is derived locally so we
//     preserve our existing placeholder shape
//     (`[STRIPE_LIVE_KEY:sk_live_…:fingerprint]`).
//
// Structural normalization (postgres URL preserves host/db) is NOT
// done here — gitleaks doesn't surface that. Findings without a
// structural description fall back to the prefix-only placeholder.
// Closing that gap is a follow-up; see gitleaks-spike.md "Risks".
type GitleaksBackend struct {
	detector *detect.Detector
}

// NewGitleaks builds a backend with gitleaks' default rule set
// (currently 222 rules in v8.30.1). Returns an error if gitleaks
// fails to construct its detector, which we surface to the caller
// so the factory can fall back to the shhh-native backend gracefully.
func NewGitleaks() (*GitleaksBackend, error) {
	d, err := detect.NewDetectorDefaultConfig()
	if err != nil {
		return nil, fmt.Errorf("gitleaks: build detector: %w", err)
	}
	// We don't want gitleaks to log on its own — output ordering
	// belongs to shhh. Silence its global logger by setting a
	// no-op writer; gitleaks reads it via logging.Logger which is
	// internal, so the cleanest knob is to swallow via the
	// detector's verbose/log flags if exposed. v8.30.1 has no
	// such public flag yet, so we accept the occasional stderr
	// line from gitleaks until upstream exposes one. Tracked in
	// gitleaks-spike.md follow-ups.
	return &GitleaksBackend{detector: d}, nil
}

func (g *GitleaksBackend) Detect(content []byte) []Finding {
	if g == nil || g.detector == nil {
		return nil
	}
	// DetectString is the simplest entry point that returns
	// per-match findings with line numbers. We don't need the
	// streaming or fragment API for in-memory hook-sized content.
	gf := g.detector.DetectString(string(content))
	if len(gf) == 0 {
		return nil
	}
	out := make([]Finding, 0, len(gf))
	for _, f := range gf {
		// Gitleaks line numbers are 1-indexed; we keep them in
		// Start/End as character offsets the way the shhh-native
		// detector does. Gitleaks doesn't expose absolute offsets
		// directly, so recover them from the Match string.
		start := strings.Index(string(content), f.Match)
		end := start + len(f.Match)
		if start < 0 {
			start, end = 0, 0
		}
		out = append(out, Finding{
			Value:        f.Match,
			Rule:         "gitleaks:" + f.RuleID,
			Label:        mapGitleaksLabel(f.RuleID, f.Match),
			PublicPrefix: derivePublicPrefix(f.Match),
			Description:  f.Description,
			Start:        start,
			End:          end,
		})
	}
	return out
}

// mapGitleaksLabel translates a gitleaks RuleID into shhh's
// placeholder label vocabulary. Explicit entries keep the
// agent-facing narration consistent ("Claude saw a Stripe key, not
// `generic-api-key`"). Unmapped rules fall back to a derived
// uppercase form — coarser but better than empty.
//
// Maintenance: add a new row here whenever a gitleaks rule starts
// appearing in real session diffs (compare with `shhh bench`).
// Most of the 222 rules will never need an explicit mapping.
func mapGitleaksLabel(ruleID, match string) string {
	if l, ok := gitleaksLabelMap[ruleID]; ok {
		return l
	}
	// Heuristic fallback: convert "stripe-access-token" → "STRIPE_ACCESS_TOKEN".
	upper := strings.ToUpper(strings.ReplaceAll(ruleID, "-", "_"))
	return upper
}

// gitleaksLabelMap covers the ~30 gitleaks rules that overlap with
// shhh's existing labels. Keeping the labels aligned means the
// placeholder format is stable across the migration.
var gitleaksLabelMap = map[string]string{
	"stripe-access-token":      "STRIPE_LIVE_KEY",
	"aws-access-token":         "AWS_ACCESS_KEY",
	"github-pat":               "GITHUB_PAT",
	"github-app-token":         "GITHUB_APP_TOKEN",
	"github-fine-grained-pat":  "GITHUB_FINE_GRAINED_PAT",
	"github-oauth":             "GITHUB_OAUTH_TOKEN",
	"jwt":                      "JWT_TOKEN",
	"openai-api-key":           "OPENAI_API_KEY",
	"anthropic-api-key":        "ANTHROPIC_KEY",
	"slack-bot-token":          "SLACK_BOT_TOKEN",
	"slack-user-token":         "SLACK_USER_TOKEN",
	"slack-webhook-url":        "SLACK_WEBHOOK",
	"sendgrid-api-token":       "SENDGRID_API_KEY",
	"gcp-api-key":              "GCP_API_KEY",
	"google-api-key":           "GOOGLE_API_KEY",
	"private-key":              "PRIVATE_KEY",
	"rsa-private-key":          "RSA_PRIVATE_KEY",
	"ssh-private-key":          "SSH_PRIVATE_KEY",
	"npm-access-token":         "NPM_ACCESS_TOKEN",
	"pypi-upload-token":        "PYPI_TOKEN",
	"generic-api-key":          "GENERIC_API_KEY",
}

// derivePublicPrefix returns the first N "obvious public" chars of
// match for the placeholder display. For known vendor prefixes
// (sk_live_, ghp_, etc.) it preserves the full prefix; otherwise it
// takes a short slice. The placeholder format always ellipsizes
// after, so the public-prefix is purely for human-readable hinting.
func derivePublicPrefix(match string) string {
	prefixes := []string{
		"sk_live_", "sk_test_", "sk-proj-", "sk-ant-",
		"ghp_", "gho_", "ghu_", "ghs_", "ghr_",
		"AKIA", "ASIA",
		"xoxb-", "xoxa-", "xoxp-", "xoxe.",
		"npm_",
		"AIza", // Google API
		"-----BEGIN ",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(match, p) {
			return p
		}
	}
	// Short default — first 4 chars when the match is long enough.
	if len(match) >= 4 {
		return match[:4]
	}
	return match
}

// GitleaksDefaultAllowlistPaths returns the compiled path-regex
// allowlist that ships with the embedded gitleaks default config.
// `internal/ignore` uses this as its first layer so the
// `.shhhignore` cascade starts from the same lockfile/vendor/
// binary baseline as gitleaks itself — keeping shhh-native
// scanning aligned with gitleaks even when gitleaks isn't in the
// user's engine selection. See docs/engine-architecture.md §2.3.
//
// Returns nil + nil if the gitleaks detector fails to construct
// (rare). Callers should treat that as "no defaults" and continue
// with user-supplied layers.
func GitleaksDefaultAllowlistPaths() ([]*regexp.Regexp, error) {
	d, err := detect.NewDetectorDefaultConfig()
	if err != nil {
		return nil, fmt.Errorf("gitleaks defaults: %w", err)
	}
	var out []*regexp.Regexp
	for _, al := range d.Config.Allowlists {
		out = append(out, al.Paths...)
	}
	return out, nil
}

// _ ensures we keep the gitleaksconfig import referenced for
// future direct access to the rule set (e.g., to list rule IDs in
// a `shhh doctor --list-rules`-style command). Removing this
// later is harmless.
var _ = gitleaksconfig.Config{}

// _ ensures report.Finding is referenced for godoc cross-linking
// when this file gets expanded.
var _ = report.Finding{}
