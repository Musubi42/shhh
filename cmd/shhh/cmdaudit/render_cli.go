// Package cmdaudit implements the `shhh audit` subcommand renderers.
//
// This file provides the ANSI terminal renderer. The machine-readable
// JSON form lives in json_export.go. The subcommand entry point that
// wires the scan, snapshot persistence, and flag parsing lives
// elsewhere in this package.
package cmdaudit

import (
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	auditpkg "github.com/Musubi42/shhh/internal/audit"
)

// ruleWidth is the width of the horizontal separator bars.
const ruleWidth = 69

// pathColumnWidth is the column at which the [STATUS] tag is
// right-aligned in project headers.
const pathColumnWidth = 56

// rotationURLs maps a label prefix to a display name + rotation URL
// shown in the footer action block. Prefix matching is applied
// against Finding.Label.
var rotationURLs = map[string]struct {
	display string
	url     string
}{
	"STRIPE":   {"stripe", "https://dashboard.stripe.com/apikeys"},
	"AWS":      {"aws", "https://console.aws.amazon.com/iam/home#/users"},
	"OPENAI":   {"openai", "https://platform.openai.com/api-keys"},
	"GITHUB":   {"github", "https://github.com/settings/tokens"},
	"SENDGRID": {"sendgrid", "https://app.sendgrid.com/settings/api_keys"},
}

// colorizer wraps conditional ANSI escape emission. When on is false
// every method is a no-op and the string passes through unchanged.
type colorizer struct{ on bool }

