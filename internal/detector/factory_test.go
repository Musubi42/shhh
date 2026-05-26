package detector

import "testing"

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

