package cmdallow

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Musubi42/shhh/cmd/shhh/cmdhook"
)

func TestRun_AddPersistsToAllowStore(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	var out, errOut bytes.Buffer
	if err := run([]string{"--session-id", "test-cli", "--add", "STRIPE_LIVE_KEY"}, &out, &errOut); err != nil {
		t.Fatalf("run: %v (stderr=%s)", err, errOut.String())
	}
	allowed, err := cmdhook.LoadAllow("test-cli")
	if err != nil {
		t.Fatalf("LoadAllow: %v", err)
	}
	if _, ok := allowed["STRIPE_LIVE_KEY"]; !ok {
		t.Errorf("STRIPE_LIVE_KEY not in allow store, got %v", allowed)
	}
	if !strings.Contains(out.String(), "STRIPE_LIVE_KEY") {
		t.Errorf("confirmation message missing name, got %q", out.String())
	}
	if !strings.Contains(out.String(), "24h") {
		t.Errorf("confirmation should mention 24h lifetime, got %q", out.String())
	}
}

func TestRun_FallsBackToEnvSessionID(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	t.Setenv("SHHH_SESSION_ID", "env-sess-xyz")
	var out, errOut bytes.Buffer
	if err := run([]string{"--add", "ANTHROPIC_API_KEY"}, &out, &errOut); err != nil {
		t.Fatalf("run: %v (stderr=%s)", err, errOut.String())
	}
	allowed, err := cmdhook.LoadAllow("env-sess-xyz")
	if err != nil {
		t.Fatalf("LoadAllow: %v", err)
	}
	if _, ok := allowed["ANTHROPIC_API_KEY"]; !ok {
		t.Errorf("expected env-derived session id to receive the allow, got %v", allowed)
	}
}

func TestRun_MissingSessionIDErrors(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	t.Setenv("SHHH_SESSION_ID", "")
	var out, errOut bytes.Buffer
	err := run([]string{"--add", "X"}, &out, &errOut)
	if err == nil {
		t.Fatal("expected error when no session id is available")
	}
}

func TestRun_NothingToDoErrors(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	var out, errOut bytes.Buffer
	err := run([]string{"--session-id", "x"}, &out, &errOut)
	if err == nil {
		t.Fatal("expected error when neither --add nor --list provided")
	}
}

func TestRun_ListShowsAllowedNames(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	if err := cmdhook.AddAllow("listing", "STRIPE_LIVE_KEY"); err != nil {
		t.Fatalf("AddAllow: %v", err)
	}
	var out, errOut bytes.Buffer
	if err := run([]string{"--session-id", "listing", "--list"}, &out, &errOut); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !strings.Contains(out.String(), "STRIPE_LIVE_KEY") {
		t.Errorf("--list missing STRIPE_LIVE_KEY, got %q", out.String())
	}
}
