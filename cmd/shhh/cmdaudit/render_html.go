// Package cmdaudit HTML report renderer.
//
// RenderHTML produces a self-contained HTML report tree from an
// audit.Result. The output is local-only, file:// viewable, and
// contains only placeholders — never raw secret values.
//
// Layout:
//
//	outDir/
//	  index.html                 -- the overview page
//	  projects/
//	    <slug>.html              -- one per project with findings
//
// Projects with StatusClean and no findings are counted in the overview
// summary but do NOT get a detail page.
//
// The CSS is inlined per template so each HTML file is self-contained
// and works under file:// with no assets beyond the (CDN) Google Fonts
// link. Offline font embedding is a v0.2.1 polish item.
package cmdaudit

import (
	"bytes"
	"crypto/sha1"
	"embed"
	"fmt"
	"html/template"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	auditpkg "github.com/musubi-sasu/shhh/internal/audit"
)

//go:embed templates/*.tmpl
var htmlTemplatesFS embed.FS

// ---- view models -----------------------------------------------------------

type overviewViewModel struct {
	AuditTimeDisplay    string
	AuditDateDisplay    string
	Agent               string
	TotalProjects       int
	SessionCount        int
	SpanDescription     string
	ScanDurationDisplay string

	Leaked        counterVM
	AtRisk        counterVM
	Protected     counterVM
	ProjectsTotal counterVM

	HasDelta bool
	Delta    deltaVM

	SectionTitle string

	VisibleProjects   []projectRowVM
	HasLeakedAnywhere bool

	Footer footerVM

	GeneratedBy string
}

type counterVM struct {
	Value      int
	DeltaText  string
	DeltaClass string
}

type deltaVM struct {
	TitleText string
	Lines     []deltaLineVM
}

type deltaLineVM struct {
	Emoji   string
	Label   string
	Before  int
	After   int
	Class   string
	Comment string
}

type projectRowVM struct {
	Slug         string
	BadgeClass   string
	BadgeLabel   string
	DisplayPath  string
	SummaryParts []string
	Secrets      []secretLineVM
}

type secretLineVM struct {
	SevClass    string
	Emoji       string
	Placeholder string
	Meta        string
}

type footerVM struct {
	LeakedCount         int
	AtRiskCount         int
	UnprotectedProjects []unprotectedVM
	RotationDashboards  []rotationLinkVM
}

type unprotectedVM struct {
	DisplayPath string
}

type rotationLinkVM struct {
	Vendor string
	URL    string
}

type projectViewModel struct {
	Agent          string
	ReportBackLink string

	DisplayPath string
	DirPart     string
	BasePart    string
	BadgeClass  string
	BadgeLabel  string

	SessionsTotal int
	LeakedCount   int
	AtRiskCount   int
	FirstSeenDate string

	HasLeaked     bool
	LeakedSecrets []leakedDetailVM

	HasAtRisk     bool
	AtRiskSecrets []atRiskDetailVM

	HasTimeline bool
	Timeline    []timelineItemVM

	Actions projectActionsVM
}

type leakedDetailVM struct {
	Placeholder    string
	SourcesLabel   string
	FirstSeenDate  string
	LastSeenDate   string
	RotationVendor string
	RotationURL    string
}

type atRiskDetailVM struct {
	Placeholder string
	SubLabel    string
	Location    string
	Rule        string
}

type timelineItemVM struct {
	Date         string
	EventText    template.HTML
	LeakPill     string
	SessionShort string
}

type projectActionsVM struct {
	Title    string
	Body     string
	Commands []template.HTML
}

// ---- public entrypoint -----------------------------------------------------

