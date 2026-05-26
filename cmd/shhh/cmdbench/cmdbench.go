// Package cmdbench implements `shhh bench` — a side-by-side
// comparison of the available detection engines on a corpus of
// real files. Born from the gitleaks Step 1 work: with two
// engines available (`shhh-native`, `gitleaks`), the honest way
// to pick a default is to run them on your actual content and
// look at the diff.
//
// Output: a compact terminal summary + an HTML report (path-served
// like `shhh audit`) with engine-vs-engine tables and per-label
// breakdowns. The terminal output is the "did the user need
// the HTML?" trigger — most of the time it suffices.
package cmdbench

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Musubi42/shhh/cmd/shhh/cmdaudit"
	"github.com/Musubi42/shhh/internal/detector"
	"github.com/Musubi42/shhh/internal/scanner"
)

const (
	engineNative   = "shhh-native"
	engineGitleaks = "gitleaks"
)

// Run is the entry point. Positional args are paths to scan;
// flags configure HTML output and engine selection.
func Run(args []string) error {
	fs := flag.NewFlagSet("bench", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	var (
		noServe    = fs.Bool("no-serve", false, "terminal output only; do not start the HTML viewer")
		noHTML     = fs.Bool("no-html", false, "skip HTML rendering entirely (implies --no-serve)")
		openFlag   = fs.Bool("open", false, "launch the default browser on the HTML viewer URL")
		enginesArg = fs.String("engines", "shhh-native,gitleaks", "comma-separated subset of engines to compare")
	)
	fs.Usage = func() {
		fmt.Fprintln(os.Stderr, `shhh bench — compare detection engines on real content

Usage:
  shhh bench <path> [<path>...]                    compare both engines
  shhh bench --engines=shhh-native,gitleaks <p>    explicit subset
  shhh bench --no-serve <path>                     terminal only, no HTML viewer
  shhh bench --no-html <path>                      terminal only, no HTML at all
  shhh bench --open <path>                         default + launch browser

Engines:
  shhh-native  shhh's first-party detector (env cross-ref, structural URL)
  gitleaks     gitleaks v8 library (~222 rules)`)
	}

	// Same flag-split pattern as cmdaudit/cmdinstall — flags after
	// positionals were silently swallowed by the stdlib parser
	// before we added the helper. See docs/testing-playbook.md.
	flagArgs, paths := splitFlagsAndPositionals(args)
	if err := fs.Parse(flagArgs); err != nil {
		return err
	}
	if len(paths) == 0 {
		fs.Usage()
		return fmt.Errorf("bench: at least one path is required")
	}

	if *noHTML {
		*noServe = true
	}
	if *noServe && *openFlag {
		return fmt.Errorf("--no-serve and --open are mutually exclusive")
	}

	engines, err := parseEngines(*enginesArg)
	if err != nil {
		return err
	}

	// Resolve targets to absolute paths so the report is reproducible.
	absTargets := make([]string, 0, len(paths))
	for _, p := range paths {
		abs, err := filepath.Abs(p)
		if err != nil {
			return fmt.Errorf("resolve %q: %w", p, err)
		}
		if _, statErr := os.Stat(abs); statErr != nil {
			return fmt.Errorf("target %q: %w", p, statErr)
		}
		absTargets = append(absTargets, abs)
	}

	report, err := runBench(absTargets, engines)
	if err != nil {
		return err
	}

	useColor := isTTY(os.Stdout)
	renderTerminal(os.Stdout, report, useColor)

	if *noHTML {
		return nil
	}
	htmlDir, err := prepareHTMLOutputDir(report)
	if err != nil {
		return fmt.Errorf("prepare html dir: %w", err)
	}
	if err := renderHTML(htmlDir, report); err != nil {
		return fmt.Errorf("render html: %w", err)
	}
	indexPath := filepath.Join(htmlDir, "index.html")
	if *noServe {
		fmt.Fprintln(os.Stdout)
		fmt.Fprintf(os.Stdout, "📄 HTML report written to: %s\n", indexPath)
		return nil
	}

	url, stop, err := cmdaudit.ServeReport(htmlDir)
	if err != nil {
		fmt.Fprintln(os.Stdout)
		fmt.Fprintf(os.Stdout, "📄 HTML report written to: %s\n", indexPath)
		fmt.Fprintf(os.Stdout, "(could not start local server: %v)\n", err)
		return nil
	}
	defer stop()
	fmt.Fprintln(os.Stdout)
	fmt.Fprintf(os.Stdout, "🌐 Bench report: %s\n", url)
	fmt.Fprintln(os.Stdout, "   Press Ctrl-C to stop the report server.")

	if *openFlag {
		_ = openBrowser(url)
	}

	// Block until interrupted — same idiom as cmdaudit.
	waitForInterrupt()
	return nil
}

func parseEngines(arg string) ([]string, error) {
	if arg == "" {
		return nil, fmt.Errorf("--engines cannot be empty")
	}
	parts := strings.Split(arg, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		switch p {
		case engineNative, engineGitleaks:
			out = append(out, p)
		default:
			return nil, fmt.Errorf("unknown engine %q (want: shhh-native, gitleaks)", p)
		}
	}
	return out, nil
}

// splitFlagsAndPositionals mirrors cmdaudit's helper — all bench
// flags are bool so the trivial split is correct.
func splitFlagsAndPositionals(args []string) (flags, positionals []string) {
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			// `--engines=foo` stays as one token (Go's flag.Parse
			// handles it). `--engines foo` is the value-taking
			// form bench doesn't accept (string flag with explicit
			// `=` only) — caller learns from the parse error.
			flags = append(flags, a)
		} else {
			positionals = append(positionals, a)
		}
	}
	return
}

