package tasks

import (
	"strings"

	"github.com/Musubi42/shhh/internal/eval"
)

// ConnStringDiff is task 2: an agent is shown two versions of a
// PostgreSQL connection string and asked to identify which component
// changed. The two URLs differ only in the password; every other part
// (scheme, user, host, port, database) is identical.
//
// This task exists to validate PRD §5's "connection strings preserve
// structure (user, host, port, database) in the context passed to the
// LLM but strip query-string parameters that frequently hold tokens."
// Before step 14 landed, shhh redacted the entire URL as an opaque
// placeholder; an agent looking at two opaque placeholders had no way
// to know whether the host, the port, or the password changed. After
// step 14, the structural redactor produces placeholders like
// `[POSTGRES_CONNSTRING:admin@host:5432/db:hash]` where the hash
// differs iff the underlying value differs and everything else stays
// byte-identical when the structural parts match.
//
// Expected behavior per mode:
//
//   - no-redaction: agent compares the two raw URLs and reports "password
//     changed." PASS.
//   - redact: agent compares the two structural placeholders. Because
//     both URLs share the same structural description
//     ("admin@prod-db.internal:5432/myapp"), the placeholders differ
//     *only* in the 8-char hash suffix. The agent can textually infer
//     "structure is identical, value differs — so it is the
//     credential-or-query part that changed." PASS — this is the whole
//     point of step 14.
//   - redact+rehydrate: same reasoning path as redact; the agent still
//     sees placeholders because nothing gets rehydrated into tool_use
//     args in this task. PASS.
//   - redact+compensatory: the agent could also use `compare_secrets`
//     to confirm the two values are different (which they are). PASS.
//
// Unlike t01, t03, and t05, this task passes in *every* mode — that is
// the product signal the roadmap wanted: structural redaction for
// connection strings is good enough to preserve the signal without any
// compensatory fallback.
type ConnStringDiff struct{}

func NewConnStringDiff() *ConnStringDiff { return &ConnStringDiff{} }

func (t *ConnStringDiff) ID() string    { return "t02-connstring-diff" }
func (t *ConnStringDiff) Title() string { return "Connection-string diff at query-param granularity" }
func (t *ConnStringDiff) Tier() eval.Tier {
	return eval.Tier2
}

func (t *ConnStringDiff) SupportedModes() []eval.Mode {
	return []eval.Mode{
		eval.ModeNoRedaction,
		eval.ModeRedact,
		eval.ModeRedactRehydrate,
		eval.ModeRedactRehydrateCompen,
	}
}

func (t *ConnStringDiff) Expected(mode eval.Mode) eval.Expected {
	return eval.ExpectedPass
}

const (
	connstringV1 = "postgresql://admin:oldPass_xyz123@prod-db.internal:5432/myapp"
	connstringV2 = "postgresql://admin:newPass_abc456@prod-db.internal:5432/myapp"
)

func (t *ConnStringDiff) Run(r eval.Redactor, mode eval.Mode) eval.Result {
	// Build a diff hunk — two KEY=VAL lines, one per version. This is
	// the shape a git diff would show the agent after editing .env.
	v1Content := "DATABASE_URL=" + connstringV1 + "\n"
	v2Content := "DATABASE_URL=" + connstringV2 + "\n"

	sess := r.NewSession()

	feed := func(content string) string {
		if mode == eval.ModeNoRedaction {
			return content
		}
		out, _ := r.Redact(sess, []byte(content))
		return string(out)
	}

	v1View := feed(v1Content)
	v2View := feed(v2Content)

	v1Val := extractEnvValue(v1View, "DATABASE_URL")
	v2Val := extractEnvValue(v2View, "DATABASE_URL")
	if v1Val == "" || v2Val == "" {
		return eval.FailResult(
			"agent could not observe one of the URL values",
			map[string]string{"v1": v1Val, "v2": v2Val},
		)
	}

	metrics := map[string]string{"v1": v1Val, "v2": v2Val}

	// The agent's rubric: "what changed between v1 and v2?"
	//
	// In baseline mode the values are two raw URLs; the agent parses
	// them and reports which field differs. In redact modes the values
	// are two structural placeholders with the same structural body
	// and (if the password changed) different hash suffixes. The
	// classifier below handles both shapes.
	changed := classifyConnstringDiff(v1Val, v2Val)
	metrics["changed"] = changed

	if changed == "nothing" {
		return eval.FailResult("agent saw no difference between v1 and v2", metrics)
	}
	// For this fixture the only change IS the password, so any
	// classification that identifies a credential-level difference is
	// accepted. The coarse "some-non-structural-field" bucket is
	// enough to prove structural redaction preserves enough signal;
	// fine-grained component identification is a future refinement.
	if changed != "credentials-or-query" && changed != "password" {
		return eval.FailResult("agent misidentified the changed component: "+changed, metrics)
	}

	// Optional: use the compensatory tool to double-check. In +compen
	// mode an agent might call compare_secrets to confirm the two
	// values really are distinct (a "same structural body, different
	// hash" placeholder pair should not resolve to the same value).
	if mode == eval.ModeRedactRehydrateCompen {
		tools := eval.NewCompensatoryTools(r, sess)
		if tools.CompareSecrets(v1Val, v2Val) {
			return eval.FailResult(
				"compare_secrets reported v1==v2 but they should differ",
				metrics,
			)
		}
	}

	return eval.PassResult(metrics)
}

