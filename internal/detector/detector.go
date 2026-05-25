// Package detector runs the secret detection pipeline over text content.
//
// In-file pipeline (PRD §7.2 stages 1–2):
//  1. Known pattern matching via built-in rules, filtered by the
//     public-placeholder allowlist (rules.IsKnownExample).
//  2. Shannon-entropy analysis for high-entropy strings missed by patterns,
//     gated by charset diversity and the integrity-prefix skip.
//
// Stage 3 from the PRD (cross-reference against .env values) lives in the
// scanner package, because it needs filesystem context the detector does
// not have.
package detector

import (
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/Musubi42/shhh/internal/rules"
)

// Finding is a single detected secret with enough context to label it.
type Finding struct {
	Value        string
	Rule         string
	Label        string
	PublicPrefix string
	// StructuralDesc, if non-empty, carries a structural public
	// description of the value (e.g., "admin@prod-db:5432/myapp" for a
	// postgres URL) that overrides the default `PublicPrefix...`
	// rendering when the session map builds a placeholder. Populated
	// by rules that provide a Normalize hook.
	StructuralDesc string
	Description    string
	Start, End     int
}

// Detector runs the detection pipeline.
type Detector struct {
	rules             []rules.Rule
	entropyThreshold  float64
	entropyMinLen     int
	entropyDenylist   map[string]struct{}
	integrityPrefixes []string
	// tokenRe splits candidate high-entropy tokens out of free text.
	tokenRe *regexp.Regexp
}

// New returns a detector with the built-in rules and default thresholds.
func New() *Detector {
	return &Detector{
		rules:            rules.Builtins(),
		entropyThreshold: 4.5,
		entropyMinLen:    20,
		entropyDenylist: map[string]struct{}{
			"password":  {},
			"changeme":  {},
			"localhost": {},
			"example":   {},
			"12345":     {},
			"secret":    {},
		},
		// Tokens for the entropy fallback. We deliberately exclude `_` and `=`
		// so that a line like `PREVIOUS_COMMIT=f1e2...` tokenizes into
		// `PREVIOUS`, `COMMIT`, `f1e2...` rather than one long concatenated
		// string whose mixed charset inflates entropy beyond the threshold.
		// Pattern rules already catch real secrets by prefix; the entropy
		// fallback is for values that appear in isolation.
		tokenRe: regexp.MustCompile(`[A-Za-z0-9+/\-]{16,}`),
		// Integrity prefixes from npm/package-lock.json and Subresource
		// Integrity. These are high-entropy base64 by design but are public
		// content hashes, not secrets.
		integrityPrefixes: []string{"sha1-", "sha256-", "sha384-", "sha512-"},
	}
}

