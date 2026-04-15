package tasks

import (
	"encoding/base64"
	"strings"

	"github.com/musubi-sasu/shhh/internal/eval"
)

// Consistency is task 3: cross-file identity for a single secret that
// appears in three different representations:
//
//  1. Plaintext in a .env file           (API_KEY=sk_live_…)
//  2. Hardcoded in a Go source constant  (const apiKey = "sk_live_…")
//  3. Base64-encoded in a Kubernetes Secret manifest (data: api-key: …)
//
// All three files are fed through the redactor in the same session. The
// simulated agent extracts the value observable at each site and asks:
// "are these three the same underlying secret?"
//
// Expected behavior per mode:
//
//   - no-redaction: the agent sees raw content at all three sites. With
//     base64 decoding available (a baseline agent can do it mentally)
//     the three values reduce to one, and the agent concludes "same." PASS.
//   - redact: sites 1 and 2 redact to the *same* placeholder because the
//     session map deduplicates by byte-identity and the two copies are
//     byte-identical. Site 3 redacts to a *different* placeholder because
//     its base64 form is a different byte string. String equality fails
//     on the 1-vs-3 comparison. FAIL (by design).
//   - redact+rehydrate: rehydration operates on tool_use args, not on
//     content the model is reasoning about, so the agent is still looking
//     at placeholders. Same result as redact. FAIL (by design).
//   - redact+compensatory: the agent calls `compare_secrets`, which
//     resolves both placeholders through the session map and applies the
//     cross-representation equality rule (try base64 decode either way).
//     The 1-vs-3 comparison now succeeds. PASS.
//
// The critical lesson task 3 teaches is whether the detector needs a
// canonicalization pass before map lookup, or whether the compensatory
// tool alone is enough. Phase-0 answer: compensatory tool is enough for
// this subcase, because the detector has no reliable way to know that a
// random 54-char base64 blob is the encoded form of some *other* value
// already seen in the session without maintaining a cross-reference map
// the detector does not own. Pushing the knowledge into the tool keeps
// detection stateless.
type Consistency struct{}

func NewConsistency() *Consistency { return &Consistency{} }

func (t *Consistency) ID() string    { return "t03-consistency" }
func (t *Consistency) Title() string { return "Consistency across files (plain + base64 + env)" }
func (t *Consistency) Tier() eval.Tier {
	return eval.Tier1
}

func (t *Consistency) SupportedModes() []eval.Mode {
	return []eval.Mode{
		eval.ModeNoRedaction,
		eval.ModeRedact,
		eval.ModeRedactRehydrate,
		eval.ModeRedactRehydrateCompen,
	}
}

func (t *Consistency) Expected(mode eval.Mode) eval.Expected {
	switch mode {
	case eval.ModeNoRedaction, eval.ModeRedactRehydrateCompen:
		return eval.ExpectedPass
	case eval.ModeRedact, eval.ModeRedactRehydrate:
		return eval.ExpectedFail
	}
	return eval.ExpectedPass
}

// consistencySecret is a realistic-looking Stripe live key that hits the
// stripe-live-key pattern rule in its raw form and the entropy detector
// in its base64 form. Body length (32 chars) keeps it above the
// `{24,}` minimum; total length (40 bytes) gives a 54-char base64
// representation with enough charset diversity to clear the 18-distinct
// gate.
const consistencySecret = "sk_live_AbC7f9Kd2nE8xR4tL1qM0pH5sW3vZ6yB"

