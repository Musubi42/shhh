package cmdinstall

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestInstallShhhAllowCommand_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, ".claude", "settings.json")
	if err := os.MkdirAll(filepath.Dir(settings), 0o700); err != nil {
		t.Fatal(err)
	}
	created, err := installShhhAllowCommand(settings)
	if err != nil {
		t.Fatalf("install: %v", err)
	}
	if !created {
		t.Errorf("expected created=true on first install")
	}
	body, err := os.ReadFile(filepath.Join(dir, ".claude", "commands", "shhh-allow.md"))
	if err != nil {
		t.Fatalf("read command file: %v", err)
	}
	if !strings.Contains(string(body), "shhh allow") {
		t.Errorf("command body missing `shhh allow` invocation, got: %q", string(body))
	}
	if !strings.Contains(string(body), "SHHH_SESSION_ID") {
		t.Errorf("command body should reference SHHH_SESSION_ID, got: %q", string(body))
	}
}

func TestInstallShhhAllowCommand_IsIdempotent(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, ".claude", "settings.json")
	if _, err := installShhhAllowCommand(settings); err != nil {
		t.Fatalf("first install: %v", err)
	}
	created, err := installShhhAllowCommand(settings)
	if err != nil {
		t.Fatalf("second install: %v", err)
	}
	if created {
		t.Errorf("second install should report no change")
	}
}

func TestUninstallShhhAllowCommand_RemovesFile(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, ".claude", "settings.json")
	if _, err := installShhhAllowCommand(settings); err != nil {
		t.Fatalf("install: %v", err)
	}
	removed, err := uninstallShhhAllowCommand(settings)
	if err != nil {
		t.Fatalf("uninstall: %v", err)
	}
	if !removed {
		t.Errorf("expected removed=true after install")
	}
	if _, err := os.Stat(shhhAllowCommandPath(settings)); !os.IsNotExist(err) {
		t.Errorf("command file should be gone, stat err = %v", err)
	}
}

func TestUninstallShhhAllowCommand_NoFileIsNoop(t *testing.T) {
	dir := t.TempDir()
	settings := filepath.Join(dir, ".claude", "settings.json")
	removed, err := uninstallShhhAllowCommand(settings)
	if err != nil {
		t.Fatalf("uninstall on missing: %v", err)
	}
	if removed {
		t.Errorf("expected removed=false when file did not exist")
	}
}
