package cmdbench

import (
	"crypto/sha1"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/Musubi42/shhh/internal/detector"
)

// findingDetail is one row in the "Findings detail" section of the
// HTML report. It is the cross-engine merged shape: the same secret
// occurrence (overlapping offsets in the same file) appears ONCE
// here even if two engines flagged it, with Engines listing
// everyone who fired.
type findingDetail struct {
	File        string   // absolute path; the template tilde-abbreviates for display
	Line        int      // 1-indexed
	Label       string   // shhh placeholder label, picked from the best-labeled engine for the group
	Placeholder string   // `[LABEL:prefix…:hash]` — NEVER the raw match
	Snippet     string   // the source line with the match replaced inline by the placeholder; empty when the file can't be read. Other findings on the same line are also scrubbed to "[redacted]" so this column never leaks an adjacent raw secret.
	Engines     []string // sorted, deduped engine names that fired on this occurrence
}

// labelGroup is the outer drill-down section the template iterates
// over. Sorted by Count desc so the noisiest labels
// (HIGH_ENTROPY×449) surface first.
//
// Hierarchy: label → file → finding. The file level groups findings
// from the same source file so the path is shown ONCE and the user
// instantly sees `200 in go.sum, 5 in .env` — the diagnostic shape
// for false-positive triage.
type labelGroup struct {
	Label   string
	Count   int
	Engines []string    // union of engines firing anywhere in this label
	Files   []fileGroup // sorted by Count desc
}

// fileGroup nests inside a labelGroup. One per (label, file) pair.
type fileGroup struct {
	File        string          // absolute path
	DisplayPath string          // path relative to the report's ScanRoot, or tilde-abbreviated when no common root exists
	Count       int             // findings for this label in this file
	Engines     []string        // union of engines firing on findings in this file (within this label)
	Items       []findingDetail // sorted by Line ascending
}

// buildFindingDetails performs the cross-engine merge that the
// HTML drill-down needs. For each file, all findings from all
// engines are sorted by start offset and grouped by overlap; one
// findingDetail emerges per group with the union of contributing
// engines. Line numbers are resolved from cached file content.
//
// The merge tolerates the common case where two engines flag the
// same secret with slightly different boundaries (e.g. one
// includes the `KEY=` prefix, the other doesn't). A naive equality
// match would double-count those.
func buildFindingDetails(r *benchReport) []findingDetail {
	// 1) Gather (engine, finding) pairs by file. Only effective
	// engines contribute attribution. With the `both` mode gone
	// in Phase 1, every engine in the run is effective; the
	// suppression below remains a no-op until Phase 6 introduces
	// the `union` pseudo-engine.
	keep := map[string]bool{}
	for _, e := range effectiveEngines(r) {
		keep[e] = true
	}
	byFile := map[string][]anyTagged{}
	for _, er := range r.Engines {
		if !keep[er.Engine] {
			continue
		}
		for _, ref := range er.Findings {
			byFile[ref.File] = append(byFile[ref.File], anyTagged{er.Engine, ref})
		}
	}

	lc := newLineComputer()

	// Pre-compute every finding value per file, so the snippet
	// pass can scrub OTHER findings whose raw values happen to
	// sit before the current match on the same source line.
	// Required for files like go.sum where one line contains two
	// hashes — without scrubbing, finding #2's snippet prefix
	// would contain finding #1's raw value.
	valuesByFile := map[string][]string{}
	for file, items := range byFile {
		seen := map[string]bool{}
		for _, it := range items {
			v := it.ref.Finding.Value
			if v == "" || seen[v] {
				continue
			}
			seen[v] = true
			valuesByFile[file] = append(valuesByFile[file], v)
		}
	}

	var out []findingDetail

	for file, items := range byFile {
		// 2) Sort by Start offset, then End for stable ordering.
		sort.Slice(items, func(i, j int) bool {
			if items[i].ref.Finding.Start != items[j].ref.Finding.Start {
				return items[i].ref.Finding.Start < items[j].ref.Finding.Start
			}
			return items[i].ref.Finding.End < items[j].ref.Finding.End
		})

		// 3) Walk and group overlapping ranges. A new group starts
		//    when the current finding's Start is past the previous
		//    group's End — i.e. no overlap at all.
		var current []anyTagged
		var currentEnd int
		flush := func() {
			if len(current) == 0 {
				return
			}
			out = append(out, mergeGroup(current, file, lc, valuesByFile[file]))
			current = nil
		}
		for _, it := range items {
			if len(current) == 0 || it.ref.Finding.Start >= currentEnd {
				flush()
				current = []anyTagged{it}
				currentEnd = it.ref.Finding.End
				continue
			}
			current = append(current, it)
			if it.ref.Finding.End > currentEnd {
				currentEnd = it.ref.Finding.End
			}
		}
		flush()
	}

	// 4) Stable global order: by file, then line.
	sort.Slice(out, func(i, j int) bool {
		if out[i].File != out[j].File {
			return out[i].File < out[j].File
		}
		return out[i].Line < out[j].Line
	})
	return out
}

