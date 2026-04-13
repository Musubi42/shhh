// Package redactor implements the redact/rehydrate cycle described in PRD §5.
//
// Redact runs the detector over content and replaces every finding with a
// session-deterministic placeholder. Rehydrate walks the text looking for
// placeholders and substitutes back the real values from the session map.
// Rehydration fails closed: unknown placeholders are left untouched.
package redactor

import (
	"sort"
	"strings"

	"github.com/musubi-sasu/shhh/internal/detector"
	"github.com/musubi-sasu/shhh/internal/session"
)

// Redactor is the stateful pair of (detector, session map) used for one agent
// session. Multiple Redact calls with the same Redactor produce consistent
// placeholders for the same values.
type Redactor struct {
	det  *detector.Detector
	sess *session.Map
}

// New wires a detector and a session map together.
func New(det *detector.Detector, sess *session.Map) *Redactor {
	return &Redactor{det: det, sess: sess}
}

// Redact replaces every detected secret in content with a placeholder. Returns
// the redacted bytes and the list of findings (for logging and scan reports).
func (r *Redactor) Redact(content []byte) ([]byte, []detector.Finding) {
	findings := r.det.Detect(content)
	if len(findings) == 0 {
		return content, nil
	}

	// Walk findings in reverse order and splice in placeholders so the offsets
	// of earlier findings stay valid.
	sort.Slice(findings, func(i, j int) bool {
		return findings[i].Start > findings[j].Start
	})

	out := make([]byte, len(content))
	copy(out, content)
	for _, f := range findings {
		placeholder := r.sess.PlaceholderFor(f.Value, f.Label, f.PublicPrefix)
		out = append(append(out[:f.Start:f.Start], []byte(placeholder)...), out[f.End:]...)
	}

	// Return findings in original order for downstream callers.
	sort.Slice(findings, func(i, j int) bool {
		return findings[i].Start < findings[j].Start
	})
	return out, findings
}

// Rehydrate walks content looking for placeholders in the session map and
// substitutes them back with the real values. Placeholders not in the map are
// left unchanged (fail-closed).
func (r *Redactor) Rehydrate(content []byte) []byte {
	text := string(content)
	return []byte(session.PlaceholderRe.ReplaceAllStringFunc(text, func(match string) string {
		if v, ok := r.sess.Resolve(match); ok {
			return v
		}
		return match
	}))
}

// RedactString is a convenience wrapper for string inputs.
func (r *Redactor) RedactString(s string) (string, []detector.Finding) {
	b, f := r.Redact([]byte(s))
	return string(b), f
}

// RehydrateString is a convenience wrapper for string inputs.
func (r *Redactor) RehydrateString(s string) string {
	return string(r.Rehydrate([]byte(s)))
}

// ContainsPlaceholder reports whether s has any shhh-shaped placeholder. Used by
// tests and by the proxy's inbound rehydration fast-path.
func ContainsPlaceholder(s string) bool {
	return strings.Contains(s, "[") && session.PlaceholderRe.MatchString(s)
}