// RenderHTML writes the HTML report files for r into outDir. outDir
// must exist. The projects/ subdir is created on demand.
func RenderHTML(outDir string, r *auditpkg.Result) error {
	if r == nil {
		return fmt.Errorf("render html: nil Result")
	}
	if outDir == "" {
		return fmt.Errorf("render html: empty outDir")
	}
	if info, err := os.Stat(outDir); err != nil {
		return fmt.Errorf("render html: outDir: %w", err)
	} else if !info.IsDir() {
		return fmt.Errorf("render html: outDir %q is not a directory", outDir)
	}

	overviewTmpl, err := template.New("overview.html.tmpl").ParseFS(htmlTemplatesFS, "templates/overview.html.tmpl")
	if err != nil {
		return fmt.Errorf("render html: parse overview template: %w", err)
	}
	projectTmpl, err := template.New("project.html.tmpl").ParseFS(htmlTemplatesFS, "templates/project.html.tmpl")
	if err != nil {
		return fmt.Errorf("render html: parse project template: %w", err)
	}

	// Build slug map up front so overview links and project filenames agree.
	slugs := buildSlugs(r.Projects)

	overview := newOverviewViewModel(r, slugs)
	var buf bytes.Buffer
	if err := overviewTmpl.Execute(&buf, overview); err != nil {
		return fmt.Errorf("render html: execute overview: %w", err)
	}
	if err := writeAtomic(filepath.Join(outDir, "index.html"), buf.Bytes()); err != nil {
		return err
	}

	// Project detail pages.
	projectsDir := filepath.Join(outDir, "projects")
	var projectsDirEnsured bool
	for i := range r.Projects {
		p := &r.Projects[i]
		if len(p.Leaked) == 0 && len(p.AtRisk) == 0 {
			continue
		}
		if !projectsDirEnsured {
			if err := os.MkdirAll(projectsDir, 0o755); err != nil {
				return fmt.Errorf("render html: mkdir projects: %w", err)
			}
			projectsDirEnsured = true
		}
		vm := newProjectViewModel(r, p)
		var pbuf bytes.Buffer
		if err := projectTmpl.Execute(&pbuf, vm); err != nil {
			return fmt.Errorf("render html: execute project %q: %w", p.DisplayPath, err)
		}
		fname := slugs[p.AbsPath] + ".html"
		if err := writeAtomic(filepath.Join(projectsDir, fname), pbuf.Bytes()); err != nil {
			return err
		}
	}

	return nil
}

// ---- view-model builders ---------------------------------------------------

