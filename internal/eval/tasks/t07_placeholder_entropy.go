// Package tasks contains concrete eval task implementations (PRD §10 Phase 0
// task catalog).
package tasks

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"

	"github.com/musubi-sasu/shhh/internal/eval"
)

// PlaceholderEntropy is task 7: placeholder distinguishability. A direct
// cryptographic measurement, not an agent run.
//
// Procedure: generate N real Stripe-shaped keys, redact each in a fresh
// session, and measure how much residual information exists in the placeholder
// beyond the public prefix.
//
// The pass criterion is: for N distinct real values, if the redactor is
// working correctly, each placeholder must differ from the others (otherwise
// placeholders collide and compare_secrets produces false positives) AND no
// placeholder must contain any characters of the real value beyond the public
// prefix (otherwise the placeholder leaks bits).
type PlaceholderEntropy struct {
	N int
}

// NewPlaceholderEntropy returns the task with a default sample size.
func NewPlaceholderEntropy() *PlaceholderEntropy {
	return &PlaceholderEntropy{N: 100}
}

func (t *PlaceholderEntropy) ID() string    { return "t07-placeholder-entropy" }
func (t *PlaceholderEntropy) Title() string { return "Placeholder distinguishability" }
func (t *PlaceholderEntropy) Tier() eval.Tier {
	return eval.Tier2
}

func (t *PlaceholderEntropy) SupportedModes() []eval.Mode {
	// Mode-agnostic: measurement is over the redact path alone.
	return []eval.Mode{eval.ModeRedact}
}

func (t *PlaceholderEntropy) Run(r eval.Redactor, mode eval.Mode) eval.Result {
	// Generate N distinct real values. We use a well-formed Stripe live key
	// shape so that shhh's pattern rule will fire and produce a labeled
	// placeholder rather than a generic entropy placeholder.
	values := make([]string, t.N)
	for i := 0; i < t.N; i++ {
		values[i] = genStripeKey()
	}

	// Redact each value in a FRESH session. This is the adversarial model:
	// an attacker observing placeholders from many users cannot use the
	// session-salted suffix to correlate users.
	placeholders := make([]string, t.N)
	for i, v := range values {
		sess := r.NewSession()
		redacted, _ := r.Redact(sess, []byte(v))
		placeholders[i] = string(redacted)
	}

	// Check 1: all placeholders must be distinct. Collisions across sessions
	// would let an attacker bucket users by secret.
	seen := make(map[string]int, t.N)
	collisions := 0
	for i, p := range placeholders {
		if j, ok := seen[p]; ok {
			collisions++
			_ = j
		}
		seen[p] = i
	}

	// Check 2: no placeholder should contain any characters of the real
	// secret value beyond the public prefix "sk_live_". We check for the
	// value-specific suffix (the 24 chars after the prefix) in the
	// placeholder.
	leaks := 0
	for i, v := range values {
		valueBody := v[len("sk_live_"):]
		if containsAny(placeholders[i], valueBody, 6) {
			leaks++
		}
	}

	metrics := map[string]string{
		"samples":            itoa(t.N),
		"distinct_placeholders": itoa(len(seen)),
		"collisions":         itoa(collisions),
		"value_body_leaks":   itoa(leaks),
	}

	if collisions > 0 {
		return eval.FailResult(
			fmt.Sprintf("%d placeholder collisions across %d samples", collisions, t.N),
			metrics,
		)
	}
	if leaks > 0 {
		return eval.FailResult(
			fmt.Sprintf("%d placeholders leaked value bits beyond public prefix", leaks),
			metrics,
		)
	}
	return eval.PassResult(metrics)
}

// genStripeKey returns a well-formed Stripe live secret key with 24 random
// characters in the body. The format matches shhh's stripe-live-key rule so
// the redactor will label it correctly.
func genStripeKey() string {
	b := make([]byte, 12)
	_, _ = rand.Read(b)
	return "sk_live_" + hex.EncodeToString(b) // 24-char hex body
}

// containsAny reports whether placeholder contains any contiguous substring
// of the real value body that is at least minLen characters long. This is a
// proxy for "did the placeholder leak bits of the value." A more rigorous
// test would measure bit-for-bit mutual information; this is sufficient for
// the Phase 0 bar.
func containsAny(placeholder, valueBody string, minLen int) bool {
	if minLen > len(valueBody) {
		return false
	}
	for i := 0; i+minLen <= len(valueBody); i++ {
		needle := valueBody[i : i+minLen]
		if indexString(placeholder, needle) >= 0 {
			return true
		}
	}
	return false
}

// indexString is a small local implementation of strings.Index to keep this
// file dependency-free.
func indexString(haystack, needle string) int {
	if len(needle) == 0 {
		return 0
	}
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	neg := false
	if n < 0 {
		neg = true
		n = -n
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
