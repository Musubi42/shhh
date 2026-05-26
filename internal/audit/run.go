package audit

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Run orchestrates a complete audit for one agent and returns a
// Result. It is the top-level entry point cmdaudit calls.
//
// Run:
//   1. Enumerates Claude Code projects under <ClaudeRoot>/projects/
//   2. Applies the selectedProjects filter (if any)
//   3. Fans out the four AuditSources in parallel
//   4. Runs a current-disk scan on each project's files to find
//      at-risk secrets (distinct from the "leaked" findings from
//      past Claude state)
//   5. Joins findings back onto projects and decides Status
//   6. Computes the delta vs the last snapshot (if any)
//   7. Returns the Result — it is the caller's job to render and
//      persist
//
// Run is read-only up to the snapshot write (which is separate).
// It does not modify ~/.claude/ in any way.
func Run(ctx context.Context, cfg Config) (*Result, error) {
	start := time.Now()

	if cfg.Agent == "" {
		cfg.Agent = "claude-code"
	}
	if cfg.ClaudeRoot == "" {
		root, err := ClaudeRoot()
		if err != nil {
			return nil, err
		}
		cfg.ClaudeRoot = root
	}
	if cfg.AuditDir == "" {
		auditDir, err := AuditDir()
		if err != nil {
			return nil, err
		}
		cfg.AuditDir = auditDir
	}

	emit := func(e ProgressEvent) {
		if cfg.OnProgress != nil {
			cfg.OnProgress(e)
		}
	}

	// ----- Step 1: enumerate projects from Claude's own state -----
	// Pass cfg.ScopePaths into enumeration so the per-project
	// transcript read (`ResolveProjectPath` inside the loop) is
	// skipped for entries whose dash-name can't possibly map to a
	// path in scope. On a user with 50 projects and scope=1, this
	// avoids ~49 transcript reads + readdir calls — the dominant
	// cost of audit start-up. See F6 in dryrun-2026-05-26.md.
	projects, err := enumerateClaudeProjects(cfg.ClaudeRoot, cfg.ScopePaths)
	if err != nil {
		return nil, fmt.Errorf("enumerate claude projects: %w", err)
	}

	// ----- Step 2a: drop ignored projects (user opt-out via
	// ~/.shhh/config.json) BEFORE counting sessions or scanning. -----
	if len(cfg.IgnoredPaths) > 0 {
		ignore := make(map[string]bool, len(cfg.IgnoredPaths))
		for _, p := range cfg.IgnoredPaths {
			ignore[p] = true
		}
		kept := projects[:0]
		for _, p := range projects {
			if !ignore[p.AbsPath] {
				kept = append(kept, p)
			}
		}
		projects = kept
	}

	// ----- Step 2a': scope filter (CLI positional paths) -----
	// Only keep projects whose AbsPath equals or is under one of
	// cfg.ScopePaths. Empty list disables this filter.
	if len(cfg.ScopePaths) > 0 {
		kept := projects[:0]
		for _, p := range projects {
			if pathUnderAny(p.AbsPath, cfg.ScopePaths) {
				kept = append(kept, p)
			}
		}
		projects = kept
	}

	// ----- Step 2b: apply selectedProjects filter -----
	selected := cfg.SelectedProjects
	if len(selected) > 0 {
		filter := make(map[string]bool, len(selected))
		for _, s := range selected {
			filter[s] = true
		}
		kept := projects[:0]
		for _, p := range projects {
			if filter[p.DashName] {
				kept = append(kept, p)
			}
		}
		projects = kept
	}

	// ----- Step 2c: translate the surviving project set into a
	// dashName allow-list for the sources. Sources walk ~/.claude on
	// their own and don't know about ignored_paths — without this they
	// would keep reading the .jsonl of projects we already dropped,
	// blowing past sessionsTotal and surfacing ignored projects in the
	// live scroll log (real bug observed 2026-05-25).
	if selected == nil && (len(cfg.IgnoredPaths) > 0 || len(projects) > 0) {
		selected = make([]string, 0, len(projects))
		for _, p := range projects {
			selected = append(selected, p.DashName)
		}
	}

	// Announce the work the renderer is about to watch.
	totalSessions := 0
	for _, p := range projects {
		totalSessions += p.SessionsTotal
	}
	emit(ProgressEvent{
		Kind:          ProgressEnumerated,
		ProjectsTotal: len(projects),
		SessionsTotal: totalSessions,
	})

	// ----- Step 3: run audit sources in parallel -----
	transcriptSrc := NewTranscriptSource(cfg.ClaudeRoot)
	if cfg.OnProgress != nil {
		transcriptSrc.OnSessionDone = func() {
			emit(ProgressEvent{Kind: ProgressSessionFinished})
		}
		transcriptSrc.OnProjectDone = func(dashName, absPath string, sessionsScanned int) {
			emit(ProgressEvent{
				Kind:            ProgressProjectScanned,
				ProjectDashName: dashName,
				ProjectDisplay:  TildePath(absPath),
				Sessions:        sessionsScanned,
			})
		}
	}
	sources := []AuditSource{
		transcriptSrc,
		NewPromptHistorySource(cfg.ClaudeRoot),
		NewPasteCacheSource(cfg.ClaudeRoot),
		NewFileHistorySource(cfg.ClaudeRoot),
	}
	agg := newAggregator()
	if cfg.OnProgress != nil {
		agg.SetOnFinding(func(placeholder, projectDashName string) {
			emit(ProgressEvent{
				Kind:            ProgressFinding,
				Placeholder:     placeholder,
				ProjectDashName: projectDashName,
			})
		})
	}

	var sourceProgress func(string, int)
	if cfg.OnProgress != nil {
		sourceProgress = func(source string, count int) {
			emit(ProgressEvent{Kind: ProgressSourceCount, Source: source, Count: count})
		}
	}
	drainSources(ctx, sources, selected, agg, sourceProgress)
	leakedByProject := agg.Finalize()

	shhhInstalledPaths, shhhScope := loadShhhInstalledState(cfg.ShhhConfigPath)

	// ----- Steps 4 + 5 merged: per-project at-risk scan, leaked
	// findings join, status decision, and a progress emit so the
	// renderer can flow projects one-by-one into its scroll log.
	for i := range projects {
		if projects[i].OnDisk {
			projects[i].AtRisk = scanProjectAtRisk(ctx, projects[i].AbsPath, agg)
		}

		key := projects[i].DashName
		if pf, ok := leakedByProject[key]; ok {
			projects[i].Leaked = pf.Leaked
			for _, f := range pf.Leaked {
				if !f.LastSeen.IsZero() && f.LastSeen.After(projects[i].LastSessionAt) {
					projects[i].LastSessionAt = f.LastSeen
				}
				if !f.FirstSeen.IsZero() {
					if projects[i].FirstSeen.IsZero() || f.FirstSeen.Before(projects[i].FirstSeen) {
						projects[i].FirstSeen = f.FirstSeen
					}
				}
			}
		}
		ApplyStatus(&projects[i], shhhInstalledPaths, shhhScope)

		emit(ProgressEvent{
			Kind:           ProgressProjectFinished,
			ProjectIndex:   i,
			ProjectDisplay: projects[i].DisplayPath,
			Sessions:       projects[i].SessionsTotal,
			Leaked:         len(projects[i].Leaked),
			AtRisk:         len(projects[i].AtRisk),
			Status:         projects[i].Status,
		})
	}

	// Drop the unattributed orphan bucket from leakedByProject — in
	// v0.2 we don't render it. Log the count if any.
	if orphans := leakedByProject[unattributedKey]; len(orphans.Leaked) > 0 {
		// No-op: the aggregator has already dropped these from the
		// per-project map if cross-source attribution worked. Anything
		// left here is a legitimate orphan and is not reported in
		// v0.2. A future version could surface it as a "paste cache
		// with no project match" bucket.
		_ = orphans
	}

	// ----- Step 6: summary + delta -----
	result := &Result{
		SchemaVersion: CurrentSchemaVersion,
		Agent:         cfg.Agent,
		AuditTime:     start.UTC(),
		ScanDuration:  time.Since(start),
		Projects:      projects,
	}
	result.Summary = computeSummary(projects)

	delta, err := ComputeDelta(cfg.AuditDir, result)
	if err != nil {
		// Delta failure is non-fatal — log-equivalent (return the
		// result without delta, the caller shows "first audit" copy).
		// TODO: once the project has a logger, wire it here.
		_ = err
	}
	result.Delta = delta

	emit(ProgressEvent{Kind: ProgressDone})
	return result, nil
}