func newOverviewViewModel(r *auditpkg.Result, slugs map[string]string) overviewViewModel {
	vm := overviewViewModel{
		AuditTimeDisplay:    r.AuditTime.UTC().Format("2006-01-02 · 15:04:05 UTC"),
		AuditDateDisplay:    r.AuditTime.UTC().Format("2006-01-02"),
		Agent:               strings.ToUpper(strings.ReplaceAll(r.Agent, "-", " ")),
		TotalProjects:       r.Summary.ProjectsTotal,
		ScanDurationDisplay: formatScanDuration(r.ScanDuration),
		GeneratedBy:         "shhh v0.2.0-dev · forensic audit · local-only · no data leaves this machine",
	}
	if vm.Agent == "" {
		vm.Agent = "UNKNOWN"
	}

	// Aggregate session counts and time span.
	var earliest time.Time
	for _, p := range r.Projects {
		vm.SessionCount += p.SessionsTotal
		if !p.FirstSeen.IsZero() && (earliest.IsZero() || p.FirstSeen.Before(earliest)) {
			earliest = p.FirstSeen
		}
	}
	if !earliest.IsZero() && !r.AuditTime.IsZero() {
		vm.SpanDescription = describeSpan(r.AuditTime.Sub(earliest))
	}

	// Counter cards.
	vm.Leaked = counterVM{Value: r.Summary.SecretsLeaked}
	vm.AtRisk = counterVM{Value: r.Summary.SecretsAtRisk}
	vm.Protected = counterVM{Value: r.Summary.SecretsProtected}
	vm.ProjectsTotal = counterVM{Value: r.Summary.ProjectsTotal, DeltaText: "audited"}

	if r.Delta != nil {
		vm.HasDelta = true
		vm.Leaked.DeltaText, vm.Leaked.DeltaClass = formatCounterDelta(r.Delta.Leaked, true, r.Delta.Since)
		vm.AtRisk.DeltaText, vm.AtRisk.DeltaClass = formatCounterDelta(r.Delta.AtRisk, true, r.Delta.Since)
		vm.Protected.DeltaText, vm.Protected.DeltaClass = formatCounterDelta(r.Delta.Protected, false, r.Delta.Since)

		days := int(r.AuditTime.Sub(r.Delta.Since).Hours() / 24)
		ago := fmt.Sprintf("%d days ago", days)
		if days == 1 {
			ago = "1 day ago"
		}
		if days <= 0 {
			ago = "today"
		}
		vm.Delta.TitleText = fmt.Sprintf("Progress since last audit · %s · %s",
			r.Delta.Since.UTC().Format("2006-01-02"), ago)
		vm.Delta.Lines = []deltaLineVM{
			makeDeltaLine("🚨", "Leaked", r.Delta.Leaked, true),
			makeDeltaLine("⚠️", "At risk", r.Delta.AtRisk, true),
			makeDeltaLine("✅", "Protected", r.Delta.Protected, false),
		}
	} else {
		vm.Leaked.DeltaText = "first audit"
		vm.AtRisk.DeltaText = "first audit"
		vm.Protected.DeltaText = "first audit"
	}

	// Section title summary.
	needAttention := r.Summary.ProjectsUnprotected
	vm.SectionTitle = fmt.Sprintf("%d need attention · %d clean · %d archived",
		needAttention, r.Summary.ProjectsClean, r.Summary.ProjectsArchived)

	// Project rows: include any project that has findings, ordered
	// leaked-first then at-risk-only, then by path for stability.
	rows := make([]projectRowVM, 0, len(r.Projects))
	for i := range r.Projects {
		p := &r.Projects[i]
		if len(p.Leaked) == 0 && len(p.AtRisk) == 0 {
			continue
		}
		rows = append(rows, buildProjectRow(p, slugs[p.AbsPath]))
	}
	sort.SliceStable(rows, func(i, j int) bool {
		// Unprotected first, then others, then archived; alphabetic within.
		rank := func(b string) int {
			switch b {
			case "unprotected":
				return 0
			case "protected":
				return 1
			case "archived":
				return 2
			}
			return 3
		}
		ri, rj := rank(rows[i].BadgeClass), rank(rows[j].BadgeClass)
		if ri != rj {
			return ri < rj
		}
		return rows[i].DisplayPath < rows[j].DisplayPath
	})
	vm.VisibleProjects = rows

	// Footer action block.
	vm.HasLeakedAnywhere = r.Summary.SecretsLeaked > 0
	vm.Footer = buildFooter(r)

	return vm
}

