package tasks

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/musubi-sasu/shhh/internal/eval"
)

// GrepHardcoded is task 5: can the agent find *new* occurrences of a
// secret in files it has never read, given only the placeholder for
// that secret?
//
// Fixture is a temporary directory containing five files. Three of
// them hardcode the same Stripe-live key at different locations; two
// are decoys. The simulated agent is told "find every file that
// contains API_KEY" after observing the key's placeholder in a single
// entry-point file.
//
// Expected behavior per mode:
//
//   - no-redaction: the agent greps the raw value — trivially finds all
//     three hits. PASS.
//   - redact: the agent greps the placeholder string. The real
//     filesystem contains raw bytes, not placeholders, so the grep
//     returns zero hits. FAIL (by design).
//   - redact+rehydrate: this is the key case. The simulated proxy
//     rehydrates placeholder needles in tool_use args before the tool
//     executes. The grep sees the raw value, finds all three hits,
//     the result list comes back through the return-path redaction
//     (paths are not secrets, so nothing rewrites). PASS.
//   - redact+compensatory: the agent calls `grep_hardcoded(placeholder,
//     dir)`. The tool resolves the placeholder in-session and walks
//     the directory. Same three hits. PASS.
//
// Teaching value: this is the first task in the suite where
// `redact+rehydrate` actually passes without a compensatory tool.
// Compare to task 1 (JWT claims): there was nothing to rehydrate
// because the agent was reasoning about the value, not passing it to
// a tool. Task 5 flips that — the value *is* going to a tool (as the
// grep needle) so tool_use-arg rehydration is the right layer. The
// compensatory tool still passes, but as a UX refinement rather than
// a capability unlock. This result tells us PostToolUse redaction
// consistency alone is not a reason to ship `grep_hardcoded`; it is
// justified as a session-boundary guarantee (raw value never leaves
// the daemon via a shell command) rather than as a "must exist or
// task fails" primitive.
type GrepHardcoded struct{}

func NewGrepHardcoded() *GrepHardcoded { return &GrepHardcoded{} }

func (t *GrepHardcoded) ID() string    { return "t05-grep-hardcoded" }
func (t *GrepHardcoded) Title() string { return "Grep for hardcoded secret across files" }
func (t *GrepHardcoded) Tier() eval.Tier {
	return eval.Tier1
}

func (t *GrepHardcoded) SupportedModes() []eval.Mode {
	return []eval.Mode{
		eval.ModeNoRedaction,
		eval.ModeRedact,
		eval.ModeRedactRehydrate,
		eval.ModeRedactRehydrateCompen,
	}
}

// Expected reflects the task-5 design story: rehydration-in-tool-args
// *is* sufficient for this kind of task, so both rehydrate-only and
// compensatory modes pass. Pure redact is the single failing cell.
func (t *GrepHardcoded) Expected(mode eval.Mode) eval.Expected {
	switch mode {
	case eval.ModeRedact:
		return eval.ExpectedFail
	default:
		return eval.ExpectedPass
	}
}

// grepSecret is the fixture value. Hits the stripe-live-key pattern
// rule (24+ alphanumeric body, `sk_live_` prefix, word-boundary
// anchored on either side).
const grepSecret = "sk_live_3aB9Kq2dPr7nT8vXwY5zC6hL"

