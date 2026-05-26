package detector

import (
	"fmt"
	"os"
)

// Mode selects which Backend `NewFromEnv` returns.
type Mode string

const (
	ModeNative   Mode = "shhh-native" // shhh's first-party detector (env cross-ref, structural URL)
	ModeGitleaks Mode = "gitleaks"    // gitleaks library
)

// NewFromEnv returns the Backend chosen by the `SHHH_DETECTOR`
// environment variable. Unset or unrecognised values fall back to
// `ModeNative`. Failures to initialise gitleaks (rare) are
// reported with a stderr warning and a downgrade to shhh-native —
// detection MUST be available.
//
// This is a debug knob retained while Phase 2 introduces
// `NewFromConfig` and threads engine selection through the user
// config. After Phase 2 lands, callers should prefer
// `NewFromConfig`; this helper remains for ad-hoc testing.
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
