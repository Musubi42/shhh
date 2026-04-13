package eval

import (
	"fmt"
	"strings"
	"time"
)

// Cell is one (task, mode) result.
type Cell struct {
	TaskID string
	Mode   Mode
	Result Result
	Took   time.Duration
}

// Run executes every supported mode of every task in tasks against the given
// redactor and returns the flat list of cells.
func Run(r Redactor, tasks []Task) []Cell {
	var out []Cell
	for _, t := range tasks {
		for _, m := range t.SupportedModes() {
			start := time.Now()
			res := t.Run(r, m)
			out = append(out, Cell{
				TaskID: t.ID(),
				Mode:   m,
				Result: res,
				Took:   time.Since(start),
			})
		}
	}
	return out
}

// Matrix renders cells as a task×mode grid for human reading.
func Matrix(cells []Cell, tasks []Task) string {
	// Determine the column headers (modes in canonical order).
	modes := AllModes()

	// Index cells by (task, mode).
	index := make(map[string]Result, len(cells))
	for _, c := range cells {
		index[c.TaskID+"|"+string(c.Mode)] = c.Result
	}

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("shhh-eval results\n")
	b.WriteString("=================\n\n")

	// Header row.
	b.WriteString(fmt.Sprintf("%-36s", "task"))
	for _, m := range modes {
		b.WriteString(fmt.Sprintf(" %-14s", shortMode(m)))
	}
	b.WriteString("\n")
	b.WriteString(strings.Repeat("-", 36+15*len(modes)))
	b.WriteString("\n")

	// Body rows.
	for _, t := range tasks {
		label := fmt.Sprintf("[T%d] %s", t.Tier(), t.ID())
		b.WriteString(fmt.Sprintf("%-36s", truncate(label, 36)))
		supported := modeSet(t.SupportedModes())
		for _, m := range modes {
			if _, ok := supported[m]; !ok {
				b.WriteString(fmt.Sprintf(" %-14s", "—"))
				continue
			}
			res, has := index[t.ID()+"|"+string(m)]
			switch {
			case !has:
				b.WriteString(fmt.Sprintf(" %-14s", "skipped"))
			case res.Pass:
				b.WriteString(fmt.Sprintf(" %-14s", "✅ pass"))
			default:
				b.WriteString(fmt.Sprintf(" %-14s", "❌ fail"))
			}
		}
		b.WriteString("\n")
	}

	// Detail section for failures.
	hasFailures := false
	for _, c := range cells {
		if !c.Result.Pass {
			if !hasFailures {
				b.WriteString("\nfailure details\n---------------\n")
				hasFailures = true
			}
			b.WriteString(fmt.Sprintf("  %s / %s: %s\n", c.TaskID, shortMode(c.Mode), c.Result.Reason))
		}
	}

	// Metrics section.
	hasMetrics := false
	for _, c := range cells {
		if len(c.Result.Metrics) > 0 {
			if !hasMetrics {
				b.WriteString("\nmetrics\n-------\n")
				hasMetrics = true
			}
			b.WriteString(fmt.Sprintf("  %s / %s:\n", c.TaskID, shortMode(c.Mode)))
			for k, v := range c.Result.Metrics {
				b.WriteString(fmt.Sprintf("    %s = %s\n", k, v))
			}
		}
	}

	return b.String()
}

func shortMode(m Mode) string {
	switch m {
	case ModeNoRedaction:
		return "baseline"
	case ModeRedact:
		return "redact"
	case ModeRedactRehydrate:
		return "redact+rehyd"
	case ModeRedactRehydrateCompen:
		return "+compen"
	}
	return string(m)
}

func modeSet(modes []Mode) map[Mode]struct{} {
	out := make(map[Mode]struct{}, len(modes))
	for _, m := range modes {
		out[m] = struct{}{}
	}
	return out
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
