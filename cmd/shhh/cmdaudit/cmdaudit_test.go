package cmdaudit

import (
	"reflect"
	"testing"
)

// TestSplitFlagsAndPositionals locks down the fix for the
// 2026-05-26 dryrun bug where `shhh audit . --no-serve` silently
// ignored --no-serve because Go's flag.Parse stops at the first
// positional. The helper must reorder args so flags work
// regardless of where they appear relative to positional paths.
func TestSplitFlagsAndPositionals(t *testing.T) {
	cases := []struct {
		name           string
		args           []string
		wantFlags      []string
		wantPositional []string
	}{
		{
			name:           "flags before positional",
			args:           []string{"--no-serve", "--no-select", "."},
			wantFlags:      []string{"--no-serve", "--no-select"},
			wantPositional: []string{"."},
		},
		{
			name:           "flags after positional (the regression)",
			args:           []string{".", "--no-serve", "--no-select"},
			wantFlags:      []string{"--no-serve", "--no-select"},
			wantPositional: []string{"."},
		},
		{
			name:           "interleaved",
			args:           []string{"--no-serve", "~/work/a", "--html", "~/work/b"},
			wantFlags:      []string{"--no-serve", "--html"},
			wantPositional: []string{"~/work/a", "~/work/b"},
		},
		{
			name:           "single-dash also counts as flag",
			args:           []string{"-json", "."},
			wantFlags:      []string{"-json"},
			wantPositional: []string{"."},
		},
		{
			name: "no args at all",
			args: nil,
		},
		{
			name:           "only positional",
			args:           []string{".", "~/x"},
			wantPositional: []string{".", "~/x"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f, p := splitFlagsAndPositionals(tc.args)
			if !reflect.DeepEqual(f, tc.wantFlags) {
				t.Errorf("flags = %v, want %v", f, tc.wantFlags)
			}
			if !reflect.DeepEqual(p, tc.wantPositional) {
				t.Errorf("positionals = %v, want %v", p, tc.wantPositional)
			}
		})
	}
}
