package cmdbench

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// benchReportJSON is the on-disk + on-wire shape of one bench
// run. Both `data.json` (in the bench output dir) and the HTML
// viewer consume this — the HTML fetches it and renders
// client-side, so this is the single source of truth.
//
// Field tags follow lowerCamelCase JS convention so the
// browser-side code reads naturally; the Go side is unaffected.
type benchReportJSON struct {
	Generated        string             `json:"generated"`        // RFC 3339 UTC
	Targets          []string           `json:"targets"`          // absolute target paths the user passed
	ScanRoot         string             `json:"scanRoot"`         // longest common ancestor of every file with a finding, or ""
	ScanRootDisplay  string             `json:"scanRootDisplay"`  // tilde-abbreviated form of ScanRoot for the header
	FilesScanned    int                 `json:"filesScanned"`
	BytesScanned    int64               `json:"bytesScanned"`
	Engines         []engineJSON        `json:"engines"`
	EffectiveEngines []string           `json:"effectiveEngines"` // engines that drive per-finding attribution
	Agreement       *agreementJSON      `json:"agreement,omitempty"`
	Recommendation  string              `json:"recommendation"` // one-line `→ Best coverage: …` summary text
	Labels          []labelJSON         `json:"labels"`         // hierarchy: label → file → finding
}

type engineJSON struct {
	Name         string `json:"name"`
	Findings     int    `json:"findings"`
	DurationMS   int64  `json:"durationMs"`
	PerFileMS    int64  `json:"perFileMs"`
	FilesScanned int    `json:"filesScanned"`
	BytesScanned int64  `json:"bytesScanned"`
}

type agreementJSON struct {
	Shared            []labelCountJSON `json:"shared"`
	OnlyNative        []labelCountJSON `json:"onlyShhhNative"`
	OnlyGitleaks      []labelCountJSON `json:"onlyGitleaks"`
	SharedTotal       int              `json:"sharedTotal"`
	OnlyNativeTotal   int              `json:"onlyShhhNativeTotal"`
	OnlyGitleaksTotal int              `json:"onlyGitleaksTotal"`
}

type labelCountJSON struct {
	Label string `json:"label"`
	Count int    `json:"count"`
}

type labelJSON struct {
	Label   string     `json:"label"`
	Count   int        `json:"count"`
	Engines []string   `json:"engines"`
	Files   []fileJSON `json:"files"`
}

type fileJSON struct {
	DisplayPath string        `json:"displayPath"`
	AbsPath     string        `json:"absPath"`
	Count       int           `json:"count"`
	Engines     []string      `json:"engines"`
	Findings    []findingJSON `json:"findings"`
}

type findingJSON struct {
	Line        int      `json:"line"`
	Placeholder string   `json:"placeholder"`
	Snippet     string   `json:"snippet"`
	Engines     []string `json:"engines"`
}

// toJSON converts the in-memory report + computed groups into the
// serializable shape. ScanRoot is the longest common ancestor of
// every file that has a finding — empty when no such ancestor
// exists (multi-target scan across unrelated trees).
func toJSON(r *benchReport, groups []labelGroup, scanRoot string) *benchReportJSON {
	out := &benchReportJSON{
		Generated:        r.Generated.UTC().Format("2006-01-02T15:04:05Z"),
		Targets:          append([]string{}, r.Targets...),
		ScanRoot:         scanRoot,
		ScanRootDisplay:  tildePath(scanRoot),
		FilesScanned:     r.FilesScanned,
		BytesScanned:     r.BytesScanned,
		EffectiveEngines: effectiveEngines(r),
		Recommendation:   recommendationText(r),
	}
	for _, e := range r.Engines {
		perFile := int64(0)
		if e.FilesScanned > 0 {
			perFile = e.Duration.Milliseconds() / int64(e.FilesScanned)
		}
		out.Engines = append(out.Engines, engineJSON{
			Name:         e.Engine,
			Findings:     len(e.Findings),
			DurationMS:   e.Duration.Milliseconds(),
			PerFileMS:    perFile,
			FilesScanned: e.FilesScanned,
			BytesScanned: e.BytesScanned,
		})
	}
	if a := tmplAgreementVM(r); a != nil {
		out.Agreement = &agreementJSON{
			Shared:            asLabelCountJSON(a.Shared),
			OnlyNative:        asLabelCountJSON(a.OnlyNative),
			OnlyGitleaks:      asLabelCountJSON(a.OnlyGitleaks),
			SharedTotal:       a.SharedTotal,
			OnlyNativeTotal:   a.NativeTotal,
			OnlyGitleaksTotal: a.GitTotal,
		}
	}
	for _, g := range groups {
		lj := labelJSON{Label: g.Label, Count: g.Count, Engines: g.Engines}
		for _, f := range g.Files {
			fj := fileJSON{
				DisplayPath: f.DisplayPath,
				AbsPath:     f.File,
				Count:       f.Count,
				Engines:     f.Engines,
			}
			for _, it := range f.Items {
				fj.Findings = append(fj.Findings, findingJSON{
					Line:        it.Line,
					Placeholder: it.Placeholder,
					Snippet:     it.Snippet,
					Engines:     it.Engines,
				})
			}
			lj.Files = append(lj.Files, fj)
		}
		out.Labels = append(out.Labels, lj)
	}
	return out
}

