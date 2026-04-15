package eval

import (
	"encoding/base64"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

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
// value in the session. This is the task-3 compensatory tool.
//
// The straightforward case is raw equality — two placeholders whose
// resolved values are byte-identical. That handles the "same value
// copy-pasted across files" subcase.
//
// The subtler case is cross-representation equality: a Kubernetes Secret
// stores `sk_live_abc…` as base64, a .env file stores the same bytes
// verbatim. The session map sees two distinct values (because they are
// distinct byte strings) and produces two distinct placeholders. Without
// base64 awareness an agent looking at those two placeholders cannot
// conclude they refer to the same underlying secret, so the compensatory
// tool tries base64 decoding on either side before giving up.
//
// We accept both StdEncoding (padded with `=`) and RawStdEncoding (no
// padding). Real-world callers split roughly evenly between the two:
// kubectl emits padded, some config generators emit raw.
func (c *CompensatoryTools) CompareSecrets(a, b string) bool {
	va, oka := c.r.ResolvePlaceholder(c.sess, a)
	vb, okb := c.r.ResolvePlaceholder(c.sess, b)
	if !oka || !okb {
		return false
	}
	if va == vb {
		return true
	}
	return base64Equals(va, vb) || base64Equals(vb, va)
}

// base64Equals reports whether decoding encoded as base64 yields plain.
// Tries padded and raw standard alphabets. Returns false on any decode
// error or length mismatch.
func base64Equals(encoded, plain string) bool {
	if decoded, err := base64.StdEncoding.DecodeString(encoded); err == nil && string(decoded) == plain {
		return true
	}
	if decoded, err := base64.RawStdEncoding.DecodeString(encoded); err == nil && string(decoded) == plain {
		return true
	}
	return false
}

// GrepHardcoded is the task-5 compensatory tool. It resolves a
// placeholder to its real value via the session map and walks a
// directory looking for the raw bytes in file contents. Returns a
// sorted slice of file paths that contain the value. The list is
// sorted so callers see a deterministic order across runs.
//
// The tool exists because an agent looking at a redacted placeholder
// cannot grep the filesystem directly — the placeholder is not what is
// on disk. Two paths solve the problem:
//
//  1. Rehydrate placeholders in tool_use args at the proxy boundary,
//     so the agent can call a plain `grep` and the proxy substitutes
//     the real value into the command before execution. This is the
//     PRD §7.5 rehydration story and it works for task 5.
//  2. Offer `grep_hardcoded(placeholder_id, directory)` as an MCP
//     tool that does the same thing, but within the session boundary
//     so the raw value never leaves the daemon.
//
// Phase 0 ships both so the eval can measure them independently. In
// Phase 4 the choice is a UX question (one tool call vs a bash
// roundtrip) rather than a capability gap.
//
// On unknown placeholders or walk errors the tool returns an empty
// slice (fail closed). It does not surface I/O errors to the caller;
// an agent would interpret those as "no hits" anyway.
func (c *CompensatoryTools) GrepHardcoded(placeholder, dir string) []string {
	value, ok := c.r.ResolvePlaceholder(c.sess, placeholder)
	if !ok || value == "" {
		return nil
	}
	var hits []string
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if strings.Contains(string(content), value) {
			hits = append(hits, path)
		}
		return nil
	})
	sort.Strings(hits)
	return hits
}
