package cmdbench

import (
	"os"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Musubi42/shhh/internal/detector"
)

// helper: build a benchReport from (engine, file, label, start, end, value) tuples
func mkReport(engines []string, findings map[string][]struct {
	File           string
	Label          string
	Start, End     int
	Value          string
}) *benchReport {
	r := &benchReport{}
	for _, eng := range engines {
		er := &engineResult{Engine: eng}
		for _, f := range findings[eng] {
			er.Findings = append(er.Findings, findingRef{
				File: f.File,
				Finding: detector.Finding{
					Label: f.Label,
					Start: f.Start,
					End:   f.End,
					Value: f.Value,
				},
			})
		}
		r.Engines = append(r.Engines, er)
	}
	return r
}

func TestBuildFindingDetailsCrossEngineMerge(t *testing.T) {
	// Two engines flag the same Stripe key with slightly
	// different offsets — shhh-native includes the `KEY=` prefix,
	// gitleaks just the value. Must merge into ONE finding with
	// Engines=[shhh-native, gitleaks].
	r := mkReport([]string{"shhh-native", "gitleaks"}, map[string][]struct {
		File       string
		Label      string
		Start, End int
		Value      string
	}{
		"shhh-native": {
			{File: "x.env", Label: "STRIPE_LIVE_KEY", Start: 0, End: 40, Value: "STRIPE_KEY=sk_live_4eC39HqLyjWDarjtT1zdp7"},
		},
		"gitleaks": {
			{File: "x.env", Label: "STRIPE_LIVE_KEY", Start: 11, End: 40, Value: "sk_live_4eC39HqLyjWDarjtT1zdp7"},
		},
	})
	details := buildFindingDetails(r)
	if len(details) != 1 {
		t.Fatalf("want 1 merged finding, got %d", len(details))
	}
	got := details[0]
	if !reflect.DeepEqual(got.Engines, []string{"shhh-native", "gitleaks"}) {
		t.Errorf("engines = %v, want [shhh-native gitleaks]", got.Engines)
	}
	if !strings.HasPrefix(got.Placeholder, "[STRIPE_LIVE_KEY:") {
		t.Errorf("placeholder = %q, want STRIPE_LIVE_KEY prefix", got.Placeholder)
	}
}

func TestBuildFindingDetailsDistinctNonOverlapping(t *testing.T) {
	// Two non-overlapping findings in the same file → two
	// separate details, each attributed to its lone engine.
	r := mkReport([]string{"shhh-native", "gitleaks"}, map[string][]struct {
		File       string
		Label      string
		Start, End int
		Value      string
	}{
		"shhh-native": {
			{File: "x.env", Label: "STRIPE_LIVE_KEY", Start: 0, End: 30, Value: "sk_live_aaaa"},
		},
		"gitleaks": {
			{File: "x.env", Label: "GENERIC_API_KEY", Start: 50, End: 80, Value: "random-token"},
		},
	})
	details := buildFindingDetails(r)
	if len(details) != 2 {
		t.Fatalf("want 2 separate findings, got %d", len(details))
	}
}