func buildProjectRow(p *auditpkg.Project, slug string) projectRowVM {
	row := projectRowVM{
		Slug:        slug,
		DisplayPath: p.DisplayPath,
		BadgeClass:  string(p.Status),
		BadgeLabel:  badgeLabelFor(p.Status),
	}

	// Summary parts: leaked count, at-risk count, then session info.
	if n := len(p.Leaked); n > 0 {
		row.SummaryParts = append(row.SummaryParts, fmt.Sprintf("🚨 %d leaked", n))
	}
	if n := len(p.AtRisk); n > 0 {
		row.SummaryParts = append(row.SummaryParts, fmt.Sprintf("⚠️ %d at risk", n))
	}
	switch p.Status {
	case auditpkg.StatusArchived:
		row.SummaryParts = append(row.SummaryParts,
			fmt.Sprintf("folder removed · %d session%s retained in Claude history",
				p.SessionsTotal, plural(p.SessionsTotal)))
	case auditpkg.StatusProtected:
		installed := ""
		if p.ShhhInstalledAt != nil {
			installed = fmt.Sprintf("shhh installed %s · ", p.ShhhInstalledAt.UTC().Format("2006-01-02"))
		}
		row.SummaryParts = append(row.SummaryParts,
			fmt.Sprintf("%s%d session%s protected since", installed, p.SessionsTotal, plural(p.SessionsTotal)))
	default:
		first := ""
		if !p.FirstSeen.IsZero() {
			first = fmt.Sprintf(" · first seen %s", p.FirstSeen.UTC().Format("2006-01-02"))
		}
		row.SummaryParts = append(row.SummaryParts,
			fmt.Sprintf("%d session%s%s", p.SessionsTotal, plural(p.SessionsTotal), first))
	}

	// Secret lines: leaked first, then at-risk.
	for _, f := range p.Leaked {
		row.Secrets = append(row.Secrets, secretLineVM{
			SevClass:    "leaked",
			Emoji:       "🚨",
			Placeholder: f.Placeholder,
			Meta:        leakedMeta(f),
		})
	}
	for _, f := range p.AtRisk {
		row.Secrets = append(row.Secrets, secretLineVM{
			SevClass:    "at-risk",
			Emoji:       "⚠️",
			Placeholder: f.Placeholder,
			Meta:        atRiskMeta(f),
		})
	}
	return row
}

func leakedMeta(f auditpkg.Finding) string {
	n := len(f.SessionIDs)
	if n == 0 {
		n = f.Occurrences
	}
	since := ""
	if !f.FirstSeen.IsZero() {
		since = " · since " + f.FirstSeen.UTC().Format("2006-01-02")
	}
	return fmt.Sprintf("%d session%s%s", n, plural(n), since)
}

func atRiskMeta(f auditpkg.Finding) string {
	if len(f.Locations) > 0 {
		loc := f.Locations[0]
		// Trim to just the first file path without line numbers for
		// the compact overview row; the detail page shows full info.
		if i := strings.LastIndex(loc, ":"); i > 0 {
			loc = loc[:i]
		}
		return "in " + loc
	}
	return "in project files"
}

func buildFooter(r *auditpkg.Result) footerVM {
	f := footerVM{
		LeakedCount: r.Summary.SecretsLeaked,
		AtRiskCount: r.Summary.SecretsAtRisk,
	}
	seenRot := map[string]bool{}
	seenProj := map[string]bool{}
	for i := range r.Projects {
		p := &r.Projects[i]
		// Collect rotation dashboards from leaked findings.
		for _, fnd := range p.Leaked {
			for prefix, entry := range rotationURLs {
				if strings.HasPrefix(strings.ToUpper(fnd.Label), prefix) {
					if !seenRot[prefix] {
						seenRot[prefix] = true
						f.RotationDashboards = append(f.RotationDashboards,
							rotationLinkVM{Vendor: entry.display, URL: entry.url})
					}
					break
				}
			}
		}
		// Unprotected projects that still have at-risk secrets get an
		// install-here command line in the footer.
		if p.Status != auditpkg.StatusUnprotected {
			continue
		}
		if len(p.AtRisk) == 0 {
			continue
		}
		if seenProj[p.DisplayPath] {
			continue
		}
		seenProj[p.DisplayPath] = true
		f.UnprotectedProjects = append(f.UnprotectedProjects,
			unprotectedVM{DisplayPath: p.DisplayPath})
	}
	sort.Slice(f.RotationDashboards, func(i, j int) bool {
		return f.RotationDashboards[i].Vendor < f.RotationDashboards[j].Vendor
	})
	sort.Slice(f.UnprotectedProjects, func(i, j int) bool {
		return f.UnprotectedProjects[i].DisplayPath < f.UnprotectedProjects[j].DisplayPath
	})
	return f
}

