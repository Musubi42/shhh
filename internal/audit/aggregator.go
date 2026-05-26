package audit

import (
	"context"
	"sort"
	"sync"
	"time"

	"github.com/Musubi42/shhh/internal/detector"
	"github.com/Musubi42/shhh/internal/redactor"
	"github.com/Musubi42/shhh/internal/session"
)

// rawFinding is the per-item result of running the detector on an
// AuditItem's content. The aggregator groups these by secret value to
// produce the final per-project Finding list.
//
// We key on session.PlaceholderFor(value, label, ...) rather than on
// the raw value: the placeholder is deterministic within an aggregator
// instance (same salt throughout the audit), and keying on it means
// the aggregator never holds a map with raw secret values as keys
// beyond the narrow scope of the detector invocation.
type rawFinding struct {
	placeholder    string
	label          string
	sourceName     string
	projectDashKey string    // may be "" for paste-cache / unresolved file-history
	sessionID      string
	location       string
	timestamp      time.Time
}

// aggregator accumulates rawFindings across all sources and produces
// the final []Project shaped output.
type aggregator struct {
	mu       sync.Mutex
	redactor *redactor.Redactor
	sess     *session.Map

	// findings is every raw observation, flat. The grouping happens
	// in Finalize once all sources are drained.
	findings []rawFinding

	// onFinding, if non-nil, is called once per unique (placeholder,
	// project) pair as Process discovers it. Used by cmdaudit to tick
	// a live "leaked so far" counter without waiting for Finalize().
	// Called while the aggregator mutex is held — callbacks must be
	// fast and must not call back into the aggregator.
	onFinding func(placeholder, projectDashName string)
	seen      map[string]bool
}

func newAggregator() *aggregator {
	sess := session.New()
	return &aggregator{
		redactor: redactor.New(detector.New(), sess),
		sess:     sess,
		findings: make([]rawFinding, 0, 128),
		seen:     make(map[string]bool, 64),
	}
}

// SetOnFinding wires a live-counter callback. Must be called before
// Process. Safe to leave nil for tests / non-interactive paths.
func (a *aggregator) SetOnFinding(fn func(placeholder, projectDashName string)) {
	a.onFinding = fn
}

// Process runs the detector over one AuditItem and records any
// findings. Thread-safe: multiple sources can push items concurrently.
//
// The aggregator deliberately filters out HIGH_ENTROPY findings.
// Rationale: the entropy fallback in internal/detector is tuned to
// catch unnamed high-entropy tokens in arbitrary source code — but
// Claude Code's own transcripts are packed with high-entropy tokens
// that are NOT secrets: session UUIDs, tool_use_ids, git SHAs,
// message IDs, chunked tokenizer IDs, etc. Surfacing those would
// drown real findings in thousands of false positives (verified at
// 8000+ on a real machine during v0.2 bring-up).
//
// The audit's product value is "Claude saw your named secret"
// (STRIPE_LIVE_KEY, AWS_ACCESS_KEY, OPENAI_PROJECT_KEY, …), not
// "Claude saw a high-entropy string that could theoretically be
// a secret." Named rules stay; entropy fallback is suppressed.
//
// User-named .env tokens that lack a pattern rule are still caught
// in the AT-RISK path via ScanAtRiskFile, which uses the looser
// env-file detector pass (RedactEnvFile) and DOES surface them.
func (a *aggregator) Process(item AuditItem) {
	if item.Content == "" {
		return
	}
	// Run the detector. We don't care about the redacted bytes here,
	// only the findings list and the session map side-effects that
	// assign placeholders to each value.
	_, finds := a.redactor.RedactString(item.Content)
	if len(finds) == 0 {
		return
	}

	a.mu.Lock()
	defer a.mu.Unlock()
	for _, f := range finds {
		if f.Label == "HIGH_ENTROPY" {
			continue
		}
		// Placeholder for this exact value is now in the session map.
		// Re-derive it (PlaceholderFor is idempotent within a session)
		// to avoid parsing the redacted bytes.
		placeholder := a.sess.PlaceholderFor(f.Value, f.Label, f.PublicPrefix, f.StructuralDesc)
		a.findings = append(a.findings, rawFinding{
			placeholder:    placeholder,
			label:          f.Label,
			sourceName:     item.SourceName,
			projectDashKey: item.ProjectDashName,
			sessionID:      item.SessionID,
			location:       item.Location,
			timestamp:      item.Timestamp,
		})
		if a.onFinding != nil {
			key := placeholder + "@" + item.ProjectDashName
			if !a.seen[key] {
				a.seen[key] = true
				a.onFinding(placeholder, item.ProjectDashName)
			}
		}
	}
}