// mergeGroup turns a slice of overlapping (engine, finding) pairs
// from the same file into a single findingDetail. Label selection
// prefers shhh-native's (more specific labels), falling back to
// gitleaks'.
func mergeGroup(items []anyTagged, file string, lc *lineComputer, otherValues []string) findingDetail {
	pickedFinding := items[0].ref.Finding
	pickedEngine := items[0].engine

	engineSet := map[string]struct{}{}
	for _, it := range items {
		engineSet[it.engine] = struct{}{}
		if betterFinding(it.engine, it.ref.Finding, pickedEngine, pickedFinding) {
			pickedFinding = it.ref.Finding
			pickedEngine = it.engine
		}
	}
	engines := make([]string, 0, len(engineSet))
	for e := range engineSet {
		engines = append(engines, e)
	}
	sort.Slice(engines, func(i, j int) bool {
		return enginePriority(engines[i]) < enginePriority(engines[j])
	})

	placeholder := makePlaceholder(pickedFinding.Label, pickedFinding.Value)
	return findingDetail{
		File:        file,
		Line:        lc.lineAt(file, pickedFinding.Start),
		Label:       pickedFinding.Label,
		Placeholder: placeholder,
		Snippet:     lc.snippet(file, pickedFinding.Start, placeholder, otherValues),
		Engines:     engines,
	}
}

// anyTagged is the inner type alias used by mergeGroup. Local
// alias kept here so the merge stays self-contained and the slice
// element type doesn't leak across helpers.
type anyTagged = struct {
	engine string
	ref    findingRef
}

// betterFinding picks the most specific label between two
// candidates for the same overlapping group. shhh-native beats
// gitleaks for known prefix-rule labels because shhh-native's
// vocabulary is hand-curated.
func betterFinding(eA string, fA detector.Finding, eB string, fB detector.Finding) bool {
	if enginePriority(eA) < enginePriority(eB) {
		return true
	}
	if enginePriority(eA) > enginePriority(eB) {
		return false
	}
	// Same engine bucket: prefer the longer match (usually more
	// specific — includes vendor prefix).
	return (fA.End - fA.Start) > (fB.End - fB.Start)
}

// effectiveEngines returns the engines that contribute distinct
// attribution information for the drill-down. With the legacy
// `both` engine retired in Phase 1, every engine in the run is
// effective; this helper is preserved for the Phase 6 reshape
// (when a `union` pseudo-engine reintroduces a similar concept).
func effectiveEngines(r *benchReport) []string {
	out := make([]string, 0, len(r.Engines))
	for _, e := range r.Engines {
		out = append(out, e.Engine)
	}
	return out
}

// enginePriority is the tiebreaker for label selection AND the
// stable order of the engine pip list. Lower wins.
func enginePriority(e string) int {
	switch e {
	case engineNative:
		return 0
	case engineGitleaks:
		return 1
	}
	return 99
}

// makePlaceholder builds the shhh `[LABEL:prefix…:hash]` display
// string. The hash is the first 8 hex chars of SHA1(value) — same
// truncation the production hook uses for placeholder fingerprints.
//
// Raw value MUST NOT appear in the placeholder. Even though bench
// runs against fake-but-real-shaped test fixtures, the HTML report
// persists on disk and could be screenshotted.
func makePlaceholder(label, value string) string {
	sum := sha1.Sum([]byte(value))
	hash := fmt.Sprintf("%x", sum)[:8]
	prefix := derivePublicPrefix(value)
	if prefix == "" || prefix == value {
		return fmt.Sprintf("[%s:%s]", label, hash)
	}
	return fmt.Sprintf("[%s:%s…:%s]", label, prefix, hash)
}

