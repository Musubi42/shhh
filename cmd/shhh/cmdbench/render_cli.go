package cmdbench

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/x/term"
)

// renderTerminal writes the compact bench summary to w. Matches
// the mockup designed during planning: targets header, per-engine
// table with timings, agreement summary, one-line recommendation,
// HTML report path is printed by Run after this returns.
func renderTerminal(w io.Writer, r *benchReport, useColor bool) {
	c := pickColors(useColor)

	// Header
	fmt.Fprintln(w)
	fmt.Fprintf(w, "%s🛡️  shhh bench — detection engine comparison%s\n", c.bold, c.reset)
	fmt.Fprintln(w)
	fmt.Fprintf(w, "  Targets: %s\n", strings.Join(tildeAll(r.Targets), " "))
	fmt.Fprintf(w, "           %d files scanned, %s\n", r.FilesScanned, humanBytes(r.BytesScanned))
	fmt.Fprintln(w)

	// Per-engine table
	fmt.Fprintf(w, "  %-12s %-10s %-10s %s\n", "Engine", "Findings", "Time", "Per-file")
	fmt.Fprintf(w, "  %s\n", strings.Repeat("─", 50))
	var baseline time.Duration
	for i, e := range r.Engines {
		if i == 0 {
			baseline = e.Duration
		}
		perFile := time.Duration(0)
		if e.FilesScanned > 0 {
			perFile = e.Duration / time.Duration(e.FilesScanned)
		}
		deltaStr := ""
		if i > 0 && baseline > 0 {
			pct := int(100 * (float64(e.Duration) - float64(baseline)) / float64(baseline))
			sign := "+"
			if pct < 0 {
				sign = ""
			}
			deltaStr = fmt.Sprintf("  (%s%d%%)", sign, pct)
		}
		fmt.Fprintf(w, "  %-12s %-10d %-10s %s%s\n",
			e.Engine, len(e.Findings), shortDur(e.Duration), shortDur(perFile), deltaStr)
	}
	fmt.Fprintln(w)

	// Agreement block (shhh-native vs gitleaks if both present)
	native := r.findEngine(engineNative)
	gl := r.findEngine(engineGitleaks)
	if native != nil && gl != nil {
		shared, onlyNative, onlyGL := agreement(native, gl)
		fmt.Fprintf(w, "  Agreement: %d shared · %s%d only-shhh-native%s · %s%d only-gitleaks%s\n",
			totalCount(shared),
			c.yellow, totalCount(onlyNative), c.reset,
			c.cyan, totalCount(onlyGL), c.reset,
		)
		fmt.Fprintln(w)

		if len(onlyNative) > 0 {
			fmt.Fprintf(w, "  %sOnly-shhh-native:%s %s\n", c.yellow, c.reset, formatLabelList(onlyNative))
		}
		if len(onlyGL) > 0 {
			fmt.Fprintf(w, "  %sOnly-gitleaks:%s    %s\n", c.cyan, c.reset, formatLabelList(onlyGL))
		}
		if len(onlyNative) > 0 || len(onlyGL) > 0 {
			fmt.Fprintln(w)
		}

		// One-line recommendation. When the synthetic `union`
		// engine ran (bench appends it whenever ≥2 real engines are
		// selected), use its actual finding count so the footer
		// number matches the table. Otherwise fall back to a label-
		// based estimate (50+9 style) for runs where union is absent.
		unionCount := 0
		if u := r.findEngine(engineUnion); u != nil {
			unionCount = len(u.Findings)
		}
		switch {
		case len(onlyNative) > 0 && len(onlyGL) == 0:
			fmt.Fprintf(w, "  → Best coverage: %sshhh-native%s (%d findings; gitleaks misses %d labels)\n",
				c.bold, c.reset, len(native.Findings), totalCount(onlyNative))
		case len(onlyNative) == 0 && len(onlyGL) > 0:
			fmt.Fprintf(w, "  → Best coverage: %sgitleaks%s (%d findings; shhh-native misses %d labels)\n",
				c.bold, c.reset, len(gl.Findings), totalCount(onlyGL))
		case len(onlyNative) > 0 && len(onlyGL) > 0:
			if unionCount > 0 {
				fmt.Fprintf(w, "  → Best coverage: %sboth engines%s (union = %d findings)\n",
					c.bold, c.reset, unionCount)
			} else {
				fmt.Fprintf(w, "  → Best coverage: %sboth engines%s (estimate = %d findings)\n",
					c.bold, c.reset, len(native.Findings)+totalCount(onlyGL))
			}
		default:
			fmt.Fprintf(w, "  → Both engines fully agree (%d findings)\n",
				len(native.Findings))
		}
	}
}

// formatLabelList renders `[STRIPE_LIVE_KEY×3, GITHUB_PAT×1]`-style
// inline summaries, sorted by count descending then label.
func formatLabelList(lcs []labelCount) string {
	sorted := append([]labelCount(nil), lcs...)
	sort.Slice(sorted, func(i, j int) bool {
		if sorted[i].Count != sorted[j].Count {
			return sorted[i].Count > sorted[j].Count
		}
		return sorted[i].Label < sorted[j].Label
	})
	parts := make([]string, 0, len(sorted))
	for _, lc := range sorted {
		parts = append(parts, fmt.Sprintf("%s×%d", lc.Label, lc.Count))
	}
	return strings.Join(parts, ", ")
}

// tildeAll abbreviates a slice of paths.
func tildeAll(paths []string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = tildePath(p)
	}
	return out
}

func tildePath(p string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return p
	}
	if p == home {
		return "~"
	}
	if strings.HasPrefix(p, home+"/") {
		return "~" + p[len(home):]
	}
	return p
}

func shortDur(d time.Duration) string {
	switch {
	case d < time.Microsecond:
		return "< 1µs"
	case d < time.Millisecond:
		return fmt.Sprintf("%dµs", d.Microseconds())
	case d < time.Second:
		return fmt.Sprintf("%dms", d.Milliseconds())
	default:
		return fmt.Sprintf("%.2fs", d.Seconds())
	}
}

func humanBytes(n int64) string {
	switch {
	case n < 1024:
		return fmt.Sprintf("%d B", n)
	case n < 1024*1024:
		return fmt.Sprintf("%.1f KB", float64(n)/1024)
	case n < 1024*1024*1024:
		return fmt.Sprintf("%.1f MB", float64(n)/(1024*1024))
	default:
		return fmt.Sprintf("%.2f GB", float64(n)/(1024*1024*1024))
	}
}

// ---- ANSI colors ----------------------------------------------------------

type ansi struct {
	bold, yellow, cyan, dim, reset string
}

func pickColors(enabled bool) ansi {
	if !enabled {
		return ansi{}
	}
	return ansi{
		bold:   "\x1b[1m",
		yellow: "\x1b[33m",
		cyan:   "\x1b[36m",
		dim:    "\x1b[2m",
		reset:  "\x1b[0m",
	}
}

func isTTY(f *os.File) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return term.IsTerminal(f.Fd())
}