// recommendationText mirrors the terminal renderer's one-liner so
// the JSON consumer (jq, tools) gets the same human verdict
// without re-implementing the logic.
func recommendationText(r *benchReport) string {
	a := tmplAgreementVM(r)
	if a == nil {
		return ""
	}
	native := r.findEngine(engineNative)
	gl := r.findEngine(engineGitleaks)
	union := r.findEngine(engineUnion)
	switch {
	case len(a.OnlyNative) > 0 && len(a.OnlyGitleaks) == 0 && native != nil:
		return fmt.Sprintf("shhh-native: best coverage (%d findings; gitleaks misses %d labels)", len(native.Findings), a.NativeTotal)
	case len(a.OnlyNative) == 0 && len(a.OnlyGitleaks) > 0 && gl != nil:
		return fmt.Sprintf("gitleaks: best coverage (%d findings; shhh-native misses %d labels)", len(gl.Findings), a.GitTotal)
	case len(a.OnlyNative) > 0 && len(a.OnlyGitleaks) > 0 && native != nil:
		if union != nil {
			return fmt.Sprintf("union = %d findings (no engine is a superset)", len(union.Findings))
		}
		return fmt.Sprintf("union ≈ %d findings (no engine is a superset)", len(native.Findings)+a.GitTotal)
	default:
		count := a.SharedTotal
		if native != nil {
			count = len(native.Findings)
		}
		return fmt.Sprintf("both engines agree (%d findings)", count)
	}
}

func asLabelCountJSON(in []labelCount) []labelCountJSON {
	out := make([]labelCountJSON, 0, len(in))
	for _, lc := range in {
		out = append(out, labelCountJSON{Label: lc.Label, Count: lc.Count})
	}
	return out
}

// writeJSON serializes the report to <outDir>/data.json. Indented
// for human readability — `jq` users open this file directly.
func writeJSON(outDir string, data *benchReportJSON) error {
	buf, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		return err
	}
	buf = append(buf, '\n')
	return atomicWrite(filepath.Join(outDir, "data.json"), buf)
}

// computeScanRoot returns the longest common directory ancestor
// for every file that produced a finding. Empty for single-file
// scans (their parent dir is uninteresting) and for multi-target
// scans where targets don't share a meaningful ancestor.
func computeScanRoot(details []findingDetail) string {
	if len(details) == 0 {
		return ""
	}
	paths := make([]string, 0, len(details))
	seen := map[string]bool{}
	for _, d := range details {
		if seen[d.File] {
			continue
		}
		seen[d.File] = true
		paths = append(paths, d.File)
	}
	root := commonDir(paths)
	// Guard: if every path lives directly under the same dir, the
	// computed root is that dir. Good. If it's just `/` or `.`,
	// commonDir already returns "" so DisplayPath falls back to
	// tilde-paths.
	_ = os.Stat // keep os import live in case we add existence checks later
	return root
}
