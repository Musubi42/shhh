// Package redactor implements the redact/rehydrate cycle described in PRD §5.
//
// Redact runs the detector over content and replaces every finding with a
// session-deterministic placeholder. Rehydrate walks the text looking for
// placeholders and substitutes back the real values from the session map.
// Rehydration fails closed: unknown placeholders are left untouched.
package redactor

import (
	"regexp"
	"sort"
	"strings"

	"github.com/Musubi42/shhh/internal/detector"
	"github.com/Musubi42/shhh/internal/session"
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
	return r.spliceFindings(content, findings), findings
}

// RedactEnvFile runs the regular detector pass and then adds a second,
// env-aware pass for KEY=VALUE lines whose VALUE passes the looser
// .env-context strength gate (detector.CheckEnvValue). This catches short,
// high-quality random tokens that sit below the generic entropy threshold
// calibrated for arbitrary source code — the kind of value a developer
// parks in .env under a custom name like INTERNAL_LEGACY_TOKEN.
//
// Only call this for files that are known to be env-value stores (.env,
// .env.local, .envrc, etc.). Running it on arbitrary source code would
// over-redact because the KEY=VALUE shape shows up everywhere.
func (r *Redactor) RedactEnvFile(content []byte) ([]byte, []detector.Finding) {
	findings := r.det.Detect(content)
	envFindings := r.envValueFindings(content, findings)
	all := append(findings, envFindings...)
	return r.spliceFindings(content, all), all
}

// envLineRe captures KEY and VALUE from a dotenv-style line. Anchored to
// line start so it doesn't match `export FOO=bar` mid-sentence; leading
// `export ` is allowed. VALUE runs to end of line and gets trimmed for
// quotes/whitespace downstream.
var envLineRe = regexp.MustCompile(`(?m)^[ \t]*(?:export[ \t]+)?([A-Za-z_][A-Za-z0-9_]*)[ \t]*=[ \t]*(.*)$`)

// envValueFindings walks the content line by line and emits a Finding for
// each KEY=VALUE where VALUE is a strong-enough env value and isn't already
// covered by a generic-detector finding.
func (r *Redactor) envValueFindings(content []byte, existing []detector.Finding) []detector.Finding {
	text := string(content)
	var out []detector.Finding
	covered := make([][2]int, 0, len(existing))
	for _, f := range existing {
		covered = append(covered, [2]int{f.Start, f.End})
	}

	for _, m := range envLineRe.FindAllStringSubmatchIndex(text, -1) {
		// m[2]:m[3] = KEY span, m[4]:m[5] = VALUE span (unquoted,
		// raw to end of line, possibly with a trailing comment).
		key := text[m[2]:m[3]]
		rawVal := text[m[4]:m[5]]

		valStart, valEnd, value := trimEnvValue(m[4], rawVal)
		if value == "" {
			continue
		}
		// Skip lines where the value is already a shhh placeholder so we
		// don't re-redact a fixture or a file that was partially processed.
		if session.PlaceholderRe.MatchString(value) {
			continue
		}
		// Skip if the value overlaps an existing finding (pattern or
		// entropy detector already caught it).
		skip := false
		for _, c := range covered {
			if valStart < c[1] && c[0] < valEnd {
				skip = true
				break
			}
		}
		if skip {
			continue
		}
		if !r.det.CheckEnvValue(value) {
			continue
		}
		out = append(out, detector.Finding{
			Value:       value,
			Rule:        "env-value",
			Label:       sanitizeEnvLabel(key),
			Description: "Value from a .env file (env-aware pass)",
			Start:       valStart,
			End:         valEnd,
		})
	}
	return out
}

