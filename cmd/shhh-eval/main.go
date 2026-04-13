// Command shhh-eval runs the benchmark harness (PRD §10 Phase 0).
//
// Phase 0 organizational note: this binary ships from the shhh repository
// during private development. Before Phase 1 launch, it will move to a
// standalone shhh-eval repository alongside the product-agnostic Redactor
// interface.
package main

import (
	"fmt"
	"os"

	"github.com/musubi-sasu/shhh/internal/eval"
	"github.com/musubi-sasu/shhh/internal/eval/tasks"
)

func main() {
	// Phase 0: the CLI has only one action — run the full suite against the
	// shhh reference adapter and print the matrix. Subcommands (run-one,
	// diff-against-baseline, etc.) come later.
	if len(os.Args) > 1 && (os.Args[1] == "help" || os.Args[1] == "-h" || os.Args[1] == "--help") {
		fmt.Fprint(os.Stderr, `shhh-eval — the product-agnostic redaction benchmark (Phase 0)

Usage:
  shhh-eval              Run the full suite against the shhh reference adapter.
  shhh-eval help         Show this message.

Phase 0 ships task 7 (placeholder entropy) and task 8 (false-positive
calibration). The remaining 8 tasks require an agent runner and will be
added as the harness matures.
`)
		return
	}

	adapter := eval.NewShhhAdapter()

	suite := []eval.Task{
		tasks.NewJWTDecode(),
		tasks.NewPlaceholderEntropy(),
		tasks.NewPublicCorpus(),
	}

	cells := eval.Run(adapter, suite)
	fmt.Println(eval.Matrix(cells, suite))

	// Exit non-zero only on regressions (cells designed to pass that
	// failed). Surprise passes (cells designed to fail that passed) are
	// reported but do not fail the build — they are a warning that the
	// task may not be testing what we thought, and need investigation
	// rather than CI failure.
	if eval.HasRegressions(cells) {
		os.Exit(1)
	}
}
