// Package session implements the session-scoped placeholder map.
//
// The map is the glue that lets the hook, the proxy, and the MCP server all
// agree on which real value maps to which placeholder within one agent session
// (PRD §5 "The session-scoped placeholder map" and §7.3).
//
// Properties:
//   - Deterministic within a session: same value always maps to the same
//     placeholder, enabling cross-file identity reasoning.
//   - Salted across sessions: a fresh random salt per session means placeholders
//     from one session cannot be replayed, correlated, or rehydrated in another.
//   - In-memory only: never written to disk. Daemon crash discards the map.
//   - Fail-closed rehydration: an unknown placeholder is passed through
//     unchanged, never resolved.
package session

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"regexp"
	"sync"
)

// Map is an in-memory, session-scoped placeholder store.
type Map struct {
	mu       sync.RWMutex
	salt     []byte
	byValue  map[string]string // real value -> placeholder
	byPlace  map[string]string // placeholder -> real value
}

// New returns an empty session map with a fresh 32-byte random salt.
func New() *Map {
	salt := make([]byte, 32)
	if _, err := rand.Read(salt); err != nil {
		panic(fmt.Sprintf("shhh/session: cannot generate salt: %v", err))
	}
	return &Map{
		salt:    salt,
		byValue: make(map[string]string),
		byPlace: make(map[string]string),
	}
}

// PlaceholderFor returns the deterministic placeholder for a given real secret.
// Calling it twice in the same session with the same value returns the same
// placeholder, which is what lets the agent reason about cross-file identity.
//
// label is the semantic type (e.g. "STRIPE_LIVE_KEY").
// publicPrefix is the part of the value that is public information about the
// service (e.g. "sk_live_"). It is preserved in the placeholder for context;
// it never leaks bits of the actual secret beyond the public prefix.
func (m *Map) PlaceholderFor(value, label, publicPrefix string) string {
	m.mu.Lock()
	defer m.mu.Unlock()

	if p, ok := m.byValue[value]; ok {
		return p
	}

	// Short suffix: first 4 bytes of HMAC-like SHA-256 over salt || value,
	// rendered as 8 hex chars. Unique enough within a session to disambiguate
	// multiple secrets of the same type; reveals no value bits to anyone who
	// does not hold the salt.
	h := sha256.New()
	h.Write(m.salt)
	h.Write([]byte(value))
	suffix := hex.EncodeToString(h.Sum(nil))[:8]

	var placeholder string
	if publicPrefix == "" {
		placeholder = fmt.Sprintf("[%s:%s]", label, suffix)
	} else {
		placeholder = fmt.Sprintf("[%s:%s...:%s]", label, publicPrefix, suffix)
	}

	m.byValue[value] = placeholder
	m.byPlace[placeholder] = value
	return placeholder
}

// Resolve looks up the real value for a placeholder. Returns ("", false) if
// the placeholder is not in the current session map (fail-closed rehydration).
func (m *Map) Resolve(placeholder string) (string, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	v, ok := m.byPlace[placeholder]
	return v, ok
}

// Compare returns true iff two placeholders resolve to the same real value in
// this session. Used by the compare_secrets MCP tool.
func (m *Map) Compare(a, b string) bool {
	m.mu.RLock()
	defer m.mu.RUnlock()
	va, oka := m.byPlace[a]
	vb, okb := m.byPlace[b]
	return oka && okb && va == vb
}

// Size returns the number of mapped secrets. Used by status commands and tests.
func (m *Map) Size() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.byValue)
}

// PlaceholderRe matches any shhh placeholder in text. Used by the rehydration
// path in the redactor package.
var PlaceholderRe = regexp.MustCompile(`\[[A-Z_]+(?::[^\]]*)?\]`)