// EnumerateProjects is the exported wrapper around the internal
// enumeration used by Run. The pre-audit picker calls this to
// build the project list it offers the user. claudeRoot empty
// resolves to ~/.claude.
func EnumerateProjects(claudeRoot string) ([]Project, error) {
	if claudeRoot == "" {
		root, err := ClaudeRoot()
		if err != nil {
			return nil, err
		}
		claudeRoot = root
	}
	return enumerateClaudeProjects(claudeRoot, nil)
}

// enumerateClaudeProjects lists every subdirectory of
// <root>/projects/ and turns each into a Project with paths decoded
// and basic metadata populated. At this stage findings are still
// empty; Run joins them on a later pass.
func enumerateClaudeProjects(root string, scopeRoots []string) ([]Project, error) {
	projectsDir := ClaudeProjectsDir(root)
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	out := make([]Project, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		dash := e.Name()
		// Cheap scope pre-filter: skip the per-project IO
		// (transcript-read + jsonl-count) when the dash-name can't
		// possibly decode to a path in scope. False positives are
		// harmless — they're caught by the post-enumeration scope
		// filter in Run(); false negatives would be bugs and are
		// covered by TestDashNameCouldMatchScope.
		if !DashNameCouldMatchScope(dash, scopeRoots) {
			continue
		}
		projectDir := filepath.Join(projectsDir, dash)
		abs := ResolveProjectPath(dash, projectDir)
		p := Project{
			AbsPath:     abs,
			DisplayPath: TildePath(abs),
			DashName:    dash,
			OnDisk:      PathExists(abs),
		}
		// Count session files (*.jsonl) as a cheap sessions-total.
		p.SessionsTotal = countJSONLFiles(projectDir)
		out = append(out, p)
	}
	// Stable order for testability and consistent output.
	sort.Slice(out, func(i, j int) bool {
		return out[i].DashName < out[j].DashName
	})
	return out, nil
}

