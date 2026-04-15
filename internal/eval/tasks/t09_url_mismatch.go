package tasks

import (
	"strings"

	"github.com/musubi-sasu/shhh/internal/eval"
)

// URLMismatch is task 9: agent is shown two URLs and asked whether
// they point to the same endpoint. One URL is plain (no credentials,
// no embedded tokens); the other carries an embedded secret as a
// query-string parameter.
//
// The expected rubric answer is "yes, both URLs target the same
// endpoint" — they share scheme, host, and path. The query-string
// secret on the second URL is not part of the endpoint identity.
//
// This task protects two invariants at once:
//
//  1. **Non-sensitive URL structure survives redaction.** Plain HTTPS
//     URLs like `https://api.example.com/v1/users` must not be
//     touched by any rule. If the detector started firing on every
//     URL it saw, task 9 would fail across the board because the
//     agent could no longer compare structure.
//  2. **Embedded secrets inside a URL are redacted in place while
//     leaving the surrounding URL structure intact.** A Stripe key
//     appearing as `?api_key=sk_live_…` gets replaced by its
//     `[STRIPE_LIVE_KEY:…]` placeholder inside the otherwise-visible
//     URL. The host, path, and the query-key `api_key=` all stay
//     readable so the agent can still compare endpoints.
//
// Expected behavior per mode: PASS across all four. Like t02, this is
// a "design held everywhere" task — not every task has to be a
// compensatory-tool demo. t09 measures that the rules are not
// *over*-reaching.
type URLMismatch struct{}

func NewURLMismatch() *URLMismatch { return &URLMismatch{} }

func (t *URLMismatch) ID() string    { return "t09-url-mismatch" }
func (t *URLMismatch) Title() string { return "URL mismatch detection (non-sensitive URLs survive)" }
func (t *URLMismatch) Tier() eval.Tier {
	return eval.Tier2
}

func (t *URLMismatch) SupportedModes() []eval.Mode {
	return []eval.Mode{
		eval.ModeNoRedaction,
		eval.ModeRedact,
		eval.ModeRedactRehydrate,
		eval.ModeRedactRehydrateCompen,
	}
}

func (t *URLMismatch) Expected(mode eval.Mode) eval.Expected {
	return eval.ExpectedPass
}

const (
	urlPlain   = "https://api.example.com/v1/charges"
	urlWithKey = "https://api.example.com/v1/charges?api_key=sk_live_AbC7f9Kd2nE8xR4tL1qM0pH5sW3vZ6yB"
)

func (t *URLMismatch) Run(r eval.Redactor, mode eval.Mode) eval.Result {
	plainContent := "ENDPOINT_A=" + urlPlain + "\n"
	withKeyContent := "ENDPOINT_B=" + urlWithKey + "\n"

	sess := r.NewSession()

	feed := func(content string) string {
		if mode == eval.ModeNoRedaction {
			return content
		}
		out, _ := r.Redact(sess, []byte(content))
		return string(out)
	}

	plainView := feed(plainContent)
	keyView := feed(withKeyContent)

	plainVal := extractEnvValue(plainView, "ENDPOINT_A")
	keyVal := extractEnvValue(keyView, "ENDPOINT_B")
	if plainVal == "" || keyVal == "" {
		return eval.FailResult(
			"agent could not observe one of the URLs",
			map[string]string{"plain": plainVal, "with_key": keyVal},
		)
	}

	metrics := map[string]string{
		"plain":    plainVal,
		"with_key": keyVal,
	}

	// Invariant 1: the plain URL is not a secret and must survive
	// every mode unchanged. If it gets redacted, task fails — the
	// detector is firing on non-sensitive content.
	if plainVal != urlPlain {
		return eval.FailResult(
			"plain URL was altered by redaction — detector is over-reaching: "+plainVal,
			metrics,
		)
	}

	// Invariant 2: the with-key URL's structure is still present even
	// in redact modes. The scheme + host + path prefix
	// "https://api.example.com/v1/charges" must be readable in every
	// mode; only the Stripe key inside the query string is replaced
	// by a placeholder.
	endpointPrefix := "https://api.example.com/v1/charges"
	if !strings.HasPrefix(keyVal, endpointPrefix) {
		return eval.FailResult(
			"with-key URL lost its endpoint prefix after redaction: "+keyVal,
			metrics,
		)
	}

	// And in redact modes specifically, the Stripe key's raw value
	// must not survive in the observed content.
	if mode != eval.ModeNoRedaction && strings.Contains(keyVal, "sk_live_AbC7f9Kd2nE8xR4tL1qM0pH5sW3vZ6yB") {
		return eval.FailResult(
			"raw Stripe key leaked into observed URL: "+keyVal,
			metrics,
		)
	}

	// Final rubric: the two URLs share an endpoint. The agent's
	// comparison reduces to "does the prefix before any query string
	// match?" On both redacted and baseline inputs this is true, so
	// the agent concludes "same endpoint."
	if stripQuery(plainVal) != stripQuery(keyVal) {
		return eval.FailResult(
			"agent concluded endpoints differ: "+stripQuery(plainVal)+" vs "+stripQuery(keyVal),
			metrics,
		)
	}
	return eval.PassResult(metrics)
}

// stripQuery returns the URL up to the first `?`, or the whole URL if
// there is no query component.
func stripQuery(url string) string {
	if i := strings.Index(url, "?"); i >= 0 {
		return url[:i]
	}
	return url
}