// Detect scans content and returns non-overlapping findings sorted by offset.
func (d *Detector) Detect(content []byte) []Finding {
	text := string(content)
	var findings []Finding

	// Stage 1: known patterns. Any pattern match whose value is on the
	// public-placeholder allowlist (e.g., AKIAIOSFODNN7EXAMPLE from AWS
	// docs) is dropped.
	for _, r := range d.rules {
		matches := r.Pattern.FindAllStringIndex(text, -1)
		for _, m := range matches {
			value := text[m[0]:m[1]]
			if rules.IsKnownExample(value) {
				continue
			}
			var structural string
			if r.Normalize != nil {
				structural = r.Normalize(value)
			}
			findings = append(findings, Finding{
				Value:          value,
				Rule:           r.Name,
				Label:          r.Label,
				PublicPrefix:   r.PublicPrefix,
				StructuralDesc: structural,
				Description:    r.Description,
				Start:          m[0],
				End:            m[1],
			})
		}
	}

	// Stage 2: entropy analysis on tokens not already matched.
	covered := coverage(findings)
	tokenMatches := d.tokenRe.FindAllStringIndex(text, -1)
	for _, m := range tokenMatches {
		if overlaps(covered, m[0], m[1]) {
			continue
		}
		token := text[m[0]:m[1]]
		if len(token) < d.entropyMinLen {
			continue
		}
		if _, deny := d.entropyDenylist[strings.ToLower(token)]; deny {
			continue
		}
		if d.hasIntegrityPrefix(token) {
			continue
		}
		if rules.IsKnownExample(token) {
			continue
		}
		// Path-like guard: tokens with ≥3 forward slashes are almost
		// always filesystem paths or URL path segments, not secrets.
		// Real secrets (base64, JWT, API keys) contain at most 1–2
		// slashes even in worst-case base64 encoding. Without this
		// guard, comment blocks listing source files trip the entropy
		// fallback — e.g. `docs/design/vault/mockups/DESIGN-NOTES`.
		if strings.Count(token, "/") >= 3 {
			continue
		}
		e := shannonEntropy(token)
		if e < d.entropyThreshold {
			continue
		}
		// A purely-hex token (charset ≤ 16) has at most 4.0 bits/char of
		// entropy; values like git SHAs, hex-encoded hashes, and UUIDs fall
		// here. They are reliably public identifiers and explicitly should
		// not fire the entropy detector. The charset gate below requires at
		// least 18 distinct characters for an entropy match, which rejects
		// hex (16) and UUID-with-hyphen (17) without hurting base64 (64).
		if distinctChars(token) < 18 {
			continue
		}
		findings = append(findings, Finding{
			Value:        token,
			Rule:         "entropy",
			Label:        "HIGH_ENTROPY",
			PublicPrefix: "",
			Description:  "High-entropy string",
			Start:        m[0],
			End:          m[1],
		})
	}

	sort.SliceStable(findings, func(i, j int) bool {
		return findings[i].Start < findings[j].Start
	})
	return dedupe(findings)
}

// CheckEnvValue applies the length and entropy gates to a candidate value from a
// .env file before using it as a cross-reference match target. Returns true if
// the value is strong enough to treat as a potential secret.
func (d *Detector) CheckEnvValue(v string) bool {
	if len(v) < 12 {
		return false
	}
	if _, deny := d.entropyDenylist[strings.ToLower(v)]; deny {
		return false
	}
	return shannonEntropy(v) >= 3.0
}

// hasIntegrityPrefix reports whether a token begins with a well-known
// Subresource Integrity hash prefix (sha1-, sha256-, etc.). Used to skip
// package-lock.json integrity fields and SRI attributes.
func (d *Detector) hasIntegrityPrefix(token string) bool {
	for _, p := range d.integrityPrefixes {
		if strings.HasPrefix(token, p) {
			return true
		}
	}
	return false
}

// distinctChars returns the number of distinct runes in s.
func distinctChars(s string) int {
	seen := make(map[rune]struct{}, len(s))
	for _, r := range s {
		seen[r] = struct{}{}
	}
	return len(seen)
}

// shannonEntropy returns bits/char for a string.
func shannonEntropy(s string) float64 {
	if s == "" {
		return 0
	}
	counts := make(map[rune]int, len(s))
	for _, r := range s {
		counts[r]++
	}
	n := float64(len(s))
	var h float64
	for _, c := range counts {
		p := float64(c) / n
		h -= p * math.Log2(p)
	}
	return h
}

// coverage returns the set of [start,end) intervals already occupied by
// findings, for overlap checks.
func coverage(fs []Finding) [][2]int {
	iv := make([][2]int, 0, len(fs))
	for _, f := range fs {
		iv = append(iv, [2]int{f.Start, f.End})
	}
	return iv
}

func overlaps(iv [][2]int, s, e int) bool {
	for _, r := range iv {
		if s < r[1] && r[0] < e {
			return true
		}
	}
	return false
}

// dedupe removes findings with identical spans. Pattern rules are appended
// before entropy matches and Detect sorts stably by start offset, so when
// both a rule match and an entropy match share a span the rule match comes
// first and wins.
func dedupe(fs []Finding) []Finding {
	if len(fs) == 0 {
		return fs
	}
	out := fs[:0]
	var prev Finding
	for i, f := range fs {
		if i == 0 {
			out = append(out, f)
			prev = f
			continue
		}
		if f.Start == prev.Start && f.End == prev.End {
			continue
		}
		out = append(out, f)
		prev = f
	}
	return out
}