// ---- timing + scanning ----------------------------------------------------

// engineResult holds the aggregated outcome of running one engine
// across all target paths.
type engineResult struct {
	Engine       string
	Findings     []findingRef            // every finding with its file path attached
	LabelCounts  map[string]int          // label → count, sorted lexicographically in the report
	PerFile      map[string][]labelCount // file → labels found there, for the HTML detail view
	Duration     time.Duration
	FilesScanned int
	BytesScanned int64
}

// findingRef is a Finding plus the file it came from. Bench needs
// the file context to render per-file diffs; the bare
// `detector.Finding` only carries start/end offsets relative to
// content, not file identity.
type findingRef struct {
	File    string
	Finding detector.Finding
}

type labelCount struct {
	Label string
	Count int
}

// benchReport is the rendering-ready shape consumed by both the
// terminal and HTML renderers.
type benchReport struct {
	Generated time.Time
	Targets   []string
	Engines   []*engineResult
	// Sum across all engines for the per-target stats header.
	FilesScanned int
	BytesScanned int64
}

func runBench(targets, engines []string) (*benchReport, error) {
	r := &benchReport{
		Generated: time.Now().UTC(),
		Targets:   targets,
	}

	for _, e := range engines {
		backend, err := buildBackend(e)
		if err != nil {
			return nil, err
		}
		er, scanErr := scanWith(backend, e, targets)
		if scanErr != nil {
			return nil, scanErr
		}
		r.Engines = append(r.Engines, er)
		if er.FilesScanned > r.FilesScanned {
			// Each engine scans the same files; pick the largest
			// FilesScanned as the corpus stat (defensive against
			// races in walkers, which shouldn't happen).
			r.FilesScanned = er.FilesScanned
			r.BytesScanned = er.BytesScanned
		}
	}
	return r, nil
}

func buildBackend(engine string) (detector.Backend, error) {
	switch engine {
	case engineNative:
		return detector.New(), nil
	case engineGitleaks:
		return detector.NewGitleaks()
	}
	return nil, fmt.Errorf("unknown engine %q", engine)
}

func scanWith(backend detector.Backend, engine string, targets []string) (*engineResult, error) {
	er := &engineResult{
		Engine:      engine,
		LabelCounts: map[string]int{},
		PerFile:     map[string][]labelCount{},
	}
	sc := scanner.New(backend)
	start := time.Now()
	for _, t := range targets {
		results, err := sc.Scan(t)
		if err != nil {
			return nil, fmt.Errorf("%s scan %s: %w", engine, t, err)
		}
		for _, fr := range results {
			er.FilesScanned++
			perFile := map[string]int{}
			for _, f := range fr.Findings {
				er.Findings = append(er.Findings, findingRef{File: fr.Path, Finding: f})
				er.LabelCounts[f.Label]++
				perFile[f.Label]++
			}
			if len(perFile) > 0 {
				lcs := make([]labelCount, 0, len(perFile))
				for l, c := range perFile {
					lcs = append(lcs, labelCount{Label: l, Count: c})
				}
				sort.Slice(lcs, func(i, j int) bool { return lcs[i].Label < lcs[j].Label })
				er.PerFile[fr.Path] = lcs
			}
		}
	}
	er.Duration = time.Since(start)
	// BytesScanned: best-effort, sum file sizes for paths we
	// actually found findings in. Approximate but close enough
	// for the header line.
	for path := range er.PerFile {
		if info, err := os.Stat(path); err == nil {
			er.BytesScanned += info.Size()
		}
	}
	return er, nil
}

// agreement computes the label-level shared / only-A / only-B
// counts between two engines. Used by both renderers to summarise
// the diff between shhh-native and gitleaks.
func agreement(a, b *engineResult) (shared, onlyA, onlyB []labelCount) {
	allLabels := map[string]struct{}{}
	for l := range a.LabelCounts {
		allLabels[l] = struct{}{}
	}
	for l := range b.LabelCounts {
		allLabels[l] = struct{}{}
	}
	keys := make([]string, 0, len(allLabels))
	for l := range allLabels {
		keys = append(keys, l)
	}
	sort.Strings(keys)
	for _, l := range keys {
		ca := a.LabelCounts[l]
		cb := b.LabelCounts[l]
		min := ca
		if cb < min {
			min = cb
		}
		if min > 0 {
			shared = append(shared, labelCount{Label: l, Count: min})
		}
		if ca > cb {
			onlyA = append(onlyA, labelCount{Label: l, Count: ca - cb})
		}
		if cb > ca {
			onlyB = append(onlyB, labelCount{Label: l, Count: cb - ca})
		}
	}
	return
}

// totalCount sums the Count field — sugar so the renderers don't
// inline the loop.
func totalCount(lcs []labelCount) int {
	t := 0
	for _, lc := range lcs {
		t += lc.Count
	}
	return t
}

// findEngine returns the result for engine name or nil if absent.
func (r *benchReport) findEngine(name string) *engineResult {
	for _, e := range r.Engines {
		if e.Engine == name {
			return e
		}
	}
	return nil
}
