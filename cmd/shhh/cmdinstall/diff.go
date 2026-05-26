package cmdinstall

import (
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"
)

// changeReport describes a single Install or Uninstall mutation in a
// shape the renderer can consume without re-parsing JSON. It captures
// only what the user cares about: which hooks moved, the command they
// point at, and how many of their other settings were preserved.
type changeReport struct {
	path          string        // absolute settings.json path
	direction     direction     // add or remove
	hooks         []managedHook // the entries that actually changed
	command       string        // the shhh hook command string, for display
	preservedKeys int           // count of non-"hooks" top-level keys left untouched
}

type direction int

const (
	directionAdd direction = iota
	directionRemove
)

// renderChange formats a changeReport for stdout. The format is a
// compact list of inserted/removed hook entries followed by a one-line
// preservation summary — designed to fit in ~6 lines for the typical
// install. The full file is intentionally NOT echoed; users who want
// to verify the JSON open the file (path is shown above the diff).
//
// ANSI colors are applied only when stdout is a TTY and $NO_COLOR is
// unset (see no-color.org). The colorless rendering is still readable.
func renderChange(r changeReport) string {
	c := pickColors()

	var sb strings.Builder
	sign := "+"
	signColor := c.green
	if r.direction == directionRemove {
		sign = "-"
		signColor = c.red
	}

	// Column-align the matcher labels so the arrows line up. The
	// "SessionEnd" event has no matcher so its label is "*".
	maxLabel := 0
	labels := make([]string, len(r.hooks))
	for i, h := range r.hooks {
		labels[i] = formatHookLabel(h)
		if len(labels[i]) > maxLabel {
			maxLabel = len(labels[i])
		}
	}
	for i, h := range r.hooks {
		label := labels[i]
		pad := strings.Repeat(" ", maxLabel-len(label))
		// Tilde-abbreviate the command path for display only.
		cmdDisplay := tildePath(stripQuotes(r.command))
		fmt.Fprintf(&sb, "  %s%s%s %s%s  %s→%s  %s\n",
			signColor, sign, c.reset,
			label, pad,
			c.dim, c.reset,
			cmdDisplay,
		)
		_ = h
	}

	if len(r.hooks) > 0 {
		sb.WriteString("\n")
	}

	verb := "added"
	if r.direction == directionRemove {
		verb = "removed"
	}
	preservedNote := ""
	if r.preservedKeys > 0 {
		preservedNote = fmt.Sprintf(" %s· %d existing setting%s preserved%s",
			c.dim, r.preservedKeys, plural(r.preservedKeys), c.reset)
	}
	fmt.Fprintf(&sb, "  %d hook%s %s%s\n",
		len(r.hooks), plural(len(r.hooks)), verb, preservedNote)

	return sb.String()
}

// formatHookLabel turns a managedHook into a fixed-width-ish label.
// Examples:
//
//	{event: "PreToolUse", matcher: "Read"} → "PreToolUse  matcher=Read"
//	{event: "SessionEnd", matcher: ""}     → "SessionEnd  *"
func formatHookLabel(h managedHook) string {
	if h.matcher == "" {
		return fmt.Sprintf("%-11s *", h.event)
	}
	return fmt.Sprintf("%-11s matcher=%s", h.event, h.matcher)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// stripQuotes removes surrounding double-quotes from a path, since
// quoteIfNeeded may have wrapped paths with spaces for the on-disk
// JSON. For display we want the raw path so tildePath can recognize
// it.
func stripQuotes(s string) string {
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		return s[1 : len(s)-1]
	}
	return s
}

// ansi is the small set of color codes the diff renderer uses. Empty
// strings disable color cleanly.
type ansi struct {
	green string
	red   string
	dim   string
	reset string
}

// pickColors returns the active color set: real ANSI when stdout is a
// TTY and $NO_COLOR is unset, empty strings otherwise. Honours the
// no-color.org convention.
func pickColors() ansi {
	if os.Getenv("NO_COLOR") != "" {
		return ansi{}
	}
	if !term.IsTerminal(os.Stdout.Fd()) {
		return ansi{}
	}
	return ansi{
		green: "\x1b[32m",
		red:   "\x1b[31m",
		dim:   "\x1b[2m",
		reset: "\x1b[0m",
	}
}