func countJSONLFiles(dir string) int {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".jsonl" {
			n++
		}
	}
	return n
}

// scanProjectAtRisk walks the on-disk project directory looking for
// secrets in the files an agent would realistically read. Scope is
// limited to files matching well-known "env-like" paths (.env*,
// .envrc) plus any file under a top-level config/ or secrets/ dir,
// because a full-recursive scan would (a) take forever on large repos
// and (b) double-count with the agent transcript scan.
//
// The goal: surface secrets the user has not yet leaked to Claude but
// WOULD leak on the next session. A full deep scan is the scope of
// the standalone `shhh scan` command; the audit stays focused.
func scanProjectAtRisk(ctx context.Context, projectAbs string, agg *aggregator) []Finding {
	if projectAbs == "" {
		return nil
	}
	var findings []Finding

	candidates, err := collectEnvLikeFiles(projectAbs, 64)
	if err != nil {
		return nil
	}
	for _, path := range candidates {
		select {
		case <-ctx.Done():
			return findings
		default:
		}
		content, err := readSmallFile(path, 2<<20) // 2 MB cap
		if err != nil {
			continue
		}
		rel, err := filepath.Rel(projectAbs, path)
		if err != nil {
			rel = path
		}
		fs := agg.ScanAtRiskFile(rel, content)
		findings = append(findings, fs...)
	}
	return findings
}

// collectEnvLikeFiles finds the at-most N env-like paths under
// projectRoot. It does NOT recurse beyond a few well-known dirs.
func collectEnvLikeFiles(projectRoot string, cap int) ([]string, error) {
	var out []string

	// Top-level dotenv-family files.
	top, err := os.ReadDir(projectRoot)
	if err != nil {
		return nil, err
	}
	for _, e := range top {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if isEnvLikeFilename(name) {
			out = append(out, filepath.Join(projectRoot, name))
			if len(out) >= cap {
				return out, nil
			}
		}
	}

	// Well-known config dirs (one level deep only).
	for _, sub := range []string{"config", "secrets", ".secrets", "env"} {
		subdir := filepath.Join(projectRoot, sub)
		entries, err := os.ReadDir(subdir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			out = append(out, filepath.Join(subdir, e.Name()))
			if len(out) >= cap {
				return out, nil
			}
		}
	}
	return out, nil
}

