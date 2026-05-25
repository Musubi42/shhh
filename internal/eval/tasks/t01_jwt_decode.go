package tasks

import (
	"fmt"
	"strings"

	"github.com/Musubi42/shhh/internal/eval"
)

// JWTDecode is task 1: JWT decode & claims inspection. The agent is given a
// `.env` file containing a session token (a JWT) and asked to report when
// the token expires and what scopes it grants.
//
// Expected behavior per mode:
//
//   - no-redaction: the agent sees the raw JWT. Decoding is a base64+JSON
//     operation any agent can perform mentally or with a local tool. PASS.
//   - redact: the agent sees a placeholder like
//     `[JWT_TOKEN:eyJ...:abcd1234]`. There is no way to recover the claims
//     from this string alone — the whole payload is hidden. FAIL.
//   - redact+rehydrate: the proxy rehydrates placeholders in `tool_use`
//     arguments, not in the content the model is reasoning about. For a
//     claims-inspection task where the agent never calls a tool, this
//     mode is equivalent to `redact` and also FAILs. This is exactly the
//     failure mode the critique review anticipated.
//   - redact+compensatory: the agent has access to the
//     `decode_jwt_safely(placeholder_id)` MCP tool. Calling it returns the
//     payload claims (exp, scopes) without ever exposing the raw token.
//     PASS.
//
// Task 1 is the purest demonstration of "compensatory tool unblocks
// structured-secret reasoning." If this task does not pass in compensatory
// mode, the compensatory-tool pattern is broken and a lot of downstream
// architecture has to be revisited.
//
// Phase 0 note: this task simulates the agent in Go. It does not call an
// LLM. The simulation captures exactly the three branches above — try
// direct decode, try compensatory tool, give up — and tests whether each
// branch actually succeeds against the current shhh implementation. When
// we later swap to a real Claude Code runner (roadmap step 16), the
// task's contract (prompt, fixture, rubric) stays the same; only the
// "what the agent does in response" changes.
type JWTDecode struct{}

func NewJWTDecode() *JWTDecode { return &JWTDecode{} }

func (t *JWTDecode) ID() string    { return "t01-jwt-decode" }
func (t *JWTDecode) Title() string { return "JWT decode & claims inspection" }
func (t *JWTDecode) Tier() eval.Tier {
	return eval.Tier1
}

func (t *JWTDecode) SupportedModes() []eval.Mode {
	return []eval.Mode{
		eval.ModeNoRedaction,
		eval.ModeRedact,
		eval.ModeRedactRehydrate,
		eval.ModeRedactRehydrateCompen,
	}
}

// Expected encodes the Phase-0 design story for JWT decoding: baseline
// and compensatory mode pass; pure-redact and redact+rehydrate fail
// *by design* because the payload is hidden inside the token and the
// rehydration layer only operates on tool_use arguments. A pass in
// redact/redact+rehydrate would be a surprise worth investigating; a
// fail in baseline or compensatory is a regression.
func (t *JWTDecode) Expected(mode eval.Mode) eval.Expected {
	switch mode {
	case eval.ModeNoRedaction, eval.ModeRedactRehydrateCompen:
		return eval.ExpectedPass
	case eval.ModeRedact, eval.ModeRedactRehydrate:
		return eval.ExpectedFail
	}
	return eval.ExpectedPass
}

// Expected claim values used by the rubric. Fixed so the assertion is
// deterministic.
const (
	expectedSubject = "alice@example.com"
	expectedExpiry  = int64(1999999999)
	expectedScope0  = "read"
	expectedScope1  = "write"
)

func (t *JWTDecode) Run(r eval.Redactor, mode eval.Mode) eval.Result {
	// Build the fixture: a .env file with a SESSION_TOKEN containing a
	// well-formed JWT whose payload has the expected claims.
	jwt := eval.BuildJWT(map[string]interface{}{
		"sub":    expectedSubject,
		"exp":    expectedExpiry,
		"iat":    int64(1516239022),
		"scopes": []string{expectedScope0, expectedScope1},
	})
	envContent := "SESSION_TOKEN=" + jwt + "\n"

	// Step 1: simulate the agent reading the .env file. In non-baseline
	// modes the content is redacted before the agent sees it.
	sess := r.NewSession()
	var visibleContent []byte
	if mode == eval.ModeNoRedaction {
		visibleContent = []byte(envContent)
	} else {
		visibleContent, _ = r.Redact(sess, []byte(envContent))
	}

	// Step 2: simulate the agent extracting the SESSION_TOKEN value from
	// the content it sees.
	tokenStr := extractEnvValue(string(visibleContent), "SESSION_TOKEN")
	if tokenStr == "" {
		return eval.FailResult("agent could not find SESSION_TOKEN in .env", nil)
	}

	// Step 3: simulate the agent trying to decode the value as a JWT
	// directly. In baseline mode this works; in redact modes it fails
	// because the token is a placeholder.
	if claims, err := eval.DecodeJWTPayload(tokenStr); err == nil {
		return rubric(claims, mode, "direct decode")
	}

	// Step 4: if we are in compensatory mode, the agent tries the
	// decode_jwt_safely tool against the placeholder it observed.
	if mode == eval.ModeRedactRehydrateCompen {
		tools := eval.NewCompensatoryTools(r, sess)
		if claims, ok := tools.DecodeJWTSafely(tokenStr); ok {
			return rubric(claims, mode, "decode_jwt_safely")
		}
		return eval.FailResult(
			"compensatory tool decode_jwt_safely returned no claims for "+tokenStr,
			map[string]string{"placeholder": tokenStr},
		)
	}

	// Otherwise: the agent has nothing left to try. This is the expected
	// failure in `redact` and `redact+rehydrate` modes and validates the
	// thesis that pure redaction breaks JWT reasoning.
	return eval.FailResult(
		"agent has no way to decode JWT from placeholder in mode "+string(mode),
		map[string]string{"observed": tokenStr},
	)
}

// rubric checks that the decoded claims match the expected values and
// returns a Result with structured metrics.
func rubric(claims *eval.JWTClaims, mode eval.Mode, source string) eval.Result {
	metrics := map[string]string{
		"source":        source,
		"decoded_sub":   claims.Subject,
		"decoded_exp":   fmt.Sprintf("%d", claims.Expiry),
		"decoded_scopes": strings.Join(claims.Scopes, ","),
	}
	if claims.Subject != expectedSubject {
		return eval.FailResult(fmt.Sprintf("sub mismatch: got %q, want %q", claims.Subject, expectedSubject), metrics)
	}
	if claims.Expiry != expectedExpiry {
		return eval.FailResult(fmt.Sprintf("exp mismatch: got %d, want %d", claims.Expiry, expectedExpiry), metrics)
	}
	if len(claims.Scopes) != 2 || claims.Scopes[0] != expectedScope0 || claims.Scopes[1] != expectedScope1 {
		return eval.FailResult(fmt.Sprintf("scopes mismatch: got %v", claims.Scopes), metrics)
	}
	return eval.PassResult(metrics)
}

// extractEnvValue parses a KEY=value line out of .env-shaped content and
// returns the value (with surrounding quotes stripped). Returns empty
// string on miss. Trivial parser — sufficient for eval fixtures.
func extractEnvValue(content, key string) string {
	prefix := key + "="
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			value := strings.TrimPrefix(line, prefix)
			value = strings.Trim(value, `"'`)
			return value
		}
	}
	return ""
}
