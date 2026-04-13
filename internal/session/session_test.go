package session

import (
	"strings"
	"testing"
)

func TestPlaceholderDeterministic(t *testing.T) {
	m := New()
	a := m.PlaceholderFor("sk_live_abcDEF123456789012345678", "STRIPE_LIVE_KEY", "sk_live_")
	b := m.PlaceholderFor("sk_live_abcDEF123456789012345678", "STRIPE_LIVE_KEY", "sk_live_")
	if a != b {
		t.Errorf("same value should map to same placeholder: %q vs %q", a, b)
	}
}

func TestPlaceholderDistinctValues(t *testing.T) {
	m := New()
	a := m.PlaceholderFor("sk_live_aaaaaaaaaaaaaaaaaaaaaaaa", "STRIPE_LIVE_KEY", "sk_live_")
	b := m.PlaceholderFor("sk_live_bbbbbbbbbbbbbbbbbbbbbbbb", "STRIPE_LIVE_KEY", "sk_live_")
	if a == b {
		t.Errorf("different values should map to different placeholders")
	}
}

func TestPlaceholderSessionSalted(t *testing.T) {
	m1 := New()
	m2 := New()
	a := m1.PlaceholderFor("sk_live_same_value_different_session", "STRIPE_LIVE_KEY", "sk_live_")
	b := m2.PlaceholderFor("sk_live_same_value_different_session", "STRIPE_LIVE_KEY", "sk_live_")
	if a == b {
		t.Errorf("same value in different sessions should NOT produce the same placeholder")
	}
}

func TestResolveRoundTrip(t *testing.T) {
	m := New()
	value := "sk_live_4eC39HqLyjWDarjtT1zdp7dc"
	p := m.PlaceholderFor(value, "STRIPE_LIVE_KEY", "sk_live_")
	got, ok := m.Resolve(p)
	if !ok {
		t.Fatal("resolve should succeed")
	}
	if got != value {
		t.Errorf("resolved %q, want %q", got, value)
	}
}

func TestResolveFailClosed(t *testing.T) {
	m := New()
	// Manually construct a placeholder-shaped string that was never mapped.
	_, ok := m.Resolve("[FAKE_KEY:whatever:deadbeef]")
	if ok {
		t.Error("unknown placeholder should fail closed")
	}
}

func TestCompare(t *testing.T) {
	m := New()
	v := "sk_live_same_value_here_for_test_xxx"
	a := m.PlaceholderFor(v, "STRIPE_LIVE_KEY", "sk_live_")
	b := m.PlaceholderFor(v, "STRIPE_LIVE_KEY", "sk_live_")
	if !m.Compare(a, b) {
		t.Error("same value should compare equal")
	}
	c := m.PlaceholderFor("sk_live_different_value_x_x_x_x_x_x", "STRIPE_LIVE_KEY", "sk_live_")
	if m.Compare(a, c) {
		t.Error("different values should not compare equal")
	}
}

func TestPlaceholderFormat(t *testing.T) {
	m := New()
	p := m.PlaceholderFor("sk_live_abcdefghijklmnopqrstuvwx", "STRIPE_LIVE_KEY", "sk_live_")
	if !strings.HasPrefix(p, "[STRIPE_LIVE_KEY:sk_live_...:") {
		t.Errorf("unexpected placeholder format: %q", p)
	}
	if !strings.HasSuffix(p, "]") {
		t.Errorf("placeholder must end with ]: %q", p)
	}
}

func TestPlaceholderRe(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"[STRIPE_LIVE_KEY:sk_live_...:abcd1234]", true},
		{"[HIGH_ENTROPY:abcd1234]", true},
		{"not a placeholder", false},
		{"STRIPE_LIVE_KEY:sk_live_", false},
	}
	for _, tc := range cases {
		got := PlaceholderRe.MatchString(tc.in)
		if got != tc.want {
			t.Errorf("PlaceholderRe.Match(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
