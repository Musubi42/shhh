package cmdbench

import (
	"bytes"
	"embed"
	"fmt"
	"html/template"
	"os"
	"path/filepath"

	auditpkg "github.com/Musubi42/shhh/internal/audit"
)

//go:embed bench.html.tmpl
var benchTemplateFS embed.FS

// prepareHTMLOutputDir creates ~/.shhh/bench/<timestamp>/ and
// returns its path. Mirrors the audit's pattern so a future user
// can `ls ~/.shhh/bench/` to find their old runs.
func prepareHTMLOutputDir(r *benchReport) (string, error) {
	root, err := benchRoot()
	if err != nil {
		return "", err
	}
	stamp := r.Generated.Format("2006-01-02T15-04-05Z")
	dir := filepath.Join(root, stamp)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	return dir, nil
}

// benchRoot returns the parent of all bench output dirs, honouring
// the same ~/.shhh layout as cmdaudit's audit reports.
func benchRoot() (string, error) {
	base, err := auditpkg.AuditDir()
	if err != nil {
		return "", err
	}
	// AuditDir returns ~/.shhh/audits; bench lives alongside.
	return filepath.Join(filepath.Dir(base), "bench"), nil
}

// renderHTML writes two files into outDir:
//
//   - data.json: full bench report in serializable form. Single
//     source of truth; consumable directly by `jq` and any
//     downstream tooling.
//   - index.html: a thin shell that fetches data.json on load and
//     renders summary, agreement, and the label→file→finding
//     drill-down client-side.
//
// Splitting data from view means: (a) one rendering pipeline, (b)
// no duplicated counts/labels between Go template + browser, (c)
// the JSON is directly inspectable without scraping HTML.
func renderHTML(outDir string, r *benchReport) error {
	details := buildFindingDetails(r)
	scanRoot := computeScanRoot(details)
	groups := buildLabelGroups(details, scanRoot)

	// Write data.json first so a template-render failure can't
	// leave a stale HTML pointing at a missing JSON file.
	jsonReport := toJSON(r, groups, scanRoot)
	if err := writeJSON(outDir, jsonReport); err != nil {
		return fmt.Errorf("write data.json: %w", err)
	}

	tmpl, err := template.New("bench.html.tmpl").Funcs(template.FuncMap{
		// The HTML shell is data-free; these helpers only render
		// the static page-load timestamp + footer.
		"shortDur":   shortDur,
		"humanBytes": humanBytes,
		"tildePath":  tildePath,
	}).ParseFS(benchTemplateFS, "bench.html.tmpl")
	if err != nil {
		return fmt.Errorf("parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, r); err != nil {
		return fmt.Errorf("execute template: %w", err)
	}
	return atomicWrite(filepath.Join(outDir, "index.html"), buf.Bytes())
}

func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".bench-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	return os.Rename(name, path)
}

// ---- template helpers -----------------------------------------------------

// agreementVM is the JS-renderable shape of the shhh-native vs
// gitleaks comparison. Built server-side and serialized into
// data.json via toJSON; the browser-side renderer consumes it
// directly.
type agreementVM struct {
	Shared        []labelCount
	OnlyNative    []labelCount
	OnlyGitleaks  []labelCount
	SharedTotal   int
	NativeTotal   int
	GitTotal      int
}

func tmplAgreementVM(r *benchReport) *agreementVM {
	native := r.findEngine(engineNative)
	gl := r.findEngine(engineGitleaks)
	if native == nil || gl == nil {
		return nil
	}
	shared, onlyN, onlyG := agreement(native, gl)
	return &agreementVM{
		Shared:       shared,
		OnlyNative:   onlyN,
		OnlyGitleaks: onlyG,
		SharedTotal:  totalCount(shared),
		NativeTotal:  totalCount(onlyN),
		GitTotal:     totalCount(onlyG),
	}
}