// trimEnvValue strips surrounding whitespace, matching quotes, and a
// trailing `# comment` from a raw dotenv value, returning the value's
// absolute span in the source plus the cleaned string. If the cleaned
// value is empty it returns zeros.
func trimEnvValue(absStart int, raw string) (int, int, string) {
	// Trim leading whitespace, advancing absStart in lockstep.
	i := 0
	for i < len(raw) && (raw[i] == ' ' || raw[i] == '\t') {
		i++
	}
	absStart += i
	raw = raw[i:]

	// Trim trailing whitespace.
	j := len(raw)
	for j > 0 && (raw[j-1] == ' ' || raw[j-1] == '\t' || raw[j-1] == '\r') {
		j--
	}
	raw = raw[:j]

	if raw == "" {
		return 0, 0, ""
	}

	// If fully quoted, strip the quotes and return the inner span. If not,
	// drop a trailing unquoted `# comment`.
	if len(raw) >= 2 && (raw[0] == '"' || raw[0] == '\'') && raw[len(raw)-1] == raw[0] {
		inner := raw[1 : len(raw)-1]
		return absStart + 1, absStart + 1 + len(inner), inner
	}
	if idx := strings.Index(raw, " #"); idx >= 0 {
		raw = strings.TrimRight(raw[:idx], " \t")
	}
	if raw == "" {
		return 0, 0, ""
	}
	return absStart, absStart + len(raw), raw
}

// sanitizeEnvLabel normalizes a dotenv key into the label alphabet
// allowed by session.PlaceholderRe (`[A-Z_]+`). Non-matching characters
// are dropped and digits collapsed.
func sanitizeEnvLabel(key string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(key) {
		if (r >= 'A' && r <= 'Z') || r == '_' {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "ENV_SECRET"
	}
	return b.String()
}

// spliceFindings replaces each finding's span in content with a
// session-assigned placeholder and returns the rewritten bytes. Input
// findings may be in any order.
func (r *Redactor) spliceFindings(content []byte, findings []detector.Finding) []byte {
	if len(findings) == 0 {
		return content
	}
	// Drop overlaps (later pass feeding in finds that happen to overlap
	// an earlier one). Keep the earliest-starting, longest finding.
	findings = dedupeOverlap(findings)

	sort.Slice(findings, func(i, j int) bool {
		return findings[i].Start > findings[j].Start
	})

	out := make([]byte, len(content))
	copy(out, content)
	for _, f := range findings {
		placeholder := r.sess.PlaceholderFor(f.Value, f.Label, f.PublicPrefix, f.StructuralDesc)
		out = append(append(out[:f.Start:f.Start], []byte(placeholder)...), out[f.End:]...)
	}
	return out
}

// dedupeOverlap removes findings whose span overlaps an earlier (by start,
// then by longer span) finding. Used to make the multi-pass merge in
// RedactEnvFile safe against the env pass re-flagging a value the generic
// detector already found via a different span.
func dedupeOverlap(findings []detector.Finding) []detector.Finding {
	if len(findings) < 2 {
		return findings
	}
	sorted := make([]detector.Finding, len(findings))
	copy(sorted, findings)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Start != sorted[j].Start {
			return sorted[i].Start < sorted[j].Start
		}
		return (sorted[i].End - sorted[i].Start) > (sorted[j].End - sorted[j].Start)
	})
	out := sorted[:0]
	lastEnd := -1
	for _, f := range sorted {
		if f.Start < lastEnd {
			continue
		}
		out = append(out, f)
		if f.End > lastEnd {
			lastEnd = f.End
		}
	}
	return out
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

// Resolve returns the real value behind a single placeholder in this
// redactor's session map, or ("", false) on miss (fail-closed). This is the
// primitive that compensatory tools need — they look up one placeholder at a
// time rather than rehydrating a whole content buffer.
func (r *Redactor) Resolve(placeholder string) (string, bool) {
	return r.sess.Resolve(placeholder)
}

// PlaceholderForFinding returns the placeholder this redactor's session map
// will assign (or already assigned) to a given finding. Idempotent per
// session. Used by the hook narration layer to show users which placeholder
// corresponds to which secret, without having to reparse the redacted output.
func (r *Redactor) PlaceholderForFinding(f detector.Finding) string {
	return r.sess.PlaceholderFor(f.Value, f.Label, f.PublicPrefix, f.StructuralDesc)
}
