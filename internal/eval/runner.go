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

// Matches reports whether the actual result agrees with the task's design.
// A cell "matches" iff actual-pass aligns with expected-pass, or
// actual-fail aligns with expected-fail. Mismatches are either regressions
// (designed-pass → fail) or surprise-passes (designed-fail → pass); both
// deserve attention.
func (c Cell) Matches() bool {
	if c.Expected == ExpectedPass {
		return c.Result.Pass
	}
	return !c.Result.Pass
}

// IsRegression reports whether a mismatch is a regression (designed to
// pass, actually failed). Regressions fail CI; surprise-passes warn but do
// not fail.
func (c Cell) IsRegression() bool {
	return c.Expected == ExpectedPass && !c.Result.Pass
}

// IsSurprisePass reports whether a mismatch is an unexpected pass
// (designed to fail, actually passed). These are warnings that demand
// investigation but do not fail CI.
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
// Cell glyphs encode four outcomes:
//   ✅ designed to pass, passed (or designed to fail, failed) — matches design
//   ❌ designed to pass, failed — regression
//   ⚠️  designed to fail, passed — surprising, deserves investigation
//   —  mode not supported by this task
//
// The design-match reading is important: a task like t01-jwt-decode is
// *supposed* to fail in `redact` mode (that is the proof that pure
// redaction breaks JWT reasoning). Rendering that as a red ❌ would
// misrepresent the eval. Instead ✅ means "the design held in this cell."
func Matrix(cells []Cell, tasks []Task) string {
	modes := AllModes()

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
	b.WriteString("legend: ✅ design held    ❌ regression    ⚠️  surprise pass    — unsupported\n\n")

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
			c, has := index[cellKey{t.ID(), m}]
			switch {
			case !has:
				b.WriteString(fmt.Sprintf(" %-14s", "skipped"))
			case c.IsRegression():
				b.WriteString(fmt.Sprintf(" %-14s", "❌ regression"))
			case c.IsSurprisePass():
				b.WriteString(fmt.Sprintf(" %-14s", "⚠️  surprise"))
			default:
				// Matches design: either expected-pass+passed or
				// expected-fail+failed. Both are ✅ to the matrix.
				label := "✅ pass"
				if c.Expected == ExpectedFail {
					label = "✅ fail-ok"
				}
				b.WriteString(fmt.Sprintf(" %-14s", label))
			}
		}
		b.WriteString("\n")
	}

	// Detail section for mismatches (regressions and surprise passes).
	mismatches := false
	for _, c := range cells {
		if c.Matches() {
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
