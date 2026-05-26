package cmdinstall

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readJSON(t *testing.T, path string) map[string]any {
	t.Helper()
	buf, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(buf, &m); err != nil {
		t.Fatalf("unmarshal %s: %v\n%s", path, err, buf)
	}
	return m
}

func TestInstallIntoMissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "settings.json")
	d, err := Install(path, "/opt/shhh/bin/shhh", "claude-code")
	if err != nil {
		t.Fatal(err)
	}
	if d == "" {
		t.Error("expected non-empty diff for fresh install")
	}
	m := readJSON(t, path)
	hooks, ok := m["hooks"].(map[string]any)
	if !ok {
		t.Fatalf("missing hooks map: %v", m)
	}
	pre, ok := hooks["PreToolUse"].([]any)
	if !ok || len(pre) != 2 {
		t.Fatalf("PreToolUse should have 2 matcher entries (Read, Bash), got %v", hooks["PreToolUse"])
	}
	end, ok := hooks["SessionEnd"].([]any)
	if !ok || len(end) != 1 {
		t.Fatalf("SessionEnd should have 1 entry, got %v", hooks["SessionEnd"])
	}
}

func TestInstallIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if _, err := Install(path, "/opt/shhh/bin/shhh", "claude-code"); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	d, err := Install(path, "/opt/shhh/bin/shhh", "claude-code")
	if err != nil {
		t.Fatal(err)
	}
	if d != "" {
		t.Errorf("second install should be a no-op, got diff:\n%s", d)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("second install mutated file:\n--- before\n%s--- after\n%s", before, after)
	}
}

func TestInstallPreservesExistingConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	original := map[string]any{
		"model":      "claude-opus-4-6",
		"autoUpdate": false,
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"matcher": "Read",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "/usr/local/bin/other-hook.sh",
						},
					},
				},
			},
		},
	}
	out, _ := json.MarshalIndent(original, "", "  ")
	os.WriteFile(path, out, 0o600)

	if _, err := Install(path, "/opt/shhh/bin/shhh", "claude-code"); err != nil {
		t.Fatal(err)
	}
	m := readJSON(t, path)
	if m["model"] != "claude-opus-4-6" {
		t.Errorf("top-level config lost: %v", m["model"])
	}
	// Read matcher should now have BOTH the original hook and the shhh hook.
	pre := m["hooks"].(map[string]any)["PreToolUse"].([]any)
	var readEntry map[string]any
	for _, e := range pre {
		em := e.(map[string]any)
		if em["matcher"] == "Read" {
			readEntry = em
			break
		}
	}
	if readEntry == nil {
		t.Fatalf("Read matcher entry missing: %v", pre)
	}
	innerHooks := readEntry["hooks"].([]any)
	if len(innerHooks) != 2 {
		t.Fatalf("Read hooks should have 2 entries (existing + shhh), got %d: %v", len(innerHooks), innerHooks)
	}
}

func TestUninstallRemovesOnlyShhhEntries(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	original := map[string]any{
		"hooks": map[string]any{
			"PreToolUse": []any{
				map[string]any{
					"matcher": "Read",
					"hooks": []any{
						map[string]any{
							"type":    "command",
							"command": "/usr/local/bin/other-hook.sh",
						},
					},
				},
			},
		},
	}
	out, _ := json.MarshalIndent(original, "", "  ")
	os.WriteFile(path, out, 0o600)

	if _, err := Install(path, "/opt/shhh/bin/shhh", "claude-code"); err != nil {
		t.Fatal(err)
	}
	if _, err := Uninstall(path, "claude-code"); err != nil {
		t.Fatal(err)
	}

	m := readJSON(t, path)
	hooks := m["hooks"].(map[string]any)
	pre := hooks["PreToolUse"].([]any)
	if len(pre) != 1 {
		t.Fatalf("PreToolUse should have 1 entry (the original non-shhh Read hook), got %v", pre)
	}
	em := pre[0].(map[string]any)
	if em["matcher"] != "Read" {
		t.Errorf("matcher: %v", em["matcher"])
	}
	inner := em["hooks"].([]any)
	if len(inner) != 1 {
		t.Fatalf("inner hooks: %v", inner)
	}
	hm := inner[0].(map[string]any)
	if hm["command"] != "/usr/local/bin/other-hook.sh" {
		t.Errorf("non-shhh hook removed: %v", hm)
	}
	// SessionEnd should be gone (shhh was the only consumer).
	if _, ok := hooks["SessionEnd"]; ok {
		t.Errorf("SessionEnd should be removed, got %v", hooks["SessionEnd"])
	}
}

func TestUninstallRoundTripRestoresFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	orig := []byte(`{
  "model": "claude-opus-4-6"
}
`)
	os.WriteFile(path, orig, 0o600)

	if _, err := Install(path, "/opt/shhh/bin/shhh", "claude-code"); err != nil {
		t.Fatal(err)
	}
	if _, err := Uninstall(path, "claude-code"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	var a, b map[string]any
	json.Unmarshal(orig, &a)
	json.Unmarshal(got, &b)
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	if string(aj) != string(bj) {
		t.Errorf("round-trip not clean:\n  orig: %s\n  got:  %s", aj, bj)
	}
}

func TestInstallQuotesPathWithSpace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	if _, err := Install(path, "/Users/Alice Name/bin/shhh", "claude-code"); err != nil {
		t.Fatal(err)
	}
	buf, _ := os.ReadFile(path)
	if !strings.Contains(string(buf), `\"/Users/Alice Name/bin/shhh\" hook claude-code`) {
		t.Errorf("expected quoted path in settings:\n%s", buf)
	}
}
