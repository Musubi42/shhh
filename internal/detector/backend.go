package detector

// Backend is the minimal contract every detection engine must
// satisfy. Born from the 2026-05-26 gitleaks spike (see
// `docs/dev/gitleaks-spike.md`): we wanted to swap detectors behind
// an env flag without rewriting every caller.
//
// The interface is intentionally tiny — one method, one type. The
// `*Detector` struct (the shhh-native backend) satisfies it
// implicitly. `GitleaksBackend` is the gitleaks-backed
// implementation. Phase 2 introduces a multi-engine wrapper
// driven by the user config; until then, callers select via
// `NewFromEnv` or construct backends directly.
//
// Callers should depend on `Backend`, not `*Detector`. Existing
// pointer-typed call sites still compile because `*Detector`
// satisfies the interface.
type Backend interface {
	// Detect returns non-overlapping findings sorted by offset.
	// `content` is the raw bytes to scan; backends MUST NOT mutate
	// it. An empty result means "no secrets found"; an error path
	// does not exist here on purpose — detection is best-effort and
	// a backend that fails to parse one chunk should still return
	// whatever findings it managed to collect.
	Detect(content []byte) []Finding
}

// compile-time guarantee that the shhh-native concrete type
// satisfies the interface. Catches API drift the first time
// someone changes the Detect signature.
var _ Backend = (*Detector)(nil)