func (c colorizer) wrap(code, s string) string {
	if !c.on {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func (c colorizer) red(s string) string    { return c.wrap("31", s) }
func (c colorizer) green(s string) string  { return c.wrap("32", s) }
func (c colorizer) yellow(s string) string { return c.wrap("33", s) }
func (c colorizer) dim(s string) string    { return c.wrap("2", s) }
func (c colorizer) bold(s string) string   { return c.wrap("1", s) }

// RenderCLI writes the terminal form of the audit to w. It emits
// ANSI colors iff useColor is true (caller decides based on
// isatty + NO_COLOR env).
//
// The progress "scanning ..." preamble is NOT written here — that's
// printed by the command entry point during the actual scan, before
// the Result is built. RenderCLI only handles the post-scan output.
func RenderCLI(w io.Writer, r *auditpkg.Result, useColor bool) error {
	c := colorizer{on: useColor}
	var b strings.Builder

	// 1. Header.
	agent := humanizeAgent(r.Agent)
	ts := r.AuditTime.UTC().Format("2006-01-02 15:04:05") + " UTC"
	fmt.Fprintf(&b, "%s  shhh audit — %s · %s\n\n", "🛡️", agent, c.dim(ts))

	rule := c.dim(strings.Repeat("━", ruleWidth))

	// 2. Summary bar.
	fmt.Fprintln(&b, rule)
	s := r.Summary
	fmt.Fprintf(&b, "  %d projects · %d unprotected · %d protected · %d archived · %d clean\n",
		s.ProjectsTotal, s.ProjectsUnprotected, s.ProjectsProtected, s.ProjectsArchived, s.ProjectsClean)
	fmt.Fprintf(&b, "  %s %s   %s %s   %s %s\n",
		"🚨", c.red(fmt.Sprintf("%d leaked", s.SecretsLeaked)),
		"⚠️ ", c.yellow(fmt.Sprintf("%d at risk", s.SecretsAtRisk)),
		"✅", c.green(fmt.Sprintf("%d protected", s.SecretsProtected)))
	fmt.Fprintln(&b, rule)
	b.WriteByte('\n')

	// Clean-scan shortcut: no body, no action block.
	clean := s.SecretsLeaked == 0 && s.SecretsAtRisk == 0

	// 3. Delta / first-audit section.
	if r.Delta != nil {
		writeDelta(&b, c, r.Delta, r.AuditTime)
	} else {
		b.WriteString("📊 First audit — no previous snapshot to compare against.\n")
		b.WriteString("    " + c.dim("From now on shhh will track progress between audits.") + "\n")
	}
	b.WriteByte('\n')
	fmt.Fprintln(&b, rule)
	b.WriteByte('\n')

	if clean {
		fmt.Fprintf(&b, "  %s %s\n", c.green("✓"), "No secrets found. All projects clean.")
		fmt.Fprintln(&b, rule)
		_, err := io.WriteString(w, b.String())
		return err
	}

	// 4. Project blocks.
	printed := sortedPrintable(r.Projects)
	for _, p := range printed {
		writeProject(&b, c, p)
		b.WriteByte('\n')
	}

	// 5. Footer action block.
	fmt.Fprintln(&b, rule)
	if s.SecretsLeaked > 0 {
		fmt.Fprintf(&b, "  %s %s\n",
			c.red("✗"),
			c.red(fmt.Sprintf("%d secrets already leaked — rotation is not optional", s.SecretsLeaked)))
	}
	if s.SecretsAtRisk > 0 {
		// Collect unprotected projects that have at-risk findings so
		// we can decide between the "protect with:" copy (there's an
		// action to take) and the softer "in files" copy (everything
		// is already protected, the at-risk finding is in an
		// allowlisted location like a fixture file).
		var actionable []auditpkg.Project
		for _, p := range r.Projects {
			if p.Status != auditpkg.StatusUnprotected {
				continue
			}
			if len(p.AtRisk) == 0 {
				continue
			}
			actionable = append(actionable, p)
		}
		if len(actionable) > 0 {
			fmt.Fprintf(&b, "  %s %s\n",
				c.yellow("✗"),
				c.yellow(fmt.Sprintf("%d secrets at risk — protect with:", s.SecretsAtRisk)))
			b.WriteByte('\n')
			for _, p := range actionable {
				fmt.Fprintf(&b, "    cd %s && shhh install\n", p.DisplayPath)
			}
		} else {
			fmt.Fprintf(&b, "  %s %s\n",
				c.yellow("·"),
				c.dim(fmt.Sprintf("%d secrets at risk in files under already-protected projects (fixtures or legacy state)", s.SecretsAtRisk)))
		}
	}
	if s.SecretsLeaked > 0 {
		b.WriteByte('\n')
		fmt.Fprintf(&b, "  %s Rotation dashboards:\n", c.green("✓"))
		seen := map[string]bool{}
		for _, p := range r.Projects {
			for _, f := range p.Leaked {
				for prefix, entry := range rotationURLs {
					if strings.HasPrefix(strings.ToUpper(f.Label), prefix) {
						if seen[prefix] {
							break
						}
						seen[prefix] = true
						fmt.Fprintf(&b, "    %-8s %s\n", entry.display+":", entry.url)
						break
					}
				}
			}
		}
	}
	fmt.Fprintln(&b, rule)

	_, err := io.WriteString(w, b.String())
	return err
}

// humanizeAgent turns an agent slug like "claude-code" into a display
// name like "Claude Code".
func humanizeAgent(a string) string {
	if a == "" {
		return "Unknown"
	}
	parts := strings.Split(a, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

func writeDelta(b *strings.Builder, c colorizer, d *auditpkg.Delta, now time.Time) {
	date := d.Since.UTC().Format("2006-01-02")
	days := int(now.Sub(d.Since).Hours() / 24)
	ago := fmt.Sprintf("%d days ago", days)
	if days == 1 {
		ago = "1 day ago"
	}
	if days <= 0 {
		ago = "today"
	}
	fmt.Fprintf(b, "📊 Since last audit · %s · %s\n", date, c.dim(ago))
	fmt.Fprintf(b, "     🚨 Leaked     %s\n", formatDeltaLine(c, d.Leaked, "leaked"))
	fmt.Fprintf(b, "     ⚠️  At risk    %s\n", formatDeltaLine(c, d.AtRisk, "at-risk"))
	fmt.Fprintf(b, "     ✅ Protected  %s\n", formatDeltaLine(c, d.Protected, "protected"))
}

func formatDeltaLine(c colorizer, dc auditpkg.DeltaCount, counter string) string {
	arrow := "·"
	comment := "no change"
	if dc.Change < 0 {
		arrow = "▼"
		switch counter {
		case "leaked":
			comment = "rotated, good work"
		case "at-risk":
			comment = "newly protected"
		case "protected":
			comment = "protection lost"
		}
	} else if dc.Change > 0 {
		arrow = "▲"
		switch counter {
		case "leaked":
			comment = "new leaks"
		case "at-risk":
			comment = "newly at risk"
		case "protected":
			comment = "project shielded"
		}
	}
	mag := dc.Change
	if mag < 0 {
		mag = -mag
	}
	// Green for improvement (leaked/at-risk down, protected up).
	// Red for regression.
	improved := (dc.Change < 0 && (counter == "leaked" || counter == "at-risk")) ||
		(dc.Change > 0 && counter == "protected")
	regressed := (dc.Change > 0 && (counter == "leaked" || counter == "at-risk")) ||
		(dc.Change < 0 && counter == "protected")

	arrowColored := arrow
	if improved {
		arrowColored = c.green(arrow)
	} else if regressed {
		arrowColored = c.red(arrow)
	}

	return fmt.Sprintf("%d → %d   (%s %d %s)", dc.Before, dc.After, arrowColored, mag, comment)
}

// sortedPrintable returns projects that should be printed, sorted
// unprotected (most-leaked first) > protected > archived. Clean
// projects are dropped entirely.
func sortedPrintable(ps []auditpkg.Project) []auditpkg.Project {
	out := make([]auditpkg.Project, 0, len(ps))
	for _, p := range ps {
		if p.Status == auditpkg.StatusClean {
			continue
		}
		out = append(out, p)
	}
	sort.SliceStable(out, func(i, j int) bool {
		oi := statusOrder(out[i].Status)
		oj := statusOrder(out[j].Status)
		if oi != oj {
			return oi < oj
		}
		// Within unprotected, most-leaked first.
		if out[i].Status == auditpkg.StatusUnprotected {
			if len(out[i].Leaked) != len(out[j].Leaked) {
				return len(out[i].Leaked) > len(out[j].Leaked)
			}
		}
		return out[i].DisplayPath < out[j].DisplayPath
	})
	return out
}

func statusOrder(s auditpkg.Status) int {
	switch s {
	case auditpkg.StatusUnprotected:
		return 0
	case auditpkg.StatusProtected:
		return 1
	case auditpkg.StatusArchived:
		return 2
	default:
		return 3
	}
}

func writeProject(b *strings.Builder, c colorizer, p auditpkg.Project) {
	// Header with right-aligned status tag.
	tag := statusTag(p.Status)
	headerPath := "📁 " + c.bold(p.DisplayPath)
	// Pad based on raw visible length (we use DisplayPath, not the
	// colored form, to keep column alignment even with ANSI on).
	raw := "📁 " + p.DisplayPath
	pad := pathColumnWidth - len(raw)
	if pad < 1 {
		pad = 1
	}
	fmt.Fprintf(b, "%s%s%s\n", headerPath, strings.Repeat(" ", pad), colorizeTag(c, p.Status, tag))

	// Metadata line.
	meta := projectMetadata(p)
	if meta != "" {
		fmt.Fprintf(b, "   %s\n", c.dim(meta))
	}

	if len(p.Leaked) > 0 {
		fmt.Fprintf(b, "   %s %s\n", "🚨", c.red("Already leaked to Claude"))
		// Determine padding width for placeholder column.
		maxPlaceholder := 0
		for _, f := range p.Leaked {
			if len(f.Placeholder) > maxPlaceholder {
				maxPlaceholder = len(f.Placeholder)
			}
		}
		if maxPlaceholder < 32 {
			maxPlaceholder = 32
		}
		for _, f := range p.Leaked {
			sessions := len(f.SessionIDs)
			if sessions == 0 {
				sessions = f.Occurrences
			}
			when := f.FirstSeen.UTC().Format("2006-01-02")
			pad := maxPlaceholder - len(f.Placeholder)
			if pad < 1 {
				pad = 1
			}
			fmt.Fprintf(b, "      %s%s   %s\n",
				c.red(f.Placeholder),
				strings.Repeat(" ", pad),
				c.dim(fmt.Sprintf("%d sessions · since %s", sessions, when)))
		}
	}

	if len(p.AtRisk) > 0 {
		fmt.Fprintf(b, "   %s %s\n", "⚠️ ", c.yellow("Currently at risk"))
		for _, f := range p.AtRisk {
			fmt.Fprintf(b, "      %s\n", c.yellow(f.Placeholder))
			loc := ""
			if len(f.Locations) > 0 {
				loc = f.Locations[0]
			}
			if loc != "" {
				fmt.Fprintf(b, "      %s%s\n", strings.Repeat(" ", 36), c.dim(loc))
			}
		}
	}
}

func statusTag(s auditpkg.Status) string {
	switch s {
	case auditpkg.StatusProtected:
		return "[PROTECTED ✓]"
	case auditpkg.StatusArchived:
		return "[ARCHIVED]"
	case auditpkg.StatusUnprotected:
		return "[UNPROTECTED]"
	case auditpkg.StatusClean:
		return "[CLEAN]"
	}
	return "[" + strings.ToUpper(string(s)) + "]"
}

func colorizeTag(c colorizer, s auditpkg.Status, tag string) string {
	switch s {
	case auditpkg.StatusUnprotected:
		return c.red(tag)
	case auditpkg.StatusProtected:
		return c.green(tag)
	case auditpkg.StatusArchived:
		return c.dim(tag)
	}
	return tag
}

func projectMetadata(p auditpkg.Project) string {
	switch p.Status {
	case auditpkg.StatusArchived:
		return fmt.Sprintf("folder gone · %d sessions retained in Claude history", p.SessionsTotal)
	case auditpkg.StatusProtected:
		if p.ShhhInstalledAt != nil {
			return fmt.Sprintf("shhh installed %s · %d protected sessions since",
				p.ShhhInstalledAt.UTC().Format("2006-01-02"), p.SessionsTotal)
		}
		return fmt.Sprintf("shhh installed · %d protected sessions", p.SessionsTotal)
	default:
		if p.SessionsTotal == 0 {
			return ""
		}
		return fmt.Sprintf("%d sessions · first seen %s",
			p.SessionsTotal, p.FirstSeen.UTC().Format("2006-01-02"))
	}
}