func TestLineComputerSnippetScrubsOtherValues(t *testing.T) {
	// go.sum-style: two raw values on one line. The snippet for
	// the SECOND value's prefix would otherwise leak the first
	// value verbatim. The scrub pass must replace it with
	// [redacted] before the placeholder lands.
	dir := t.TempDir()
	file := dir + "/go.sum"
	line := "github.com/foo/bar v1.0.0 h1:FIRSTHASHDEADBEEFFAKE12345/go.mod h1:SECONDHASHDEADBEEFFAKE67890\n"
	if err := os.WriteFile(file, []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
	first := "FIRSTHASHDEADBEEFFAKE12345"
	second := "SECONDHASHDEADBEEFFAKE67890"
	secondStart := strings.Index(line, second)

	lc := newLineComputer()
	snippet := lc.snippet(file, secondStart, "[HIGH_ENTROPY:abcdef]", []string{first, second})

	if strings.Contains(snippet, first) {
		t.Fatalf("snippet leaked the first raw value: %s", snippet)
	}
	if strings.Contains(snippet, second) {
		t.Fatalf("snippet leaked the current raw value: %s", snippet)
	}
	if !strings.Contains(snippet, "[HIGH_ENTROPY:abcdef]") {
		t.Errorf("placeholder missing from snippet: %s", snippet)
	}
	if !strings.Contains(snippet, "[redacted]") {
		t.Errorf("first value not scrubbed to [redacted]: %s", snippet)
	}
}

func TestLineComputerSnippetTruncatesLongPrefix(t *testing.T) {
	dir := t.TempDir()
	file := dir + "/wide.txt"
	prefix := strings.Repeat("x", 300)
	if err := os.WriteFile(file, []byte(prefix+"SECRETVAL\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	lc := newLineComputer()
	snippet := lc.snippet(file, 300, "[X:abc]", nil)
	// 120 char budget; "…" is 3 bytes UTF-8 so byte len ≤ 122.
	if len(snippet) > 122 {
		t.Errorf("snippet not truncated: len=%d", len(snippet))
	}
	if !strings.HasPrefix(snippet, "…") {
		t.Errorf("expected leading ellipsis: %s", snippet)
	}
	if !strings.HasSuffix(snippet, "[X:abc]") {
		t.Errorf("placeholder dropped on truncation: %s", snippet)
	}
}

func TestMakePlaceholderRedacts(t *testing.T) {
	// Critical invariant: the raw value MUST NOT appear in the
	// placeholder. This test fails closed if anyone ever changes
	// the format to embed the value (e.g. for "debugging").
	p := makePlaceholder("STRIPE_LIVE_KEY", "sk_live_VERYSECRETVALUE12345")
	if strings.Contains(p, "VERYSECRETVALUE") {
		t.Fatalf("placeholder leaked raw value: %s", p)
	}
	if !strings.HasPrefix(p, "[STRIPE_LIVE_KEY:sk_live_") {
		t.Errorf("placeholder = %s, want STRIPE_LIVE_KEY:sk_live_… prefix", p)
	}
}

func TestBuildLabelGroupsHierarchy(t *testing.T) {
	details := []findingDetail{
		{Label: "HIGH_ENTROPY", File: "/repo/go.sum", Line: 1, Engines: []string{"shhh-native"}},
		{Label: "HIGH_ENTROPY", File: "/repo/go.sum", Line: 2, Engines: []string{"shhh-native"}},
		{Label: "HIGH_ENTROPY", File: "/repo/foo.txt", Line: 5, Engines: []string{"shhh-native"}},
		{Label: "STRIPE_LIVE_KEY", File: "/repo/.env", Line: 1, Engines: []string{"shhh-native", "gitleaks"}},
	}
	groups := buildLabelGroups(details, "/repo")
	if len(groups) != 2 {
		t.Fatalf("want 2 label groups, got %d", len(groups))
	}
	// HIGH_ENTROPY first (count desc): 3 findings across 2 files.
	he := groups[0]
	if he.Label != "HIGH_ENTROPY" || he.Count != 3 {
		t.Errorf("first group = %+v, want HIGH_ENTROPY×3", he)
	}
	if len(he.Files) != 2 {
		t.Fatalf("HIGH_ENTROPY should have 2 file groups, got %d", len(he.Files))
	}
	// File go.sum first (count=2), then foo.txt (count=1)
	if he.Files[0].DisplayPath != "go.sum" || he.Files[0].Count != 2 {
		t.Errorf("first file = %+v, want go.sum×2", he.Files[0])
	}
	if he.Files[1].DisplayPath != "foo.txt" || he.Files[1].Count != 1 {
		t.Errorf("second file = %+v, want foo.txt×1", he.Files[1])
	}
	// Items inside go.sum sorted by line ascending.
	if he.Files[0].Items[0].Line != 1 || he.Files[0].Items[1].Line != 2 {
		t.Errorf("items not sorted by line: %+v", he.Files[0].Items)
	}
	// STRIPE_LIVE_KEY: engines union shhh-native + gitleaks.
	stripe := groups[1]
	sort.Strings(stripe.Engines)
	wantEng := []string{"gitleaks", "shhh-native"}
	sort.Strings(wantEng)
	if !reflect.DeepEqual(stripe.Engines, wantEng) {
		t.Errorf("STRIPE engines = %v, want %v", stripe.Engines, wantEng)
	}
}

func TestCommonDir(t *testing.T) {
	cases := []struct {
		name  string
		paths []string
		want  string
	}{
		{"empty", nil, ""},
		{"single", []string{"/a/b/c.txt"}, "/a/b"},
		{"shared dir", []string{"/a/b/c.txt", "/a/b/d.txt"}, "/a/b"},
		{"nested", []string{"/a/b/c.txt", "/a/d/e.txt"}, "/a"},
		{"no common", []string{"/a/b.txt", "/x/y.txt"}, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := commonDir(tc.paths); got != tc.want {
				t.Errorf("commonDir(%v) = %q, want %q", tc.paths, got, tc.want)
			}
		})
	}
}

func TestDisplayPath(t *testing.T) {
	if got := displayPath("/repo/go.sum", "/repo"); got != "go.sum" {
		t.Errorf("relative strip failed: %s", got)
	}
	// No scan root → tilde fallback. We can't predict $HOME but
	// we can assert that the path is returned (possibly tildified).
	got := displayPath("/tmp/xyz/file.txt", "")
	if !strings.Contains(got, "file.txt") {
		t.Errorf("displayPath without root lost filename: %s", got)
	}
	// Prefix-trap: /repo-other must NOT be stripped by /repo.
	if got := displayPath("/repo-other/f", "/repo"); got == "f" {
		t.Errorf("prefix trap: %s should not be stripped by /repo", got)
	}
}
