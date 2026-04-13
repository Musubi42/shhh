package eval

// CompensatoryTools is the bundle of MCP-compensatory tool implementations
// available to eval tasks running in `redact-rehydrate-compensatory` mode.
// Each method maps to one of the tools specified in PRD §7.7:
//
//   - DecodeJWTSafely    → decode_jwt_safely(placeholder_id)
//   - CompareSecrets     → compare_secrets(placeholder_a, placeholder_b)
//   - GrepHardcoded      → grep_hardcoded(placeholder_id, directory)
//   - ExplainSecret      → explain_secret(placeholder_id)
//
// In Phase 0, these are plain Go methods called directly by task code. In
// Phase 4 the same logic ships behind a real MCP server that agents talk to
// over stdio JSON-RPC. The eval harness can verify the logic without
// needing the MCP transport.
//
// All compensatory methods fail closed on unknown placeholders: if the
// session map does not contain the placeholder, the method returns ok=false
// and no data. This mirrors the PRD §5 Layer-2 "Fails closed" invariant.
type CompensatoryTools struct {
	r    Redactor
	sess SessionID
}

// NewCompensatoryTools binds a compensatory-tools bundle to a particular
// redactor session. All lookups go through the session map.
func NewCompensatoryTools(r Redactor, sess SessionID) *CompensatoryTools {
	return &CompensatoryTools{r: r, sess: sess}
}

// DecodeJWTSafely resolves a JWT placeholder to its real value via the
// session map and returns the decoded payload claims. Returns ok=false if
// the placeholder is unknown or the resolved value is not a JWT.
//
// This is the task-1 compensatory tool. Without it, an agent given a
// redacted JWT has no way to inspect expiry or scopes because the raw
// payload is base64-encoded inside the token.
func (c *CompensatoryTools) DecodeJWTSafely(placeholder string) (*JWTClaims, bool) {
	value, ok := c.r.ResolvePlaceholder(c.sess, placeholder)
	if !ok {
		return nil, false
	}
	claims, err := DecodeJWTPayload(value)
	if err != nil {
		return nil, false
	}
	return claims, true
}

// CompareSecrets returns true iff two placeholders refer to the same real
// value in the session. This is the task-3 compensatory tool. The
// implementation is thin because the session map already supports value-
// identity comparison natively — the tool simply exposes it to agents that
// otherwise only see opaque placeholder strings.
func (c *CompensatoryTools) CompareSecrets(a, b string) bool {
	va, oka := c.r.ResolvePlaceholder(c.sess, a)
	vb, okb := c.r.ResolvePlaceholder(c.sess, b)
	return oka && okb && va == vb
}
