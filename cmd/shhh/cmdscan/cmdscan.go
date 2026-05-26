// Package cmdscan implements `shhh scan`.
package cmdscan

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Musubi42/shhh/internal/detector"
	"github.com/Musubi42/shhh/internal/scanner"
)

// Run is the entry point for `shhh scan`.
func Run(args []string) error {
	fs := flag.NewFlagSet("scan", flag.ContinueOnError)
	showDetails := fs.Bool("show-details", false, "show host/user details (avoid in screenshots)")
	format := fs.String("format", "text", "output format: text|json|md")
	if err := fs.Parse(reorderFlagsFirst(args)); err != nil {
		return err
	}

	root := "."
	if fs.NArg() > 0 {
		root = fs.Arg(0)
	}
	abs, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	// Honour SHHH_DETECTOR so `shhh scan` is the calibration tool
	// for the gitleaks migration (see docs/gitleaks-spike.md and
	// docs/engine-architecture.md). Accepts shhh-native or
	// gitleaks; unset/unknown falls back to shhh-native.
	det := detector.NewFromEnv()
	sc := scanner.New(det)
	results, err := sc.Scan(abs)
	if err != nil {
		return fmt.Errorf("scan: %w", err)
	}

	switch *format {
	case "text":
		return writeText(os.Stdout, abs, results, *showDetails)
	case "json":
		return writeJSON(os.Stdout, results, *showDetails)
	case "md":
		return writeMarkdown(os.Stdout, abs, results, *showDetails)
	default:
		return fmt.Errorf("unknown format %q", *format)
	}
}

func writeText(w io.Writer, root string, results []scanner.FileResult, showDetails bool) error {
	sort.Slice(results, func(i, j int) bool {
		return results[i].Path < results[j].Path
	})

	fmt.Fprintf(w, "\n🛡️  shhh scan — %s\n\n", root)
	total := 0
	for _, r := range results {
		rel, _ := filepath.Rel(root, r.Path)
		fmt.Fprintf(w, "🔴 %s — %d secret(s) detected\n", rel, len(r.Findings))
		for _, f := range r.Findings {
			fmt.Fprintf(w, "   %-20s  %s (%s)\n",
				truncate(varNameGuess(r.Path, f), 20),
				f.Description,
				displayHint(f, showDetails),
			)
			total++
		}
		fmt.Fprintln(w)
	}
	fmt.Fprintln(w, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	fmt.Fprintf(w, "  %d file(s) with secrets  ·  %d secret(s) found\n", len(results), total)
	fmt.Fprintln(w, "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━")
	if total > 0 {
		fmt.Fprintln(w)
		fmt.Fprintln(w, "Run `shhh init` to protect these secrets from AI agents.")
		if !showDetails {
			fmt.Fprintln(w, "Run with `--show-details` for usernames, hosts, and connection metadata")
			fmt.Fprintln(w, "(local inspection only — avoid in screenshots).")
		}
	} else {
		fmt.Fprintln(w, "\n✅ No secrets found.")
	}
	return nil
}

type jsonFinding struct {
	Rule        string `json:"rule"`
	Label       string `json:"label"`
	Description string `json:"description"`
	Value       string `json:"value,omitempty"`
}

type jsonFileResult struct {
	Path     string        `json:"path"`
	Findings []jsonFinding `json:"findings"`
}

func writeJSON(w io.Writer, results []scanner.FileResult, showDetails bool) error {
	out := make([]jsonFileResult, 0, len(results))
	for _, r := range results {
		fr := jsonFileResult{Path: r.Path}
		for _, f := range r.Findings {
			jf := jsonFinding{
				Rule:        f.Rule,
				Label:       f.Label,
				Description: f.Description,
			}
			if showDetails {
				jf.Value = f.Value
			}
			fr.Findings = append(fr.Findings, jf)
		}
		out = append(out, fr)
	}
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

func writeMarkdown(w io.Writer, root string, results []scanner.FileResult, showDetails bool) error {
	fmt.Fprintf(w, "# shhh scan — `%s`\n\n", root)
	if len(results) == 0 {
		fmt.Fprintln(w, "No secrets found.")
		return nil
	}
	for _, r := range results {
		rel, _ := filepath.Rel(root, r.Path)
		fmt.Fprintf(w, "## 🔴 `%s`\n\n", rel)
		fmt.Fprintln(w, "| Label | Description | Hint |")
		fmt.Fprintln(w, "|-------|-------------|------|")
		for _, f := range r.Findings {
			fmt.Fprintf(w, "| `%s` | %s | `%s` |\n", f.Label, f.Description, displayHint(f, showDetails))
		}
		fmt.Fprintln(w)
	}
	return nil
}

// displayHint renders a screenshot-safe value hint. When showDetails is true,
// structured fields (connection string host/user, first chars of a token) are
// revealed. Otherwise they are masked.
func displayHint(f detector.Finding, showDetails bool) string {
	if showDetails {
		return f.Value
	}
	// Mask: keep public prefix if any, then bullet points.
	if f.PublicPrefix != "" {
		return f.PublicPrefix + "•••"
	}
	// Connection strings and high-entropy blobs collapse to a generic hint.
	if strings.Contains(f.Value, "://") {
		return maskConnString(f.Value)
	}
	return "••• (len=" + fmt.Sprintf("%d", len(f.Value)) + ")"
}

func maskConnString(s string) string {
	idx := strings.Index(s, "://")
	if idx < 0 {
		return "•••"
	}
	return s[:idx+3] + "•••:•••@•••/•••"
}

// varNameGuess tries to extract the variable name on the line where the
// finding sits. This is a heuristic for .env-style files; unknown formats fall
// back to the rule name.
func varNameGuess(path string, f detector.Finding) string {
	// Cheapest signal: the caller has the rule name. We'll keep it simple here
	// and just use the label. A richer implementation would re-open the file
	// and find the surrounding KEY=value line; Phase 0 does not need that.
	_ = path
	return f.Label
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}

// reorderFlagsFirst lets the user pass flags after positional arguments, which
// the stdlib flag package otherwise refuses. It walks args once and splits
// them into flags (and their values) vs positionals, then concatenates.
func reorderFlagsFirst(args []string) []string {
	var flags, pos []string
	i := 0
	for i < len(args) {
		a := args[i]
		if strings.HasPrefix(a, "-") {
			flags = append(flags, a)
			// If the flag doesn't carry its value as --flag=value and the next
			// arg isn't another flag or positional, assume it's the value.
			if !strings.Contains(a, "=") && i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				// Only consume next arg as value if this flag needs one. For our
				// small set we hardcode known bool flags.
				if !isBoolFlag(a) {
					flags = append(flags, args[i+1])
					i += 2
					continue
				}
			}
			i++
			continue
		}
		pos = append(pos, a)
		i++
	}
	return append(flags, pos...)
}

func isBoolFlag(a string) bool {
	switch strings.TrimLeft(a, "-") {
	case "show-details", "rehydrate":
		return true
	}
	return false
}
