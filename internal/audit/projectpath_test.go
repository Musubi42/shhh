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
