package session

import (
	"strings"
	"testing"
)

func TestPlaceholderDeterministic(t *testing.T) {
	m := New()
	a := m.PlaceholderFor("sk_live_abcDEF123456789012345678", "STRIPE_LIVE_KEY", "sk_live_", "")
	b := m.PlaceholderFor("sk_live_abcDEF123456789012345678", "STRIPE_LIVE_KEY", "sk_live_", "")
	if a != b {
		t.Errorf("same value should map to same placeholder: %q vs %q", a, b)
	}
}

func TestPlaceholderDistinctValues(t *testing.T) {
	m := New()
	a := m.PlaceholderFor("sk_live_aaaaaaaaaaaaaaaaaaaaaaaa", "STRIPE_LIVE_KEY", "sk_live_", "")
	b := m.PlaceholderFor("sk_live_bbbbbbbbbbbbbbbbbbbbbbbb", "STRIPE_LIVE_KEY", "sk_live_", "")
	if a == b {
		t.Errorf("different values should map to different placeholders")
	}
}

func TestPlaceholderSessionSalted(t *testing.T) {
	m1 := New()
	m2 := New()
	a := m1.PlaceholderFor("sk_live_same_value_different_session", "STRIPE_LIVE_KEY", "sk_live_", "")
	b := m2.PlaceholderFor("sk_live_same_value_different_session", "STRIPE_LIVE_KEY", "sk_live_", "")
	if a == b {
		t.Errorf("same value in different sessions should NOT produce the same placeholder")
	}
}

func TestResolveRoundTrip(t *testing.T) {
	m := New()
	value := "sk_live_4eC39HqLyjWDarjtT1zdp7dc"
	p := m.PlaceholderFor(value, "STRIPE_LIVE_KEY", "sk_live_", "")
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
	a := m.PlaceholderFor(v, "STRIPE_LIVE_KEY", "sk_live_", "")
	b := m.PlaceholderFor(v, "STRIPE_LIVE_KEY", "sk_live_", "")
	if !m.Compare(a, b) {
		t.Error("same value should compare equal")
	}
	c := m.PlaceholderFor("sk_live_different_value_x_x_x_x_x_x", "STRIPE_LIVE_KEY", "sk_live_", "")
	if m.Compare(a, c) {
		t.Error("different values should not compare equal")
	}
}

func TestPlaceholderFormat(t *testing.T) {
	m := New()
	p := m.PlaceholderFor("sk_live_abcdefghijklmnopqrstuvwx", "STRIPE_LIVE_KEY", "sk_live_", "")
	if !strings.HasPrefix(p, "[STRIPE_LIVE_KEY:sk_live_...:") {
		t.Errorf("unexpected placeholder format: %q", p)
	}
	if !strings.HasSuffix(p, "]") {
		t.Errorf("placeholder must end with ]: %q", p)
	}
}

func TestPlaceholderFormat_Structural(t *testing.T) {
	m := New()
	// Structural description overrides the PublicPrefix+... rendering.
	p := m.PlaceholderFor("postgresql://admin:pw@host:5432/db", "POSTGRES_CONNSTRING", "postgresql://", "admin@host:5432/db")
	if !strings.HasPrefix(p, "[POSTGRES_CONNSTRING:admin@host:5432/db:") {
		t.Errorf("structural placeholder shape wrong: %q", p)
	}
	if strings.Contains(p, "...") {
		t.Errorf("structural placeholder should not contain `...`: %q", p)
	}
	// Two values with the same structural description but different
	// bodies must produce different suffixes.
	a := m.PlaceholderFor("postgresql://admin:pw1@host:5432/db", "POSTGRES_CONNSTRING", "postgresql://", "admin@host:5432/db")
	b := m.PlaceholderFor("postgresql://admin:pw2@host:5432/db", "POSTGRES_CONNSTRING", "postgresql://", "admin@host:5432/db")
	if a == b {
		t.Error("different values with same structural desc should produce different suffixes")
	}
	// But the structural bodies must be byte-identical so an agent
	// comparing the two placeholders can see that only the hash
	// differs.
	aBody := strings.TrimSuffix(strings.TrimPrefix(a, "[POSTGRES_CONNSTRING:"), "]")
	bBody := strings.TrimSuffix(strings.TrimPrefix(b, "[POSTGRES_CONNSTRING:"), "]")
	// Strip the trailing :hash segment from each.
	if idx := strings.LastIndex(aBody, ":"); idx >= 0 {
		aBody = aBody[:idx]
	}
	if idx := strings.LastIndex(bBody, ":"); idx >= 0 {
		bBody = bBody[:idx]
	}
	if aBody != bBody {
		t.Errorf("structural bodies should match: %q vs %q", aBody, bBody)
	}
}

func TestSnapshotRoundTrip(t *testing.T) {
	m1 := New()
	v1 := "sk_live_aaaaaaaaaaaaaaaaaaaaaaaa"
	v2 := "sk_live_bbbbbbbbbbbbbbbbbbbbbbbb"
	p1 := m1.PlaceholderFor(v1, "STRIPE_LIVE_KEY", "sk_live_", "")
	p2 := m1.PlaceholderFor(v2, "STRIPE_LIVE_KEY", "sk_live_", "")

	salt, entries := m1.Snapshot()
	if salt == "" {
		t.Fatal("snapshot salt must be non-empty")
	}
	if len(entries) != 2 {
		t.Fatalf("snapshot entries = %d, want 2", len(entries))
	}

	m2, err := FromSnapshot(salt, entries)
	if err != nil {
		t.Fatalf("FromSnapshot: %v", err)
	}
	// Resolve survives.
	if got, ok := m2.Resolve(p1); !ok || got != v1 {
		t.Errorf("resolve p1: got (%q,%v), want (%q,true)", got, ok, v1)
	}
	if got, ok := m2.Resolve(p2); !ok || got != v2 {
		t.Errorf("resolve p2: got (%q,%v), want (%q,true)", got, ok, v2)
	}
	// New values produce placeholders consistent with the original salt:
	// re-adding v1 must return the same placeholder.
	if p := m2.PlaceholderFor(v1, "STRIPE_LIVE_KEY", "sk_live_", ""); p != p1 {
		t.Errorf("placeholder for v1 after round-trip: got %q, want %q", p, p1)
	}
	// And a *new* value assigned in m2 matches what m1 would have assigned,
	// because the salt carried over.
	v3 := "sk_live_cccccccccccccccccccccccc"
	pA := m1.PlaceholderFor(v3, "STRIPE_LIVE_KEY", "sk_live_", "")
	pB := m2.PlaceholderFor(v3, "STRIPE_LIVE_KEY", "sk_live_", "")
	if pA != pB {
		t.Errorf("salt not carried across snapshot: %q vs %q", pA, pB)
	}
}

func TestFromSnapshotBadSalt(t *testing.T) {
	if _, err := FromSnapshot("", nil); err == nil {
		t.Error("empty salt should error")
	}
	if _, err := FromSnapshot("zz", nil); err == nil {
		t.Error("non-hex salt should error")
	}
}

func TestPlaceholderRe(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"[STRIPE_LIVE_KEY:sk_live_...:abcd1234]", true},
		{"[HIGH_ENTROPY:abcd1234]", true},
		{"[POSTGRES_CONNSTRING:admin@prod-db:5432/myapp:abcd1234]", true},
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