func newProjectViewModel(r *auditpkg.Result, p *auditpkg.Project) projectViewModel {
	dir, base := splitDisplayPath(p.DisplayPath)
	vm := projectViewModel{
		Agent:          strings.ToUpper(strings.ReplaceAll(r.Agent, "-", " ")),
		ReportBackLink: "../index.html",
		DisplayPath:    p.DisplayPath,
		DirPart:        dir,
		BasePart:       base,
		BadgeClass:     string(p.Status),
		BadgeLabel:     badgeLabelFor(p.Status),
		SessionsTotal:  p.SessionsTotal,
		LeakedCount:    len(p.Leaked),
		AtRiskCount:    len(p.AtRisk),
		HasTimeline:    false, // v0.3 work
	}
	if vm.Agent == "" {
		vm.Agent = "UNKNOWN"
	}
	if !p.FirstSeen.IsZero() {
		vm.FirstSeenDate = p.FirstSeen.UTC().Format("2006-01-02")
	} else {
		vm.FirstSeenDate = "—"
	}

	if len(p.Leaked) > 0 {
		vm.HasLeaked = true
		for _, f := range p.Leaked {
			d := leakedDetailVM{
				Placeholder:  f.Placeholder,
				SourcesLabel: "source: " + formatSources(f.Sources),
			}
			if !f.FirstSeen.IsZero() {
				d.FirstSeenDate = f.FirstSeen.UTC().Format("2006-01-02")
			} else {
				d.FirstSeenDate = "—"
			}
			if !f.LastSeen.IsZero() {
				d.LastSeenDate = f.LastSeen.UTC().Format("2006-01-02")
			} else {
				d.LastSeenDate = "—"
			}
			if rv, rurl := lookupRotation(f.Label); rv != "" {
				d.RotationVendor = displayURL(rurl)
				d.RotationURL = rurl
			} else if f.RotationURL != "" {
				d.RotationVendor = displayURL(f.RotationURL)
				d.RotationURL = f.RotationURL
			}
			vm.LeakedSecrets = append(vm.LeakedSecrets, d)
		}
	}

	if len(p.AtRisk) > 0 {
		vm.HasAtRisk = true
		for _, f := range p.AtRisk {
			d := atRiskDetailVM{
				Placeholder: f.Placeholder,
				Rule:        strings.ToLower(f.Label),
			}
			if len(f.Locations) > 0 {
				d.Location = formatLocation(f.Locations[0])
			} else {
				d.Location = "—"
			}
			if strings.Contains(strings.ToUpper(f.Label), "POSTGRES") ||
				strings.Contains(strings.ToUpper(f.Label), "CONNSTRING") ||
				strings.Contains(strings.ToUpper(f.Label), "URL") {
				d.SubLabel = "structural redaction · host and database name preserved for agent reasoning"
			} else {
				d.SubLabel = "detected by pattern match"
			}
			vm.AtRiskSecrets = append(vm.AtRiskSecrets, d)
		}
	}

	vm.Actions = buildProjectActions(p)
	return vm
}

func buildProjectActions(p *auditpkg.Project) projectActionsVM {
	a := projectActionsVM{
		Title: "Rotate and protect",
		Body: "Rotate any leaked secrets using the dashboards listed above, " +
			"then install shhh in this project so the rotated values — and any " +
			"future ones — never reach Claude.",
	}

	// Dedup rotation dashboards for this project's leaked findings.
	seen := map[string]bool{}
	var lines []template.HTML
	hasRotation := false
	for _, f := range p.Leaked {
		if vend, rurl := lookupRotation(f.Label); vend != "" && !seen[vend] {
			seen[vend] = true
			if !hasRotation {
				lines = append(lines, template.HTML(`<span class="comment"># step 1 · rotate (non-negotiable)</span>`))
				hasRotation = true
			}
			safe := template.HTMLEscapeString(rurl)
			lines = append(lines, template.HTML(fmt.Sprintf(`<span class="cmd">open</span> <a href="%s">%s</a>`, safe, safe)))
		}
	}

	if p.Status == auditpkg.StatusUnprotected {
		if hasRotation {
			lines = append(lines, template.HTML(""))
		}
		lines = append(lines,
			template.HTML(`<span class="comment"># step 2 · protect this project</span>`))
		lines = append(lines,
			template.HTML(fmt.Sprintf(`<span class="cmd">cd</span> %s`, template.HTMLEscapeString(p.DisplayPath))))
		lines = append(lines,
			template.HTML(`<span class="cmd">shhh install</span>`))
		lines = append(lines, template.HTML(""))
		lines = append(lines,
			template.HTML(`<span class="comment"># step 3 · verify — next shhh audit should show this project as Protected</span>`))
		lines = append(lines,
			template.HTML(`<span class="cmd">shhh audit</span>`))
	}

	if len(lines) == 0 {
		lines = append(lines,
			template.HTML(`<span class="comment"># nothing to do — this project is clean</span>`))
	}
	a.Commands = lines
	return a
}