// derivePublicPrefix returns the obvious public part of a value
// when a known vendor prefix is present, otherwise empty. Mirrors
// the helper in `internal/detector/gitleaks.go` — duplicated here
// to keep cmdbench's import surface small.
func derivePublicPrefix(value string) string {
	prefixes := []string{
		"sk_live_", "sk_test_", "sk-proj-", "sk-ant-",
		"ghp_", "gho_", "ghu_", "ghs_", "ghr_",
		"xoxb-", "xoxa-", "xoxp-", "xoxe.",
		"AKIA", "ASIA",
		"npm_",
		"AIza",
		"-----BEGIN ",
	}
	for _, p := range prefixes {
		if strings.HasPrefix(value, p) {
			return p
		}
	}
	return ""
}

// lineComputer maps (file, offset) → 1-indexed line number. File
// contents are read once per file and cached for the lifetime of
// the bench run. A nil cache entry means "read failed"; lineAt
// returns 0 in that case.
type lineComputer struct {
	cache map[string][]byte
}

func newLineComputer() *lineComputer {
	return &lineComputer{cache: map[string][]byte{}}
}

// snippet returns the source line containing offset, with the
// match at [start, end?) replaced inline by the placeholder. Only
// the line's PREFIX (start of line up to start) is included plus
// the placeholder — the suffix is dropped to avoid leaking
// downstream content on lines that contain multiple secrets.
//
// To keep prefix safe too, every entry in scrubValues is replaced
// with `[redacted]` before the placeholder is appended. This
// catches the go.sum case where two hashes share one line and the
// second finding's prefix would otherwise include the first one's
// raw value.
//
// Returns the bare placeholder if the file is unreadable or the
// offset is out of range — callers should still get something
// meaningful in the dashboard.
func (lc *lineComputer) snippet(file string, start int, placeholder string, scrubValues []string) string {
	content, seen := lc.cache[file]
	if !seen {
		buf, err := os.ReadFile(file)
		if err != nil {
			lc.cache[file] = nil
		} else {
			lc.cache[file] = buf
		}
		content = lc.cache[file]
	}
	if content == nil || start < 0 || start > len(content) {
		return placeholder
	}

	lineStart := 0
	for i := start - 1; i >= 0; i-- {
		if content[i] == '\n' {
			lineStart = i + 1
			break
		}
	}
	prefix := string(content[lineStart:start])
	prefix = strings.TrimLeft(prefix, " \t")

	// Scrub any other finding value that might appear in the
	// prefix. Skip very short values (<8 chars) to avoid noise
	// when a label or punctuation overlaps unrelated text.
	for _, v := range scrubValues {
		if len(v) < 8 {
			continue
		}
		prefix = strings.ReplaceAll(prefix, v, "[redacted]")
	}

	out := prefix + placeholder
	const maxLen = 120
	if len(out) > maxLen {
		// Truncate from the LEFT so the placeholder always
		// remains visible at the end — that's the column's
		// purpose.
		out = "…" + out[len(out)-maxLen+1:]
	}
	return out
}

func (lc *lineComputer) lineAt(file string, offset int) int {
	content, seen := lc.cache[file]
	if !seen {
		buf, err := os.ReadFile(file)
		if err != nil {
			lc.cache[file] = nil
			return 0
		}
		lc.cache[file] = buf
		content = buf
	}
	if content == nil {
		return 0
	}
	if offset < 0 || offset > len(content) {
		return 0
	}
	line := 1
	for i := 0; i < offset; i++ {
		if content[i] == '\n' {
			line++
		}
	}
	return line
}

// ---- label → file → finding grouping --------------------------------------

