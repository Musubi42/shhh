package audit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDecodeDashPath(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"-Users-alice-work-backend", "/Users/alice/work/backend"},
		{"-tmp-throwaway", "/tmp/throwaway"},
		{"-Users-musubi42-Documents-Musubi42-shhh", "/Users/musubi42/Documents/Musubi42/shhh"},
		{"", ""},
		{"not-dash-encoded", "not-dash-encoded"},
	}
	for _, tc := range cases {
		got := DecodeDashPath(tc.in)
		if got != tc.want {
			t.Errorf("DecodeDashPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestResolveProjectPath_PrefersTranscriptCwd(t *testing.T) {
	// The dash-decode is lossy on "open-source"; the transcript cwd
	// is loss-less. ResolveProjectPath must prefer the transcript.
	dir := t.TempDir()
	dash := "-Users-alice-open-source-shhh"
	projectDir := filepath.Join(dir, dash)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	jsonl := `{"type":"summary","irrelevant":true}` + "\n" +
		`{"type":"user","cwd":"/Users/alice/open-source/shhh","sessionId":"x"}` + "\n"
	if err := os.WriteFile(filepath.Join(projectDir, "abc.jsonl"), []byte(jsonl), 0o644); err != nil {
		t.Fatal(err)
	}
	got := ResolveProjectPath(dash, projectDir)
	want := "/Users/alice/open-source/shhh"
	if got != want {
		t.Errorf("ResolveProjectPath = %q, want %q (must read cwd from transcript, not dash-decode)", got, want)
	}
}

func TestResolveProjectPath_FallsBackToDashDecode(t *testing.T) {
	// No transcripts → fall back to the lossy decode rather than fail.
	dir := t.TempDir()
	dash := "-tmp-throwaway"
	projectDir := filepath.Join(dir, dash)
	if err := os.MkdirAll(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	got := ResolveProjectPath(dash, projectDir)
	want := "/tmp/throwaway"
	if got != want {
		t.Errorf("ResolveProjectPath = %q, want %q (no transcript → dash-decode fallback)", got, want)
	}
}

func TestEncodeDashPath(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"/Users/alice/work/backend", "-Users-alice-work-backend"},
		{"/tmp/throwaway", "-tmp-throwaway"},
	}
	for _, tc := range cases {
		got := EncodeDashPath(tc.in)
		if got != tc.want {
			t.Errorf("EncodeDashPath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTildePath(t *testing.T) {
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		in, want string
	}{
		{home, "~"},
		{filepath.Join(home, "work/backend"), "~/work/backend"},
		{"/tmp/other", "/tmp/other"},
	}
	for _, tc := range cases {
		got := TildePath(tc.in)
		if got != tc.want {
			t.Errorf("TildePath(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestPathExists(t *testing.T) {
	dir := t.TempDir()
	if !PathExists(dir) {
		t.Errorf("PathExists(%q) = false, want true (dir exists)", dir)
	}
	if PathExists(filepath.Join(dir, "nonexistent")) {
		t.Errorf("PathExists on missing dir should return false")
	}
	if PathExists("") {
		t.Errorf("PathExists(\"\") should return false")
	}

	// A file (not dir) should return false — Claude projects are dirs.
	file := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(file, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if PathExists(file) {
		t.Errorf("PathExists on a file should return false (expected dir)")
	}
}

func TestDashNameCouldMatchScope(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name  string
		dash  string
		roots []string
		want  bool
	}{
		{"no scope = everything matches", "-Users-me-anything", nil, true},
		{"exact match no hyphens",
			"-Users-me-work-backend",
			[]string{"/Users/me/work/backend"}, true},
		{"descendant",
			"-Users-me-work-backend-sub",
			[]string{"/Users/me/work/backend"}, true},
		{"prefix-trap rejected",
			"-Users-me-work-backendother",
			[]string{"/Users/me/work/backend"}, false},
		{"dash ambiguity is OK",
			"-Users-me-open-source-shhh", // could decode as /Users/me/open-source/shhh
			[]string{"/Users/me/open-source/shhh"}, true},
		{"dash ambiguity false-positive (acceptable)",
			"-Users-me-open-source", // matches /Users/me/open/source AND /Users/me/open-source
			[]string{"/Users/me/open-source"}, true},
		{"completely different",
			"-tmp-throwaway",
			[]string{"/Users/me/work"}, false},
		{"multi-root: matches second",
			"-Users-me-work-api",
			[]string{"/nowhere", "/Users/me/work"}, true},
		{"shorter than prefix",
			"-Users",
			[]string{"/Users/me/work"}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := DashNameCouldMatchScope(tc.dash, tc.roots); got != tc.want {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}
