package cmdhook

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

const testSession = "test-session-1234"

func withTempCache(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("SHHH_CACHE_DIR", dir)
}

func TestLoadAllow_MissingReturnsEmpty(t *testing.T) {
	withTempCache(t)
	got, err := LoadAllow(testSession)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty set, got %v", got)
	}
}

func TestAddAllow_PersistsAndIsIdempotent(t *testing.T) {
	withTempCache(t)
	if err := AddAllow(testSession, "ANTHROPIC_API_KEY"); err != nil {
		t.Fatalf("AddAllow: %v", err)
	}
	if err := AddAllow(testSession, "ANTHROPIC_API_KEY"); err != nil {
		t.Fatalf("AddAllow idempotent: %v", err)
	}
	if err := AddAllow(testSession, "STRIPE_LIVE_KEY"); err != nil {
		t.Fatalf("AddAllow second name: %v", err)
	}
	got, err := LoadAllow(testSession)
	if err != nil {
		t.Fatalf("LoadAllow: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 entries, got %v", got)
	}
	if _, ok := got["ANTHROPIC_API_KEY"]; !ok {
		t.Errorf("missing ANTHROPIC_API_KEY")
	}
	if _, ok := got["STRIPE_LIVE_KEY"]; !ok {
		t.Errorf("missing STRIPE_LIVE_KEY")
	}
}

func TestLoadAllow_RejectsInvalidSessionID(t *testing.T) {
	withTempCache(t)
	if _, err := LoadAllow("../escape"); err == nil {
		t.Fatal("expected error for invalid session_id, got nil")
	}
}

func TestLoadAllow_StaleFileTreatedAsEmpty(t *testing.T) {
	withTempCache(t)
	if err := AddAllow(testSession, "STRIPE_LIVE_KEY"); err != nil {
		t.Fatalf("AddAllow: %v", err)
	}
	path, _ := allowPath(testSession)
	old := time.Now().Add(-(allowTTL + time.Hour))
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}
	got, err := LoadAllow(testSession)
	if err != nil {
		t.Fatalf("LoadAllow: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected stale file to be ignored, got %v", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("expected stale file to be removed, stat err = %v", err)
	}
}

func TestLoadAllow_CorruptFileTreatedAsEmpty(t *testing.T) {
	withTempCache(t)
	dir, _ := sessionDir(testSession)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path, _ := allowPath(testSession)
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got, err := LoadAllow(testSession)
	if err != nil {
		t.Fatalf("LoadAllow: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("expected empty on corrupt, got %v", got)
	}
}

func TestGCStaleSessions_RemovesOldDirs(t *testing.T) {
	withTempCache(t)
	// Create a fresh session and a stale one.
	if err := AddAllow("fresh-session", "X"); err != nil {
		t.Fatalf("setup fresh: %v", err)
	}
	if err := AddAllow("stale-session", "Y"); err != nil {
		t.Fatalf("setup stale: %v", err)
	}
	staleDir, _ := sessionDir("stale-session")
	old := time.Now().Add(-(allowTTL + time.Hour))
	if err := os.Chtimes(staleDir, old, old); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	if err := GCStaleSessions(); err != nil {
		t.Fatalf("GCStaleSessions: %v", err)
	}

	freshDir, _ := sessionDir("fresh-session")
	if _, err := os.Stat(freshDir); err != nil {
		t.Errorf("fresh session should still exist: %v", err)
	}
	if _, err := os.Stat(staleDir); !os.IsNotExist(err) {
		t.Errorf("stale session should be removed, stat err = %v", err)
	}
}

func TestGCStaleSessions_MissingRootIsNotError(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("SHHH_CACHE_DIR", dir)
	// No sessions dir created at all.
	if err := GCStaleSessions(); err != nil {
		t.Fatalf("expected nil for missing root, got %v", err)
	}
}

func TestAddAllow_RejectsEmptyName(t *testing.T) {
	withTempCache(t)
	if err := AddAllow(testSession, ""); err == nil {
		t.Fatal("expected error for empty name")
	}
}

// Sanity check that allow file lives at the documented path.
func TestAllowPath_LayoutMatchesDocs(t *testing.T) {
	withTempCache(t)
	got, err := allowPath(testSession)
	if err != nil {
		t.Fatalf("allowPath: %v", err)
	}
	want := filepath.Join(os.Getenv("SHHH_CACHE_DIR"), "sessions", testSession, "allow.json")
	if got != want {
		t.Errorf("allow path = %q, want %q", got, want)
	}
}
