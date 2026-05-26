package cmdinstall

import (
	"strings"
	"testing"
)

func TestRenderChangeAddNoColor(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("HOME", "/Users/x")
	got := renderChange(changeReport{
		path:      "/Users/x/.claude/settings.json",
		direction: directionAdd,
		hooks: []managedHook{
			{"PreToolUse", "Read"},
			{"PreToolUse", "Bash"},
			{"SessionEnd", ""},
		},
		command:       "/Users/x/.local/bin/shhh hook claude-code",
		preservedKeys: 5,
	})
	wants := []string{
		"+ PreToolUse  matcher=Read",
		"+ PreToolUse  matcher=Bash",
		"+ SessionEnd  *",
		"~/.local/bin/shhh hook claude-code", // tilde-abbreviated
		"3 hooks added",
		"5 existing settings preserved",
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
	// No ANSI escapes when NO_COLOR is set.
	if strings.Contains(got, "\x1b[") {
		t.Errorf("NO_COLOR did not suppress ANSI:\n%s", got)
	}
}

func TestRenderChangeRemoveSingular(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	got := renderChange(changeReport{
		direction:     directionRemove,
		hooks:         []managedHook{{"SessionEnd", ""}},
		command:       "/x/shhh hook claude-code",
		preservedKeys: 1,
	})
	if !strings.Contains(got, "- SessionEnd") {
		t.Errorf("missing remove line:\n%s", got)
	}
	if !strings.Contains(got, "1 hook removed") {
		t.Errorf("singular noun missing:\n%s", got)
	}
	if !strings.Contains(got, "1 existing setting preserved") {
		t.Errorf("singular 'setting' missing:\n%s", got)
	}
}

func TestRenderChangeQuotedCommand(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	got := renderChange(changeReport{
		direction:     directionAdd,
		hooks:         []managedHook{{"PreToolUse", "Read"}},
		command:       `"/Users/x/My Folder/shhh" hook claude-code`,
		preservedKeys: 0,
	})
	// The on-disk JSON quotes the path; the display strips the
	// outer quotes so tildePath can abbreviate.
	if !strings.Contains(got, "/Users/x/My Folder/shhh") {
		t.Errorf("quoted command not de-quoted for display:\n%s", got)
	}
	// Preserved-keys note suppressed when 0.
	if strings.Contains(got, "0 existing settings preserved") {
		t.Errorf("should suppress preserved=0 note:\n%s", got)
	}
}
