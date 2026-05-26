package cmdbench

import (
	"reflect"
	"strings"
	"testing"

)

func TestParseEngines(t *testing.T) {
	cases := []struct {
		in      string
		want    []string
		wantErr string
	}{
		{"shhh-native,gitleaks", []string{"shhh-native", "gitleaks"}, ""},
		{"shhh-native", []string{"shhh-native"}, ""},
		{"shhh-native, gitleaks", []string{"shhh-native", "gitleaks"}, ""}, // spaces tolerated
		{"", nil, "cannot be empty"},
		{"foo", nil, "unknown engine"},
		{"shhh-native,foo", nil, "unknown engine"},
	}
	for _, tc := range cases {
		t.Run(tc.in, func(t *testing.T) {
			got, err := parseEngines(tc.in)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Errorf("want err containing %q, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestSplitFlagsAndPositionals(t *testing.T) {
	cases := []struct {
		name           string
		in             []string
		wantFlags      []string
		wantPositional []string
	}{
		{"path then flag", []string{"./x", "--no-serve"}, []string{"--no-serve"}, []string{"./x"}},
		{"flag then path", []string{"--no-serve", "./x"}, []string{"--no-serve"}, []string{"./x"}},
		{"only paths", []string{"./a", "./b"}, nil, []string{"./a", "./b"}},
		{"engines= form", []string{"./a", "--engines=shhh-native,gitleaks"}, []string{"--engines=shhh-native,gitleaks"}, []string{"./a"}},
		{"no args", nil, nil, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, p := splitFlagsAndPositionals(tc.in)
			if !reflect.DeepEqual(f, tc.wantFlags) {
				t.Errorf("flags = %v, want %v", f, tc.wantFlags)
			}
			if !reflect.DeepEqual(p, tc.wantPositional) {
				t.Errorf("positional = %v, want %v", p, tc.wantPositional)
			}
		})
	}
}

func TestAgreement(t *testing.T) {
	a := &engineResult{LabelCounts: map[string]int{
		"STRIPE_LIVE_KEY": 3, "GITHUB_PAT": 1, "POSTGRES": 2,
	}}
	b := &engineResult{LabelCounts: map[string]int{
		"STRIPE_LIVE_KEY": 3, "GENERIC_API_KEY": 1, "POSTGRES": 1,
	}}
	shared, onlyA, onlyB := agreement(a, b)
	// Shared = min per label
	if totalCount(shared) != 4 { // STRIPE(3) + POSTGRES(min=1)
		t.Errorf("shared total = %d, want 4 (entries=%v)", totalCount(shared), shared)
	}
	if totalCount(onlyA) != 2 { // GITHUB_PAT(1) + POSTGRES(2-1=1)
		t.Errorf("onlyA total = %d, want 2 (entries=%v)", totalCount(onlyA), onlyA)
	}
	if totalCount(onlyB) != 1 { // GENERIC_API_KEY(1)
		t.Errorf("onlyB total = %d, want 1 (entries=%v)", totalCount(onlyB), onlyB)
	}
}

