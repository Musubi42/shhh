package eval

import (
	"fmt"
	"strings"
	"time"
)

// Cell is one (task, mode) result.
type Cell struct {
	TaskID   string
	Mode     Mode
	Result   Result
	Expected Expected
	Took     time.Duration
}

// A cell is one of three outcomes relative to its expected value:
//
//   - matches design: (expected=pass, actual=pass) or (expected=fail, actual=fail)
//   - regression:     (expected=pass, actual=fail) — fails CI
//   - surprise pass:  (expected=fail, actual=pass) — warns, does not fail CI
//
// Regressions and surprise passes are the only two ways a cell can
// disagree with its design, so "mismatch" is exactly their disjunction.

// IsRegression reports whether a cell is a regression (designed to pass,
// actually failed). Regressions fail CI.
func (c Cell) IsRegression() bool {
	return c.Expected == ExpectedPass && !c.Result.Pass
}

// IsSurprisePass reports whether a cell is an unexpected pass (designed
// to fail, actually passed). Warnings only; they do not fail CI.
func (c Cell) IsSurprisePass() bool {
	return c.Expected == ExpectedFail && c.Result.Pass
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
				TaskID:   t.ID(),
				Mode:     m,
				Result:   res,
				Expected: t.Expected(m),
				Took:     time.Since(start),
			})
		}
	}
	return out
}

// HasRegressions reports whether any cell in the run is a regression.
// Used by the CLI for exit-code decisions.
func HasRegressions(cells []Cell) bool {
	for _, c := range cells {
		if c.IsRegression() {
			return true
		}
	}
	return false
}

// Matrix renders cells as a task×mode grid for human reading.
//
// Cell labels encode four outcomes:
//   PASS      designed to pass, passed — matches design
//   FAIL-OK   designed to fail, failed — matches design (e.g. t01 in redact)
//   REGRESS   designed to pass, failed — regression, fails CI
//   SURPRISE  designed to fail, passed — warns, does not fail CI
//   —         mode not supported by this task
//
// The design-match reading is important: a task like t01-jwt-decode is
// *supposed* to fail in `redact` mode (that is the proof that pure
// redaction breaks JWT reasoning). Rendering that as a red REGRESS would
// misrepresent the eval. Labels are ASCII rather than emoji so %-N column
// widths align on byte count the same way they align in a terminal.
func Matrix(cells []Cell, tasks []Task) string {
	modes := AllModes()
	const modeColWidth = 12

	type cellKey struct {
		task string
		mode Mode
	}
	index := make(map[cellKey]Cell, len(cells))
	for _, c := range cells {
		index[cellKey{c.TaskID, c.Mode}] = c
	}

	var b strings.Builder
	b.WriteString("\n")
	b.WriteString("shhh-eval results\n")
	b.WriteString("=================\n")
	b.WriteString("legend: PASS design-held   FAIL-OK designed-fail   REGRESS regression   SURPRISE designed-fail-but-passed   - unsupported\n\n")

	// Header row.
	b.WriteString(fmt.Sprintf("%-36s", "task"))
	for _, m := range modes {
		b.WriteString(fmt.Sprintf(" %-*s", modeColWidth, shortMode(m)))
	}
	b.WriteString("\n")
	b.WriteString(strings.Repeat("-", 36+(modeColWidth+1)*len(modes)))
	b.WriteString("\n")

	// Body rows.
	for _, t := range tasks {
		label := fmt.Sprintf("[T%d] %s", t.Tier(), t.ID())
		b.WriteString(fmt.Sprintf("%-36s", truncate(label, 36)))
		supported := modeSet(t.SupportedModes())
		for _, m := range modes {
			if _, ok := supported[m]; !ok {
				b.WriteString(fmt.Sprintf(" %-*s", modeColWidth, "-"))
				continue
			}
			c, has := index[cellKey{t.ID(), m}]
			switch {
			case !has:
				b.WriteString(fmt.Sprintf(" %-*s", modeColWidth, "skipped"))
			case c.IsRegression():
				b.WriteString(fmt.Sprintf(" %-*s", modeColWidth, "REGRESS"))
			case c.IsSurprisePass():
				b.WriteString(fmt.Sprintf(" %-*s", modeColWidth, "SURPRISE"))
			default:
				cellLabel := "PASS"
				if c.Expected == ExpectedFail {
					cellLabel = "FAIL-OK"
				}
				b.WriteString(fmt.Sprintf(" %-*s", modeColWidth, cellLabel))
			}
		}
		b.WriteString("\n")
	}

	// Detail section for mismatches (regressions and surprise passes).
	mismatches := false
	for _, c := range cells {
		if !c.IsRegression() && !c.IsSurprisePass() {
			continue
		}
		if !mismatches {
			b.WriteString("\nmismatches\n----------\n")
			mismatches = true
		}
		kind := "regression"
		if c.IsSurprisePass() {
			kind = "surprise pass"
		}
		b.WriteString(fmt.Sprintf("  %s %s / %s (expected %s): %s\n",
			kind, c.TaskID, shortMode(c.Mode), c.Expected, c.Result.Reason))
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
