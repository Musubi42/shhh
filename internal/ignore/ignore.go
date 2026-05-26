// Package ignore implements the layered ignore evaluator that
// drives shhh's `.shhhignore` semantics: gitleaks built-in
// allowlist (regex-based) → ~/.shhh/.shhhignore (gitignore syntax)
// → <project>/.shhhignore (gitignore syntax). Layers compose with
// last-decision-wins semantics, so a project-level `!go.sum`
// can re-include a path that gitleaks would otherwise allowlist.
//
// Out of scope here (feature B in docs/engine-architecture.md):
// the hook bypass file. This package only governs which paths
// the *detector* scans. The hook always redacts what it gets.
package ignore

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	gitignore "github.com/sabhiram/go-gitignore"
)

// Decision is the per-layer answer for a given path.
type Decision int

const (
	// Neutral means "this layer has no opinion".
	Neutral Decision = iota
	// Ignored means "skip this path during scan".
	Ignored
	// Included means "explicitly un-ignore this path (negation)".
	Included
)

// Matcher is the contract every layer implements. Source returns
// a short identifier the `shhh ignore` subcommand uses to
// attribute a final decision to its source layer.
type Matcher interface {
	Decision(path string) Decision
	Source() string
}

// LayeredMatcher composes N matchers into a single decision
// pipeline. Layers are evaluated in order, and the last
// non-Neutral decision wins — mirroring `.gitignore` cascade
// semantics across user-global and per-project files.
type LayeredMatcher struct {
	layers []Matcher
}

// NewLayered builds a LayeredMatcher from the given layers in
// priority order: earliest = lowest priority (gitleaks defaults),
// latest = highest priority (project file).
func NewLayered(layers ...Matcher) *LayeredMatcher {
	return &LayeredMatcher{layers: append([]Matcher(nil), layers...)}
}

// IsIgnored returns true iff the last non-Neutral layer says
// Ignored. All-Neutral or a final Included decision return false.
func (l *LayeredMatcher) IsIgnored(path string) bool {
	if l == nil {
		return false
	}
	d := Neutral
	for _, layer := range l.layers {
		if layer == nil {
			continue
		}
		if got := layer.Decision(path); got != Neutral {
			d = got
		}
	}
	return d == Ignored
}

// Explain returns the source label and Decision of the layer that
// produced the final answer for path. Useful for
// `shhh ignore check <path>`. Returns ("", Neutral) when no layer
// fired.
func (l *LayeredMatcher) Explain(path string) (string, Decision) {
	if l == nil {
		return "", Neutral
	}
	srcLabel := ""
	d := Neutral
	for _, layer := range l.layers {
		if layer == nil {
			continue
		}
		if got := layer.Decision(path); got != Neutral {
			srcLabel = layer.Source()
			d = got
		}
	}
	return srcLabel, d
}

// Layers exposes the underlying matchers in evaluation order.
// Used by `shhh ignore list` to render each layer's contribution
// without re-reading source files.
func (l *LayeredMatcher) Layers() []Matcher {
	if l == nil {
		return nil
	}
	return append([]Matcher(nil), l.layers...)
}

// ---- Gitleaks layer -------------------------------------------------------

// GitleaksLayer matches paths against gitleaks' built-in path
// allowlist regexes. The regex slices are owned by gitleaks; we
// snapshot them at construction so a later reload of gitleaks
// (rare in production) doesn't change our decisions mid-run.
type GitleaksLayer struct {
	patterns []*regexp.Regexp
}

// NewGitleaksLayer builds a layer from a slice of compiled path
// regexes — typically read from `(*detect.Detector).Config.Allowlists`.
// Passing nil produces an empty layer that always returns Neutral.
func NewGitleaksLayer(patterns []*regexp.Regexp) *GitleaksLayer {
	if len(patterns) == 0 {
		return &GitleaksLayer{}
	}
	out := make([]*regexp.Regexp, len(patterns))
	copy(out, patterns)
	return &GitleaksLayer{patterns: out}
}