func (t *Consistency) Run(r eval.Redactor, mode eval.Mode) eval.Result {
	envContent := "API_KEY=" + consistencySecret + "\n"
	goContent := "package main\n\nconst apiKey = \"" + consistencySecret + "\"\n"

	// RawStdEncoding avoids padding so the base64 token has no trailing
	// `=`, which would otherwise complicate extraction because the
	// detector's token regex stops at `=` and leaves the padding in
	// place. The compensatory tool still accepts padded base64 (see
	// base64Equals in compensatory.go) so real kubectl output with `==`
	// padding would be handled at tool time.
	k8sBase64 := base64.RawStdEncoding.EncodeToString([]byte(consistencySecret))
	yamlContent := "apiVersion: v1\nkind: Secret\nmetadata:\n  name: api\ndata:\n  api-key: " + k8sBase64 + "\n"

	// One session across all three files. Cross-file identity requires
	// that the redactor see the three payloads as belonging to the same
	// placeholder scope.
	sess := r.NewSession()

	feed := func(content string) string {
		if mode == eval.ModeNoRedaction {
			return content
		}
		out, _ := r.Redact(sess, []byte(content))
		return string(out)
	}

	envView := feed(envContent)
	goView := feed(goContent)
	yamlView := feed(yamlContent)

	envVal := extractEnvValue(envView, "API_KEY")
	goVal := extractGoConst(goView, "apiKey")
	yamlVal := extractYAMLValue(yamlView, "api-key")

	if envVal == "" || goVal == "" || yamlVal == "" {
		return eval.FailResult(
			"agent could not extract one of the three values",
			map[string]string{"env": envVal, "go": goVal, "yaml": yamlVal},
		)
	}

	metrics := map[string]string{
		"env":  envVal,
		"go":   goVal,
		"yaml": yamlVal,
	}

	// Pick the equality function for this mode. In compensatory mode the
	// agent calls the tool; in every other mode it has only direct
	// comparison. Baseline can still succeed because it sees raw values
	// and can try base64 decoding on its own.
	var equal func(a, b string) bool
	switch mode {
	case eval.ModeRedactRehydrateCompen:
		tools := eval.NewCompensatoryTools(r, sess)
		equal = tools.CompareSecrets
	case eval.ModeNoRedaction:
		equal = rawEqualWithBase64
	default:
		// redact and redact+rehydrate: plain string compare on
		// placeholders. Designed to fail on the 1-vs-3 comparison.
		equal = func(a, b string) bool { return a == b }
	}

	if !equal(envVal, goVal) {
		return eval.FailResult("env vs go differ: "+envVal+" / "+goVal, metrics)
	}
	if !equal(envVal, yamlVal) {
		return eval.FailResult("env vs yaml differ: "+envVal+" / "+yamlVal, metrics)
	}
	if !equal(goVal, yamlVal) {
		return eval.FailResult("go vs yaml differ: "+goVal+" / "+yamlVal, metrics)
	}
	return eval.PassResult(metrics)
}

// rawEqualWithBase64 implements the baseline-mode equality an unassisted
// agent would perform on visible raw strings: byte equality first, then
// base64 decoding on either side. This is the work the compensatory tool
// does for the agent in redact+compensatory mode.
func rawEqualWithBase64(a, b string) bool {
	if a == b {
		return true
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(a); err == nil && string(decoded) == b {
		return true
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(b); err == nil && string(decoded) == a {
		return true
	}
	if decoded, err := base64.StdEncoding.DecodeString(a); err == nil && string(decoded) == b {
		return true
	}
	if decoded, err := base64.StdEncoding.DecodeString(b); err == nil && string(decoded) == a {
		return true
	}
	return false
}

// extractGoConst pulls VALUE from a `const NAME = "VALUE"` line. Simple
// enough for the fixture; not a real Go parser.
func extractGoConst(content, name string) string {
	needle := "const " + name + " = \""
	idx := strings.Index(content, needle)
	if idx < 0 {
		return ""
	}
	rest := content[idx+len(needle):]
	end := strings.Index(rest, "\"")
	if end < 0 {
		return ""
	}
	return rest[:end]
}

// extractYAMLValue pulls the value after `key:` on its own line. Strips
// surrounding whitespace. Sufficient for the single-line value the
// fixture uses; a real YAML parser is unnecessary.
func extractYAMLValue(content, key string) string {
	prefix := key + ":"
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, prefix))
		}
	}
	return ""
}