// Finalize collapses the raw flat findings into per-project, per-
// severity aggregated Findings. Implements the cross-source
// attribution trick: findings from sources without project attribution
// (paste-cache, unresolved file-history) adopt the project of any
// transcript/prompt-history finding that carries the same placeholder.
// If no such transcript exists, they fall into the
// unattributedProjectKey bucket.
//
// The aggregator returns a map from projectDashKey to the grouped
// findings for that project. The top-level Run function then joins
// this with the project registry (see Run).
func (a *aggregator) Finalize() map[string]projectFindings {
	a.mu.Lock()
	defer a.mu.Unlock()

	// Step 1: build a map from placeholder to the first non-empty
	// project attribution we saw for it. This lets us back-fill
	// attribution for paste-cache and unresolved file-history items.
	resolveProject := make(map[string]string, len(a.findings))
	for _, rf := range a.findings {
		if rf.projectDashKey == "" {
			continue
		}
		if _, ok := resolveProject[rf.placeholder]; !ok {
			resolveProject[rf.placeholder] = rf.projectDashKey
		}
	}

	// Step 2: propagate project attribution across all findings for
	// the same placeholder. Findings without a project adopt the
	// resolved one; findings with a project keep theirs.
	perProject := make(map[string]map[string]*Finding)
	for _, rf := range a.findings {
		key := rf.projectDashKey
		if key == "" {
			if resolved, ok := resolveProject[rf.placeholder]; ok {
				key = resolved
			} else {
				key = unattributedKey
			}
		}
		byLabel, ok := perProject[key]
		if !ok {
			byLabel = make(map[string]*Finding)
			perProject[key] = byLabel
		}
		// Group within a project by (placeholder) so the same secret
		// seen in multiple sources/sessions collapses to one Finding
		// with enriched metadata.
		existing, ok := byLabel[rf.placeholder]
		if !ok {
			existing = &Finding{
				Placeholder: rf.placeholder,
				Label:       rf.label,
				Severity:    SevLeaked, // anything surfaced by an audit source is "leaked"
				Sources:     nil,
				Occurrences: 0,
				FirstSeen:   time.Time{},
				LastSeen:    time.Time{},
				Locations:   nil,
				SessionIDs:  nil,
				RotationURL: rotationURLFor(rf.label),
			}
			byLabel[rf.placeholder] = existing
		}
		existing.Occurrences++
		existing.Sources = addUnique(existing.Sources, rf.sourceName)
		if rf.sessionID != "" {
			existing.SessionIDs = addUnique(existing.SessionIDs, rf.sessionID)
		}
		if rf.location != "" {
			existing.Locations = addUnique(existing.Locations, rf.location)
		}
		if !rf.timestamp.IsZero() {
			if existing.FirstSeen.IsZero() || rf.timestamp.Before(existing.FirstSeen) {
				existing.FirstSeen = rf.timestamp
			}
			if existing.LastSeen.IsZero() || rf.timestamp.After(existing.LastSeen) {
				existing.LastSeen = rf.timestamp
			}
		}
	}

	// Step 3: flatten into projectFindings with sorted leaked slices.
	out := make(map[string]projectFindings, len(perProject))
	for key, byLabel := range perProject {
		findings := make([]Finding, 0, len(byLabel))
		for _, f := range byLabel {
			findings = append(findings, *f)
		}
		sort.Slice(findings, func(i, j int) bool {
			if findings[i].FirstSeen.Equal(findings[j].FirstSeen) {
				return findings[i].Label < findings[j].Label
			}
			if findings[i].FirstSeen.IsZero() {
				return false
			}
			if findings[j].FirstSeen.IsZero() {
				return true
			}
			return findings[i].FirstSeen.Before(findings[j].FirstSeen)
		})
		out[key] = projectFindings{Leaked: findings}
	}
	return out
}