func (t *GrepHardcoded) Run(r eval.Redactor, mode eval.Mode) eval.Result {
	dir, err := buildGrepFixture()
	if err != nil {
		return eval.FailResult("cannot build fixture: "+err.Error(), nil)
	}
	defer os.RemoveAll(dir)

	// Expected hits: every file whose content contains the raw value.
	// Compute this from disk so the rubric stays in sync with the
	// fixture layout without hard-coded filenames.
	expected, err := filesContaining(dir, grepSecret)
	if err != nil {
		return eval.FailResult("cannot index fixture: "+err.Error(), nil)
	}
	if len(expected) == 0 {
		return eval.FailResult("fixture has no hits, task is vacuous", nil)
	}

	sess := r.NewSession()

	// Step 1: the agent "reads" an entry-point file that contains the
	// secret and observes whatever the redactor produces. In redact
	// modes this is a placeholder; in baseline it is the raw value.
	entryPoint := filepath.Join(dir, "entrypoint.env")
	entryContent, err := os.ReadFile(entryPoint)
	if err != nil {
		return eval.FailResult("cannot read entrypoint: "+err.Error(), nil)
	}
	var visibleContent []byte
	if mode == eval.ModeNoRedaction {
		visibleContent = entryContent
	} else {
		visibleContent, _ = r.Redact(sess, entryContent)
	}
	observed := extractEnvValue(string(visibleContent), "API_KEY")
	if observed == "" {
		return eval.FailResult("agent could not observe a value in entrypoint.env", nil)
	}

	// Step 2: the agent wants to find every file containing that
	// value. Its strategy depends on what it has available.
	var hits []string
	switch mode {
	case eval.ModeRedactRehydrateCompen:
		tools := eval.NewCompensatoryTools(r, sess)
		hits = tools.GrepHardcoded(observed, dir)
	case eval.ModeRedactRehydrate:
		// Simulate the proxy rehydrating the placeholder in the
		// tool_use args before the grep runs. After rehydration the
		// needle is the raw value and a plain grep finds real hits.
		needle := string(r.Rehydrate(sess, []byte(observed)))
		hits = grepDir(dir, needle)
	default:
		// baseline (raw value) and redact (placeholder) go through
		// the same grep codepath with different needles.
		hits = grepDir(dir, observed)
	}

	sort.Strings(hits)
	metrics := map[string]string{
		"observed":      observed,
		"expected_hits": fmt.Sprintf("%d", len(expected)),
		"actual_hits":   fmt.Sprintf("%d", len(hits)),
	}
	if !sameFileSet(hits, expected) {
		return eval.FailResult(
			fmt.Sprintf("hit set mismatch: got %v, want %v", relPaths(hits, dir), relPaths(expected, dir)),
			metrics,
		)
	}
	return eval.PassResult(metrics)
}

// buildGrepFixture lays out a temporary directory with five files:
// three hits and two decoys. Returns the directory path.
func buildGrepFixture() (string, error) {
	dir, err := os.MkdirTemp("", "shhh-eval-t05-")
	if err != nil {
		return "", err
	}
	write := func(rel, content string) error {
		full := filepath.Join(dir, rel)
		if mkErr := os.MkdirAll(filepath.Dir(full), 0o755); mkErr != nil {
			return mkErr
		}
		return os.WriteFile(full, []byte(content), 0o600)
	}
	files := map[string]string{
		"entrypoint.env":    "API_KEY=" + grepSecret + "\n",
		"src/app.go":        "package app\n\nconst apiKey = \"" + grepSecret + "\" // TODO: move to env\n",
		"src/legacy.py":     "import os\nAPI_KEY = \"" + grepSecret + "\"\n",
		"README.md":         "# Project\nUse API_KEY from .env. Never hardcode it.\n",
		"docs/overview.txt": "A short overview that mentions API keys conceptually but stores none.\n",
	}
	for rel, content := range files {
		if err := write(rel, content); err != nil {
			os.RemoveAll(dir)
			return "", err
		}
	}
	return dir, nil
}

// filesContaining returns the sorted set of files under dir whose
// content contains needle.
func filesContaining(dir, needle string) ([]string, error) {
	var hits []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}
		if strings.Contains(string(content), needle) {
			hits = append(hits, path)
		}
		return nil
	})
	sort.Strings(hits)
	return hits, err
}

// grepDir is a simulated `grep -r -l needle dir`: walks the tree,
// returns sorted paths whose content contains needle. Empty needle
// returns no hits (defensive — simulates a shell that would match
// every line with an empty pattern).
func grepDir(dir, needle string) []string {
	if needle == "" {
		return nil
	}
	hits, _ := filesContaining(dir, needle)
	return hits
}

// sameFileSet returns true iff a and b contain the same paths. Both
// must already be sorted.
func sameFileSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// relPaths strips the fixture root prefix from paths for compact
// error messages.
func relPaths(paths []string, root string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		if rel, err := filepath.Rel(root, p); err == nil {
			out[i] = rel
		} else {
			out[i] = p
		}
	}
	return out
}