// ---- helpers ---------------------------------------------------------------

func badgeLabelFor(s auditpkg.Status) string {
	switch s {
	case auditpkg.StatusUnprotected:
		return "Unprotected"
	case auditpkg.StatusProtected:
		return "Protected ✓"
	case auditpkg.StatusArchived:
		return "Archived"
	case auditpkg.StatusClean:
		return "Clean"
	}
	return string(s)
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func formatScanDuration(d time.Duration) string {
	if d <= 0 {
		return "< 1ms"
	}
	if d < time.Second {
		return fmt.Sprintf("%dms", d.Milliseconds())
	}
	return fmt.Sprintf("%.1fs", d.Seconds())
}

func describeSpan(d time.Duration) string {
	days := int(d.Hours() / 24)
	switch {
	case days < 1:
		return ""
	case days < 30:
		if days == 1 {
			return "1 day"
		}
		return fmt.Sprintf("%d days", days)
	case days < 365:
		m := days / 30
		if m == 1 {
			return "1 month"
		}
		return fmt.Sprintf("%d months", m)
	default:
		y := days / 365
		if y == 1 {
			return "1 year"
		}
		return fmt.Sprintf("%d years", y)
	}
}

// formatCounterDelta returns the "▼ 3 · since YYYY-MM-DD" string and the
// CSS class ("sign-down", "sign-up", "sign-flat"). downIsGood=true means
// a decrease is coloured protected-green (correct for leaked/at-risk);
// downIsGood=false flips it (protected count going up is good).
func formatCounterDelta(dc auditpkg.DeltaCount, downIsGood bool, since time.Time) (string, string) {
	var arrow, class string
	switch {
	case dc.Change < 0:
		arrow = fmt.Sprintf("▼ %d", -dc.Change)
		if downIsGood {
			class = "sign-down"
		} else {
			class = "sign-up"
		}
	case dc.Change > 0:
		arrow = fmt.Sprintf("▲ %d", dc.Change)
		if downIsGood {
			class = "sign-up"
		} else {
			class = "sign-down"
		}
	default:
		arrow = "flat"
		class = "sign-flat"
	}
	sinceStr := ""
	if !since.IsZero() {
		sinceStr = " · since " + since.UTC().Format("2006-01-02")
	}
	return arrow + sinceStr, class
}

func makeDeltaLine(emoji, label string, dc auditpkg.DeltaCount, downIsGood bool) deltaLineVM {
	line := deltaLineVM{
		Emoji:  emoji,
		Label:  label,
		Before: dc.Before,
		After:  dc.After,
	}
	switch {
	case dc.Change < 0:
		if downIsGood {
			line.Class = "down"
			line.Comment = fmt.Sprintf("%d fewer", -dc.Change)
		} else {
			line.Class = "up"
			line.Comment = fmt.Sprintf("%d fewer", -dc.Change)
		}
	case dc.Change > 0:
		if downIsGood {
			line.Class = "up"
			line.Comment = fmt.Sprintf("+%d new", dc.Change)
		} else {
			line.Class = "down"
			line.Comment = fmt.Sprintf("+%d new", dc.Change)
		}
	default:
		line.Class = "flat"
		line.Comment = "no change"
	}
	return line
}

func splitDisplayPath(p string) (dir, base string) {
	if p == "" {
		return "", ""
	}
	// Find last slash; keep the trailing slash in dir for the styled rendering.
	i := strings.LastIndex(p, "/")
	if i < 0 {
		return "", p
	}
	return p[:i+1], p[i+1:]
}

func formatSources(sources []string) string {
	if len(sources) == 0 {
		return "session transcript"
	}
	// Replace dashed tokens with spaced ones and join with " · ".
	out := make([]string, 0, len(sources))
	for _, s := range sources {
		out = append(out, strings.ReplaceAll(s, "-", " "))
	}
	return strings.Join(out, " · ")
}

func formatLocation(loc string) string {
	// Turn "path/to/file:42" into "path/to/file · line 42".
	if i := strings.LastIndex(loc, ":"); i > 0 {
		rest := loc[i+1:]
		if isAllDigits(rest) {
			return loc[:i] + " · line " + rest
		}
	}
	return loc
}

func isAllDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return false
		}
	}
	return true
}