// ScanAtRiskFile runs the detector on a single file's content and
// returns an at-risk Finding per detected secret. Used by Run when
// walking a project's current on-disk files.
//
// The file's content is NOT added to the raw findings list — at-risk
// is a separate severity and goes directly into the project's AtRisk
// slice at join time. But we DO use the same session map so placeholder
// values stay consistent with leaked findings in the same audit.
func (a *aggregator) ScanAtRiskFile(path, content string) []Finding {
	if content == "" {
		return nil
	}
	_, finds := a.redactor.RedactString(content)
	if len(finds) == 0 {
		return nil
	}
	out := make([]Finding, 0, len(finds))
	seen := make(map[string]bool)
	for _, f := range finds {
		if f.Label == "HIGH_ENTROPY" {
			continue
		}
		placeholder := a.sess.PlaceholderFor(f.Value, f.Label, f.PublicPrefix, f.StructuralDesc)
		if seen[placeholder] {
			continue
		}
		seen[placeholder] = true
		out = append(out, Finding{
			Placeholder: placeholder,
			Label:       f.Label,
			Severity:    SevAtRisk,
			Sources:     []string{"project-file"},
			Occurrences: 1,
			Locations:   []string{path},
			RotationURL: rotationURLFor(f.Label),
		})
	}
	return out
}

// projectFindings is the per-project grouping the aggregator returns.
// Right now only Leaked is populated — AtRisk comes from
// ScanAtRiskFile and is joined at the Run level.
type projectFindings struct {
	Leaked []Finding
}

// unattributedKey is the pseudo-project key for findings that have no
// resolvable project. Cross-source attribution collapses most of
// these; only truly orphaned findings end up here. Run handles them
// by either dropping (v0.2 policy: drop — they'd show up as an
// "unknown project" block nobody wants) or folding into an "orphan"
// bucket in a future version.
const unattributedKey = "__unattributed__"

// addUnique returns slice with v appended iff it's not already present.
// Used for Sources, SessionIDs, Locations — order is insertion order,
// which matches the order findings stream in and is stable enough for
// tests.
func addUnique(slice []string, v string) []string {
	for _, existing := range slice {
		if existing == v {
			return slice
		}
	}
	return append(slice, v)
}

// rotationURLFor returns the best-known rotation dashboard URL for a
// given finding Label. Returns "" for labels we don't have a mapping
// for. The renderer's rotationURLs map is a richer version of this;
// keep them in sync when adding new entries.
func rotationURLFor(label string) string {
	switch {
	case startsWith(label, "STRIPE"):
		return "https://dashboard.stripe.com/apikeys"
	case startsWith(label, "AWS"):
		return "https://console.aws.amazon.com/iam/home#/users"
	case startsWith(label, "OPENAI"):
		return "https://platform.openai.com/api-keys"
	case startsWith(label, "GITHUB"):
		return "https://github.com/settings/tokens"
	case startsWith(label, "SENDGRID"):
		return "https://app.sendgrid.com/settings/api_keys"
	default:
		return ""
	}
}

func startsWith(s, prefix string) bool {
	if len(s) < len(prefix) {
		return false
	}
	return s[:len(prefix)] == prefix
}

// drainSources spins up a goroutine per source, fans all AuditItems
// into the aggregator, and waits for all sources to finish.
//
// If progress is non-nil, it is called periodically with a
// per-source item count so callers can render a live progress
// indicator. Progress calls are serialized on a single goroutine so
// the callback does not need to be thread-safe, but the callback
// should return quickly.
func drainSources(ctx context.Context, sources []AuditSource, selectedProjects []string, agg *aggregator, progress func(sourceName string, count int)) {
	var wg sync.WaitGroup
	items := make(chan AuditItem, 64)

	counts := make(map[string]int)
	lastReport := make(map[string]int)

	// Consumer — single goroutine so the aggregator's mutex contention
	// is minimal and Process ordering is deterministic enough for tests.
	consumerDone := make(chan struct{})
	go func() {
		defer close(consumerDone)
		for {
			select {
			case <-ctx.Done():
				return
			case item, ok := <-items:
				if !ok {
					return
				}
				agg.Process(item)
				if progress != nil {
					counts[item.SourceName]++
					// Rate-limit: report every 25 items per source
					// to avoid spamming stderr.
					if counts[item.SourceName]-lastReport[item.SourceName] >= 25 {
						lastReport[item.SourceName] = counts[item.SourceName]
						progress(item.SourceName, counts[item.SourceName])
					}
				}
			}
		}
	}()

	// Producers.
	for _, src := range sources {
		wg.Add(1)
		go func(s AuditSource) {
			defer wg.Done()
			_ = s.Walk(ctx, selectedProjects, items)
		}(src)
	}
	wg.Wait()
	close(items)
	<-consumerDone

	// Final report per source.
	if progress != nil {
		for name, n := range counts {
			if n != lastReport[name] {
				progress(name, n)
			}
		}
	}
}