// buildLabelGroups groups a flat findingDetail slice into the
// label → file → finding hierarchy the dashboard renders. The
// scanRoot parameter is used to compute each fileGroup's
// DisplayPath (path relative to the common scan root); pass "" to
// fall back to tilde-paths.
func buildLabelGroups(details []findingDetail, scanRoot string) []labelGroup {
	type fileKey struct{ label, file string }

	byLabel := map[string]*labelGroup{}
	byFile := map[fileKey]*fileGroup{}
	labelEng := map[string]map[string]struct{}{}
	fileEng := map[fileKey]map[string]struct{}{}

	for _, d := range details {
		// Label bucket.
		lg, ok := byLabel[d.Label]
		if !ok {
			lg = &labelGroup{Label: d.Label}
			byLabel[d.Label] = lg
			labelEng[d.Label] = map[string]struct{}{}
		}
		lg.Count++
		for _, e := range d.Engines {
			labelEng[d.Label][e] = struct{}{}
		}

		// File bucket (scoped to this label).
		key := fileKey{label: d.Label, file: d.File}
		fg, ok := byFile[key]
		if !ok {
			fg = &fileGroup{
				File:        d.File,
				DisplayPath: displayPath(d.File, scanRoot),
			}
			byFile[key] = fg
			fileEng[key] = map[string]struct{}{}
		}
		fg.Items = append(fg.Items, d)
		fg.Count++
		for _, e := range d.Engines {
			fileEng[key][e] = struct{}{}
		}
	}

	// Finalize file groups: sort items by line, attach engine set,
	// attach to their parent label.
	for key, fg := range byFile {
		sort.Slice(fg.Items, func(i, j int) bool { return fg.Items[i].Line < fg.Items[j].Line })
		es := fileEng[key]
		eng := make([]string, 0, len(es))
		for e := range es {
			eng = append(eng, e)
		}
		sort.Slice(eng, func(i, j int) bool { return enginePriority(eng[i]) < enginePriority(eng[j]) })
		fg.Engines = eng
		byLabel[key.label].Files = append(byLabel[key.label].Files, *fg)
	}

	// Finalize label groups: sort their files by count desc,
	// attach engine set, emit in count-desc order.
	out := make([]labelGroup, 0, len(byLabel))
	for label, lg := range byLabel {
		sort.Slice(lg.Files, func(i, j int) bool {
			if lg.Files[i].Count != lg.Files[j].Count {
				return lg.Files[i].Count > lg.Files[j].Count
			}
			return lg.Files[i].DisplayPath < lg.Files[j].DisplayPath
		})
		es := labelEng[label]
		eng := make([]string, 0, len(es))
		for e := range es {
			eng = append(eng, e)
		}
		sort.Slice(eng, func(i, j int) bool { return enginePriority(eng[i]) < enginePriority(eng[j]) })
		lg.Engines = eng
		out = append(out, *lg)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].Label < out[j].Label
	})
	return out
}

// commonDir returns the longest directory that is an ancestor of
// every path in paths. Empty when paths is empty or when no
// non-root common ancestor exists. Used to compute the report's
// scanRoot so file groups can display short relative paths.
func commonDir(paths []string) string {
	if len(paths) == 0 {
		return ""
	}
	pref := filepathDir(paths[0])
	for _, p := range paths[1:] {
		for !hasDirPrefix(p, pref) {
			parent := filepathDir(pref)
			if parent == pref {
				return ""
			}
			pref = parent
		}
	}
	if pref == "/" || pref == "." || pref == "" {
		return ""
	}
	return pref
}

// hasDirPrefix reports whether path lives under prefix or equals
// it. Both must be absolute (or both relative); the helper just
// guards against the classic `/foo` matching `/foobar` trap.
func hasDirPrefix(path, prefix string) bool {
	if path == prefix {
		return true
	}
	return strings.HasPrefix(path, prefix+"/")
}

// filepathDir is a stable wrapper so we can swap out path
// semantics under test without import-cycle gymnastics. Defers
// to the stdlib for now.
func filepathDir(p string) string {
	if i := strings.LastIndex(p, "/"); i > 0 {
		return p[:i]
	}
	if strings.HasPrefix(p, "/") {
		return "/"
	}
	return "."
}

// displayPath returns the path the dashboard shows for a file
// group header. With a non-empty scanRoot, the root prefix is
// stripped; otherwise the path is tilde-abbreviated against
// $HOME.
func displayPath(file, scanRoot string) string {
	if scanRoot != "" && hasDirPrefix(file, scanRoot) {
		if file == scanRoot {
			return "."
		}
		return file[len(scanRoot)+1:]
	}
	return tildePath(file)
}