func lookupRotation(label string) (vendor, url string) {
	up := strings.ToUpper(label)
	for prefix, entry := range rotationURLs {
		if strings.HasPrefix(up, prefix) {
			return entry.display, entry.url
		}
	}
	return "", ""
}

func displayURL(raw string) string {
	if raw == "" {
		return ""
	}
	if u, err := url.Parse(raw); err == nil && u.Host != "" {
		host := strings.TrimPrefix(u.Host, "www.")
		if u.Path != "" && u.Path != "/" {
			return host + u.Path
		}
		return host
	}
	return raw
}

// writeAtomic writes data to path via a tmp file + rename so readers
// never see a partial file.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".shhh-html-*")
	if err != nil {
		return fmt.Errorf("render html: tmp: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("render html: write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("render html: close: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("render html: rename: %w", err)
	}
	return nil
}

// ---- slug ------------------------------------------------------------------

// buildSlugs assigns a deterministic, filename-safe slug to every
// project, resolving collisions with a short content hash suffix.
func buildSlugs(projects []auditpkg.Project) map[string]string {
	out := make(map[string]string, len(projects))
	used := make(map[string]string)
	for i := range projects {
		p := &projects[i]
		base := slugifyPath(p.DisplayPath)
		if base == "" {
			base = "project"
		}
		candidate := base
		if prior, exists := used[candidate]; exists && prior != p.AbsPath {
			// Collision: disambiguate with a short hash of the abspath.
			sum := sha1.Sum([]byte(p.AbsPath))
			candidate = fmt.Sprintf("%s-%x", base, sum[:3])
		}
		used[candidate] = p.AbsPath
		out[p.AbsPath] = candidate
	}
	return out
}

// slug is the exported helper form; returns a deterministic slug for a
// single project. For multi-project collision resolution, RenderHTML
// uses buildSlugs.
func slug(p *auditpkg.Project) string {
	base := slugifyPath(p.DisplayPath)
	if base == "" {
		base = "project"
	}
	return base
}

func slugifyPath(displayPath string) string {
	s := displayPath
	s = strings.TrimPrefix(s, "~/")
	s = strings.TrimPrefix(s, "/")
	var b strings.Builder
	b.Grow(len(s))
	lastDash := false
	for _, r := range s {
		switch {
		case (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9'):
			b.WriteRune(r)
			lastDash = false
		case r >= 'A' && r <= 'Z':
			b.WriteRune(r + ('a' - 'A'))
			lastDash = false
		default:
			if !lastDash {
				b.WriteByte('-')
				lastDash = true
			}
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 80 {
		out = out[:80]
	}
	return out
}
