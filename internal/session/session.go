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
// structuralDesc, if non-empty, overrides publicPrefix rendering with a full
// structural description (e.g. "admin@prod-db:5432/myapp" for a postgres URL),
// which is how PRD §5 "preserve structure, strip credentials and query string"
// is implemented for connection-string rules.
func (m *Map) PlaceholderFor(value, label, publicPrefix, structuralDesc string) string {
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
	switch {
	case structuralDesc != "":
		placeholder = fmt.Sprintf("[%s:%s:%s]", label, structuralDesc, suffix)
	case publicPrefix == "":
		placeholder = fmt.Sprintf("[%s:%s]", label, suffix)
	default:
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

// Entry is one placeholder <-> value pair in a serialized snapshot.
type Entry struct {
	Placeholder string `json:"placeholder"`
	Value       string `json:"value"`
}

// Snapshot returns the salt (hex) and all mapped entries. Used by the hook
// sessionstore to persist per-session state across hook invocations — each
// hook firing is its own process, so determinism across firings requires
// persisting both the salt (so PlaceholderFor is stable for new values) and
// the entries (so Resolve works on values seen by an earlier firing).
func (m *Map) Snapshot() (saltHex string, entries []Entry) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	saltHex = hex.EncodeToString(m.salt)
	entries = make([]Entry, 0, len(m.byPlace))
	for p, v := range m.byPlace {
		entries = append(entries, Entry{Placeholder: p, Value: v})
	}
	return saltHex, entries
}

// FromSnapshot reconstructs a Map from a previously produced Snapshot. An
// empty saltHex is treated as an error (callers should use New() in that
// case). Unknown-shaped entries are skipped silently so an older on-disk
// store never blocks a newer binary.
func FromSnapshot(saltHex string, entries []Entry) (*Map, error) {
	salt, err := hex.DecodeString(saltHex)
	if err != nil {
		return nil, fmt.Errorf("shhh/session: bad salt: %w", err)
	}
	if len(salt) == 0 {
		return nil, fmt.Errorf("shhh/session: empty salt")
	}
	m := &Map{
		salt:    salt,
		byValue: make(map[string]string, len(entries)),
		byPlace: make(map[string]string, len(entries)),
	}
	for _, e := range entries {
		if e.Placeholder == "" || e.Value == "" {
			continue
		}
		m.byValue[e.Value] = e.Placeholder
		m.byPlace[e.Placeholder] = e.Value
	}
	return m, nil
}
