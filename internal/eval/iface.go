// Package eval implements the shhh-eval harness (PRD §10 Phase 0).
//
// The harness is designed to be product-agnostic: any redactor that implements
// the Redactor interface can be benchmarked. shhh is the reference
// implementation, not the subject of the test. This is strategic — a
// competitor that wants to be measured must conform to our interface, which
// makes shhh-eval a de-facto standard we control without our own product
// being prisoner of it.
//
// Phase 0 organizational note: this package lives in the main shhh module
// during private development. Before Phase 1 public launch, it will be split
// into a standalone `shhh-eval` repository with a stable import path.
package eval

// Redactor is the product-agnostic interface every benchmarked redactor must
// implement. It is intentionally minimal: redact content, rehydrate content.
// Session identity is carried by the opaque SessionID; implementations may
// store whatever state they need behind that handle.
type Redactor interface {
	// Name identifies the redactor in matrix reports (e.g., "shhh",
	// "rehydra", "contextio").
	Name() string

	// NewSession creates a fresh session and returns an opaque session ID.
	// Redactors that don't need sessions may return an empty string.
	NewSession() SessionID

	// Redact replaces secrets in content with deterministic placeholders
	// within the given session. Returns the redacted content and any
	// metadata the harness may want to log.
	Redact(sess SessionID, content []byte) (redacted []byte, meta RedactMeta)

	// Rehydrate substitutes placeholders back with their real values using
	// the session's map. Unknown placeholders must be passed through
	// unchanged (fail-closed).
	Rehydrate(sess SessionID, content []byte) []byte

	// ResolvePlaceholder looks up a single placeholder's real value in the
	// session. Returns false on miss. Used by compensatory tool
	// implementations (decode_jwt_safely, compare_secrets, etc.) that the
	// harness may mock.
	ResolvePlaceholder(sess SessionID, placeholder string) (string, bool)
}

// SessionID is an opaque handle to a per-session placeholder store.
type SessionID string

// RedactMeta carries structured information about a redaction pass for the
// harness to log, such as the number of findings and their types.
type RedactMeta struct {
	FindingCount int
	Labels       []string
}
