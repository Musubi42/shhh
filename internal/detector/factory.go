package detector

import (
	"fmt"
	"os"
	"sort"
	"sync"
)

// Mode selects which Backend `NewFromEnv` returns.
type Mode string

const (
	ModeNative   Mode = "shhh-native" // shhh's first-party detector (env cross-ref, structural URL)
	ModeGitleaks Mode = "gitleaks"    // gitleaks library
)

// NewFromEnv returns the Backend chosen by the `SHHH_DETECTOR`
// environment variable. Unset or unrecognised values fall back to
// `ModeNative`. Used by ad-hoc tools (`shhh redact`, tests) that
// don't have access to the user `Config`. Production callers
// should prefer `NewFromConfig`.
func NewFromEnv() Backend {
	return newForMode(Mode(os.Getenv("SHHH_DETECTOR")))
}

func newForMode(m Mode) Backend {
	native := New()
	switch m {
	case ModeGitleaks:
		gl, err := NewGitleaks()
		if err != nil {
			fmt.Fprintf(os.Stderr, "shhh: SHHH_DETECTOR=gitleaks but init failed (%v); falling back to shhh-native\n", err)
			return native
		}
		return gl
	default:
		return native
	}
}

// NewFromConfig builds the Backend driven by the user's engine
// selection. `engines` is an ordered, non-empty list of engine
// names (`shhh-native`, `gitleaks`). The caller is expected to
// have substituted the default already (see `Config.EffectiveEngines`).
//
// One-engine selection returns the engine directly. Multi-engine
// selection wraps them in `multiEngineBackend` which runs all
// engines in parallel and merges findings with union-by-span
// (ties broken by selection order — first engine wins for label
// attribution).
//
// Engine init failures are non-fatal: a stderr warning is emitted
// and the engine is dropped from the set. If every engine fails,
// the function falls back to `shhh-native` to guarantee detection
// remains available.
func NewFromConfig(engines []string) Backend {
	if len(engines) == 0 {
		fmt.Fprintln(os.Stderr, "shhh: empty engine list; falling back to shhh-native")
		return New()
	}
	backends := make([]Backend, 0, len(engines))
	kept := make([]string, 0, len(engines))
	for _, name := range engines {
		switch Mode(name) {
		case ModeNative:
			backends = append(backends, New())
			kept = append(kept, name)
		case ModeGitleaks:
			gl, err := NewGitleaks()
			if err != nil {
				fmt.Fprintf(os.Stderr, "shhh: engine %q init failed (%v); skipping\n", name, err)
				continue
			}
			backends = append(backends, gl)
			kept = append(kept, name)
		default:
			fmt.Fprintf(os.Stderr, "shhh: unknown engine %q; skipping\n", name)
		}
	}
	if len(backends) == 0 {
		fmt.Fprintln(os.Stderr, "shhh: every requested engine failed to init; falling back to shhh-native")
		return New()
	}
	if len(backends) == 1 {
		return backends[0]
	}
	return &multiEngineBackend{engines: backends, names: kept}
}

// multiEngineBackend runs N detection engines in parallel and
// merges their findings with union-by-span: findings sharing
// identical (Start, End) are deduplicated, keeping the one whose
// engine appears earliest in the user's `Config.Engines` ordering.
// Otherwise both findings are kept (favours redaction over
// minimalism).
//
// The implementation is intentionally simple: strict span equality
// is the dedup key. Engines that flag the same secret with slightly
// different boundaries (e.g. one includes `KEY=` prefix, one
// doesn't) will produce two findings. The redactor's span splice
// logic tolerates overlapping spans by skipping any finding whose
// span intersects an already-spliced one.
type multiEngineBackend struct {
	engines []Backend
	names   []string // parallel to engines; controls label priority
}

func (m *multiEngineBackend) Detect(content []byte) []Finding {
	results := make([][]Finding, len(m.engines))
	var wg sync.WaitGroup
	for i, e := range m.engines {
		wg.Add(1)
		go func(idx int, eng Backend) {
			defer wg.Done()
			results[idx] = eng.Detect(content)
		}(i, e)
	}
	wg.Wait()

	seen := make(map[[2]int]int) // span → engine index that owns the slot
	var out []Finding
	for engIdx, findings := range results {
		for _, f := range findings {
			key := [2]int{f.Start, f.End}
			if existing, ok := seen[key]; ok {
				if engIdx < existing {
					// Earlier engine wins; replace in-place.
					for i := range out {
						if out[i].Start == f.Start && out[i].End == f.End {
							out[i] = f
							break
						}
					}
					seen[key] = engIdx
				}
				continue
			}
			seen[key] = engIdx
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Start != out[j].Start {
			return out[i].Start < out[j].Start
		}
		return out[i].End < out[j].End
	})
	return out
}

// CheckEnvValue forwards to whichever underlying engine exposes
// it. The capability is only relevant for shhh-native; calling
// it on a gitleaks-only backend returns false. When multiple
// engines are active, the first one supporting the capability
// wins (matches the label-priority rule).
//
// The redactor type-asserts for `envValueChecker` to find this
// method, so satisfying the optional interface keeps the env-pass
// alive when the user has shhh-native in their selection.
func (m *multiEngineBackend) CheckEnvValue(value string) bool {
	type envChecker interface{ CheckEnvValue(string) bool }
	for _, eng := range m.engines {
		if ec, ok := eng.(envChecker); ok {
			return ec.CheckEnvValue(value)
		}
	}
	return false
}
