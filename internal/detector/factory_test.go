package detector

import (
	"os"
	"testing"
)

func TestNewFromEnvDefaultsToShhhNative(t *testing.T) {
	t.Setenv("SHHH_DETECTOR", "")
	b := NewFromEnv()
	if _, ok := b.(*Detector); !ok {
		t.Errorf("unset SHHH_DETECTOR should return *Detector, got %T", b)
	}
}

func TestNewFromEnvShhhNativeExplicit(t *testing.T) {
	t.Setenv("SHHH_DETECTOR", "shhh-native")
	if _, ok := NewFromEnv().(*Detector); !ok {
		t.Errorf("SHHH_DETECTOR=shhh-native should return *Detector")
	}
}

func TestNewFromEnvUnknownFallsBack(t *testing.T) {
	t.Setenv("SHHH_DETECTOR", "not-a-mode")
	if _, ok := NewFromEnv().(*Detector); !ok {
		t.Errorf("unknown SHHH_DETECTOR should fall back to *Detector")
	}
}

func TestNewFromEnvGitleaks(t *testing.T) {
	t.Setenv("SHHH_DETECTOR", "gitleaks")
	b := NewFromEnv()
	if _, ok := b.(*GitleaksBackend); !ok {
		// If gitleaks init failed silently, the factory falls back
		// to shhh-native with a stderr warning. The test should fail
		// loudly so we notice if gitleaks ever stops building.
		t.Fatalf("SHHH_DETECTOR=gitleaks should return *GitleaksBackend, got %T", b)
	}
}

func TestGitleaksBackendDetectsStripeKey(t *testing.T) {
	b, err := NewGitleaks()
	if err != nil {
		t.Fatalf("NewGitleaks: %v", err)
	}
	// A known stripe-shaped fixture value (fake, sourced from
	// shhh's own testdata).
	content := []byte("STRIPE_KEY=sk_live_4eC39HqLyjWDarjtT1zdp7dcfakeKeyForTesting")
	findings := b.Detect(content)
	if len(findings) == 0 {
		t.Fatal("expected gitleaks to detect a stripe key")
	}
	found := false
	for _, f := range findings {
		if f.Label == "STRIPE_LIVE_KEY" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected STRIPE_LIVE_KEY label, got labels: %v", labelList(findings))
	}
}

func labelList(fs []Finding) []string {
	out := make([]string, len(fs))
	for i, f := range fs {
		out[i] = f.Label
	}
	return out
}

func TestNewFromConfigDefaultIsNative(t *testing.T) {
	// Empty list → shhh-native fallback so detection is never silently disabled.
	os.Unsetenv("SHHH_DETECTOR")
	b := NewFromConfig(nil)
	if _, ok := b.(*Detector); !ok {
		t.Errorf("empty engines should fall back to *Detector, got %T", b)
	}
}

func TestNewFromConfigSingleShhhNative(t *testing.T) {
	b := NewFromConfig([]string{"shhh-native"})
	if _, ok := b.(*Detector); !ok {
		t.Errorf("[shhh-native] should return *Detector, got %T", b)
	}
}

func TestNewFromConfigSingleGitleaks(t *testing.T) {
	b := NewFromConfig([]string{"gitleaks"})
	if _, ok := b.(*GitleaksBackend); !ok {
		t.Fatalf("[gitleaks] should return *GitleaksBackend, got %T", b)
	}
}

func TestNewFromConfigUnknownEngineFallsBack(t *testing.T) {
	// All-unknown engines: caller gets a shhh-native fallback rather
	// than a nil backend.
	b := NewFromConfig([]string{"bogus"})
	if _, ok := b.(*Detector); !ok {
		t.Errorf("unknown engine should fall back to *Detector, got %T", b)
	}
}

func TestNewFromConfigMultiEngineComposition(t *testing.T) {
	b := NewFromConfig([]string{"gitleaks", "shhh-native"})
	if _, ok := b.(*multiEngineBackend); !ok {
		t.Fatalf("two engines should wrap into *multiEngineBackend, got %T", b)
	}
}

func TestMultiEngineSpanUnionAndPriority(t *testing.T) {
	// Drive the multi-engine merge manually with two stub backends
	// so the test doesn't depend on regex agreement between engines.
	a := &stubBackend{name: "A", findings: []Finding{
		{Label: "FIRST_FROM_A", Start: 0, End: 10},
		{Label: "SHARED", Start: 20, End: 30},
	}}
	b := &stubBackend{name: "B", findings: []Finding{
		{Label: "SHARED_FROM_B", Start: 20, End: 30}, // identical span → dedup
		{Label: "ONLY_FROM_B", Start: 40, End: 50},   // distinct → kept
	}}
	m := &multiEngineBackend{engines: []Backend{a, b}, names: []string{"A", "B"}}

	out := m.Detect([]byte("ignored — stubs return canned findings"))
	if len(out) != 3 {
		t.Fatalf("union should yield 3 findings, got %d (%v)", len(out), labelList(out))
	}
	if out[1].Label != "SHARED" {
		t.Errorf("shared span should keep engine-A label (priority), got %q", out[1].Label)
	}
}

// stubBackend is a minimal Backend used only by the multi-engine
// composition test. It returns a fixed slice regardless of input.
type stubBackend struct {
	name     string
	findings []Finding
}

func (s *stubBackend) Detect(_ []byte) []Finding { return s.findings }