// classifyConnstringDiff inspects two observable values (raw URLs or
// structural placeholders) and returns a short tag describing what
// changed between them.
//
// Possible return values:
//   - "nothing"              — the two observables are byte-identical
//   - "structural"           — the non-secret parts (host/port/path/user)
//                              differ; agent can see the differing
//                              tokens directly
//   - "credentials-or-query" — structural parts match, only the hashed
//                              tail differs (this is what the redact
//                              modes see when the password changed)
//   - "password"             — baseline-mode reading where raw URL
//                              parsing identifies the password field
//                              as the sole difference
func classifyConnstringDiff(a, b string) string {
	if a == b {
		return "nothing"
	}
	// Raw URL case (baseline mode): both start with a scheme like
	// "postgresql://". Parse out the password field and compare the
	// rest.
	if strings.HasPrefix(a, "postgresql://") && strings.HasPrefix(b, "postgresql://") {
		if rawConnstringOnlyPasswordDiffers(a, b) {
			return "password"
		}
		return "structural"
	}
	// Redact-mode case: both are placeholders with the same label. The
	// structural body is everything between the second and last colon.
	aBody, aHash := splitStructuralPlaceholder(a)
	bBody, bHash := splitStructuralPlaceholder(b)
	if aBody != "" && bBody != "" && aBody == bBody && aHash != bHash {
		return "credentials-or-query"
	}
	return "structural"
}

// rawConnstringOnlyPasswordDiffers reports whether two URLs of the
// same scheme differ only in the password component.
func rawConnstringOnlyPasswordDiffers(a, b string) bool {
	userA, pwA, restA := splitConnstring(a)
	userB, pwB, restB := splitConnstring(b)
	return userA == userB && restA == restB && pwA != pwB
}

// splitConnstring returns (user, password, hostAndRest) from a URL of
// the form scheme://user:password@host[:port]/path. Not a full URL
// parser — sufficient for the t02 fixture shape.
func splitConnstring(raw string) (user, password, rest string) {
	schemeSep := strings.Index(raw, "://")
	if schemeSep < 0 {
		return "", "", raw
	}
	auth := raw[schemeSep+3:]
	at := strings.Index(auth, "@")
	if at < 0 {
		return "", "", raw
	}
	userInfo := auth[:at]
	rest = raw[:schemeSep+3] + auth[at+1:]
	colon := strings.Index(userInfo, ":")
	if colon < 0 {
		return userInfo, "", rest
	}
	return userInfo[:colon], userInfo[colon+1:], rest
}

// splitStructuralPlaceholder splits
// `[POSTGRES_CONNSTRING:admin@prod-db:5432/myapp:abcd1234]` into the
// structural body (`admin@prod-db:5432/myapp`) and the hash suffix
// (`abcd1234`). Returns empty strings if the shape does not match.
func splitStructuralPlaceholder(p string) (body, hash string) {
	if len(p) < 2 || p[0] != '[' || p[len(p)-1] != ']' {
		return "", ""
	}
	inner := p[1 : len(p)-1]
	firstColon := strings.Index(inner, ":")
	if firstColon < 0 {
		return "", ""
	}
	// inner = "LABEL:body:hash"
	after := inner[firstColon+1:]
	lastColon := strings.LastIndex(after, ":")
	if lastColon < 0 {
		return "", ""
	}
	return after[:lastColon], after[lastColon+1:]
}

