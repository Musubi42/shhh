// Package detector runs the secret detection pipeline over text content.
//
// The pipeline has three stages, matching PRD §7.2:
//  1. Known pattern matching via built-in rules
//  2. Shannon-entropy analysis for high-entropy strings missed by patterns
//  3. (cross-reference is handled by the scanner, not the detector)
package detector

import (
	"math"
	"regexp"
	"sort"
	"strings"

	"github.com/musubi-sasu/shhh/internal/rules"
)

// Finding is a single detected secret with enough context to label it.
type Finding struct {
	Value        string
	Rule         string
	Label        string
	PublicPrefix string
	Description  string
	Start, End   int
}

// Detector runs the detection pipeline.
type Detector struct {
	rules            []rules.Rule
	entropyThreshold float64
	entropyMinLen    int
	entropyDenylist  map[string]struct{}
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
		tokenRe: regexp.MustCompile(`[A-Za-z0-9+/=_\-]{16,}`),
	}
}

// Detect scans content and returns non-overlapping findings sorted by offset.
func (d *Detector) Detect(content []byte) []Finding {
	text := string(content)
	var findings []Finding

	// Stage 1: known patterns.
	for _, r := range d.rules {
		matches := r.Pattern.FindAllStringIndex(text, -1)
		for _, m := range matches {
			findings = append(findings, Finding{
				Value:        text[m[0]:m[1]],
				Rule:         r.Name,
				Label:        r.Label,
				PublicPrefix: r.PublicPrefix,
				Description:  r.Description,
				Start:        m[0],
				End:          m[1],
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
		e := shannonEntropy(token)
		if e < d.entropyThreshold {
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

	sort.Slice(findings, func(i, j int) bool {
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

// dedupe removes findings with identical spans. When a span has both a rule
// match and an entropy match, the rule match wins (it's earlier in the slice
// after sort-stable behavior on equal keys).
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