func (g *GitleaksLayer) Decision(path string) Decision {
	if g == nil || len(g.patterns) == 0 {
		return Neutral
	}
	for _, re := range g.patterns {
		if re.MatchString(path) {
			return Ignored
		}
	}
	return Neutral
}

func (g *GitleaksLayer) Source() string {
	return "gitleaks (built-in allowlist)"
}

// ---- File layer (gitignore-syntax) ----------------------------------------

// fileRule is a single `.shhhignore` line plus its negation flag.
// We split on `!` ourselves so cross-layer negation works: a
// project-level `!path` returns Decision=Included, overriding an
// earlier Ignored from a lower-priority layer.
type fileRule struct {
	raw     string
	negate  bool
	matcher *gitignore.GitIgnore
}

func newFileRule(line string) *fileRule {
	negate := strings.HasPrefix(line, "!")
	pattern := line
	if negate {
		pattern = line[1:]
	}
	return &fileRule{
		raw:     line,
		negate:  negate,
		matcher: gitignore.CompileIgnoreLines(pattern),
	}
}

func (r *fileRule) matches(path string) bool {
	if r == nil || r.matcher == nil {
		return false
	}
	return r.matcher.MatchesPath(path)
}

// FileLayer evaluates the rules of one `.shhhignore` file in
// order. Within the file, last-rule-wins (standard .gitignore
// cascade). Across layers, the LayeredMatcher applies the same
// rule.
type FileLayer struct {
	source string // label for Explain (typically the file path)
	rules  []*fileRule
}

// NewFileLayer reads path and returns a FileLayer with its rules.
// A missing file returns an empty layer with no error — fresh
// installs don't ship a `.shhhignore` by default. Other read
// errors (permission, IO) are returned to the caller; the engine
// initialiser should log them and continue with an empty layer.
func NewFileLayer(path string) (*FileLayer, error) {
	label := path
	if label == "" {
		label = "<unnamed file>"
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &FileLayer{source: label}, nil
		}
		return nil, err
	}
	defer f.Close()

	var rules []*fileRule
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rules = append(rules, newFileRule(line))
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return &FileLayer{source: label, rules: rules}, nil
}

// NewInlineFileLayer builds a FileLayer from in-memory lines.
// Mirrors NewFileLayer's parsing for tests and for callers that
// have already loaded the bytes (e.g., a future embed).
func NewInlineFileLayer(source string, lines []string) *FileLayer {
	var rules []*fileRule
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		rules = append(rules, newFileRule(line))
	}
	return &FileLayer{source: source, rules: rules}
}

func (f *FileLayer) Decision(path string) Decision {
	if f == nil || len(f.rules) == 0 {
		return Neutral
	}
	d := Neutral
	for _, r := range f.rules {
		if r.matches(path) {
			if r.negate {
				d = Included
			} else {
				d = Ignored
			}
		}
	}
	return d
}

func (f *FileLayer) Source() string {
	if f == nil {
		return ""
	}
	return f.source
}

// Rules returns the raw rule lines in order. Used by
// `shhh ignore list` to display per-line attribution without
// re-reading the file.
func (f *FileLayer) Rules() []string {
	if f == nil {
		return nil
	}
	out := make([]string, 0, len(f.rules))
	for _, r := range f.rules {
		out = append(out, r.raw)
	}
	return out
}

// ---- Convenience constructor ---------------------------------------------

// DefaultPaths returns the canonical (global, project) `.shhhignore`
// file paths for the current user + working directory. Either may
// not exist; NewFileLayer handles that as "empty layer".
func DefaultPaths(projectRoot string) (globalPath, projectPath string, err error) {
	if dir := os.Getenv("SHHH_CONFIG_DIR"); dir != "" {
		globalPath = filepath.Join(dir, ".shhhignore")
	} else {
		home, herr := os.UserHomeDir()
		if herr != nil {
			return "", "", herr
		}
		globalPath = filepath.Join(home, ".shhh", ".shhhignore")
	}
	if projectRoot != "" {
		projectPath = filepath.Join(projectRoot, ".shhhignore")
	}
	return globalPath, projectPath, nil
}
