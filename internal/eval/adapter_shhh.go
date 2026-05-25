package eval

import (
	"strconv"
	"sync"

	"github.com/Musubi42/shhh/internal/detector"
	"github.com/Musubi42/shhh/internal/redactor"
	"github.com/Musubi42/shhh/internal/session"
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
	id := SessionID("shhh-sess-" + strconv.Itoa(a.seq))
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
	r := a.lookup(sess)
	if r == nil {
		return "", false
	}
	return r.Resolve(placeholder)
}

func (a *ShhhAdapter) lookup(sess SessionID) *redactor.Redactor {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.sessions[sess]
}