func isEnvLikeFilename(name string) bool {
	if name == ".env" || name == ".envrc" {
		return true
	}
	if len(name) >= 5 && name[:5] == ".env." {
		return true
	}
	return false
}

func readSmallFile(path string, cap int64) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", err
	}
	if !st.Mode().IsRegular() || st.Size() > cap {
		return "", os.ErrNotExist
	}
	buf, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}
	// Reject binaries.
	for i, b := range buf {
		if i >= 512 {
			break
		}
		if b == 0 {
			return "", os.ErrNotExist
		}
	}
	return string(buf), nil
}

func computeSummary(projects []Project) Summary {
	s := Summary{ProjectsTotal: len(projects)}
	for _, p := range projects {
		switch p.Status {
		case StatusUnprotected:
			s.ProjectsUnprotected++
		case StatusProtected:
			s.ProjectsProtected++
		case StatusArchived:
			s.ProjectsArchived++
		case StatusClean:
			s.ProjectsClean++
		}
		for _, f := range p.Leaked {
			_ = f
			s.SecretsLeaked++
		}
		for _, f := range p.AtRisk {
			_ = f
			s.SecretsAtRisk++
		}
		if p.Status == StatusProtected && (len(p.Leaked) > 0 || len(p.AtRisk) > 0) {
			s.SecretsProtected += len(p.Leaked) + len(p.AtRisk)
		}
	}
	return s
}

// loadShhhInstalledState reads the shhh user config (~/.shhh/config.json)
// to determine which paths shhh is protecting, if any. A missing
// config is not an error: it means shhh is not installed yet, and
// every project is Unprotected by default.
//
// This function is isolated here (rather than calling cmdinstall
// directly) to avoid an import cycle: cmdinstall imports cmdhook
// which imports cmdredact which imports this package eventually.
// The config schema is stable enough that re-parsing it in place
// is cheaper than plumbing a cross-package accessor.
func loadShhhInstalledState(configPath string) (paths []string, scope string) {
	if configPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, ""
		}
		configPath = filepath.Join(home, ".shhh", "config.json")
	}
	buf, err := os.ReadFile(configPath)
	if err != nil {
		return nil, ""
	}
	var minimal struct {
		Scope string   `json:"scope"`
		Paths []string `json:"installed_paths"`
	}
	if err := jsonUnmarshal(buf, &minimal); err != nil {
		return nil, ""
	}

	// Defensive: config.json can drift from the actual settings.json
	// state (e.g. user edits settings.json by hand, or pre-fix
	// uninstall didn't clean config.json). Re-read each referenced
	// settings.json and keep only the ones that genuinely contain a
	// shhh hook today. Cheap (one file per path, usually one path).
	verified := minimal.Paths[:0]
	for _, p := range minimal.Paths {
		if settingsHasShhhHook(p) {
			verified = append(verified, p)
		}
	}
	if len(verified) == 0 {
		return nil, ""
	}

	// Re-derive scope from verified paths so config drift doesn't lie:
	// any path that equals the user's global settings.json wins as
	// "global" (covers every project), otherwise scope is "project"
	// (only the per-project paths cover their respective trees).
	inferredScope := "project"
	if globalPath := globalSettingsPath(); globalPath != "" {
		for _, p := range verified {
			if filepath.Clean(p) == filepath.Clean(globalPath) {
				inferredScope = "global"
				break
			}
		}
	}
	return verified, inferredScope
}

// globalSettingsPath returns the path of the user's global Claude Code
// settings.json. Mirrors cmdinstall.claudeSettingsPath but kept here
// to avoid an import cycle (audit cannot import cmdinstall).
func globalSettingsPath() string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "settings.json")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".claude", "settings.json")
}

// settingsHasShhhHook returns true iff the file at path is a Claude
// Code settings.json containing at least one hook command that
// references the shhh binary. Tolerant of missing/malformed files —
// returns false rather than erroring.
func settingsHasShhhHook(path string) bool {
	buf, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	// We don't need to parse the schema; a substring scan over the
	// "command" strings is enough and decouples us from settings.json
	// shape changes. Avoid false positives: match "shhh hook" (the
	// subcommand) rather than the bare word "shhh", which could
	// appear in a comment or unrelated path.
	return strings.Contains(string(buf), "shhh hook")
}
