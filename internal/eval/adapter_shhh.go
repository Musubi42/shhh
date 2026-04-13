package eval

import (
	"sync"

	"github.com/musubi-sasu/shhh/internal/detector"
	"github.com/musubi-sasu/shhh/internal/redactor"
	"github.com/musubi-sasu/shhh/internal/session"
)

// ShhhAdapter wraps the shhh redactor into the product-agnostic Redactor
// interface so shhh can be benchmarked by its own harness.
type ShhhAdapter struct {
	mu       sync.Mutex
	sessions map[SessionID]*redactor.Redactor
	det      *detector.Detector
	seq      int
}

// NewShhhAdapter returns a fresh adapter with a shared detector across
// sessions (detector is stateless).
func NewShhhAdapter() *ShhhAdapter {
	return &ShhhAdapter{
		sessions: make(map[SessionID]*redactor.Redactor),
		det:      detector.New(),
	}
}

func (a *ShhhAdapter) Name() string { return "shhh" }

func (a *ShhhAdapter) NewSession() SessionID {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.seq++
	id := SessionID(genID(a.seq))
	a.sessions[id] = redactor.New(a.det, session.New())
	return id
}

func (a *ShhhAdapter) Redact(sess SessionID, content []byte) ([]byte, RedactMeta) {
	r := a.lookup(sess)
	if r == nil {
		return content, RedactMeta{}
	}
	out, findings := r.Redact(content)
	meta := RedactMeta{FindingCount: len(findings)}
	for _, f := range findings {
		meta.Labels = append(meta.Labels, f.Label)
	}
	return out, meta
}

func (a *ShhhAdapter) Rehydrate(sess SessionID, content []byte) []byte {
	r := a.lookup(sess)
	if r == nil {
		return content
	}
	return r.Rehydrate(content)
}

func (a *ShhhAdapter) ResolvePlaceholder(sess SessionID, placeholder string) (string, bool) {
	// Phase 0 note: the current shhh redactor exposes rehydration at the
	// content level, not individual placeholder lookup. We emulate the
	// latter by rehydrating a content buffer containing just the
	// placeholder and checking if it changed. This is sufficient for the
	// harness tests and will be replaced by direct session-map access in
	// Phase 4 when the daemon's session map has a public lookup API.
	r := a.lookup(sess)
	if r == nil {
		return "", false
	}
	out := r.Rehydrate([]byte(placeholder))
	if string(out) == placeholder {
		return "", false
	}
	return string(out), true
}

func (a *ShhhAdapter) lookup(sess SessionID) *redactor.Redactor {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sessions[sess]
}

func genID(n int) string {
	return "shhh-sess-" + itoa(n)
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
