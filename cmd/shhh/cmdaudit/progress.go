package cmdaudit

import (
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	auditpkg "github.com/Musubi42/shhh/internal/audit"
)

// maxVisibleEntries caps how many scroll-log rows the live UI shows
// at once. Older rows scroll off the top conceptually (we still keep
// them in memory for upgrades). 18 is enough for a typical audit
// (23 projects) without overflowing an 80x24 terminal.
const maxVisibleEntries = 18

// minSessionsForETA suppresses ETA until we have enough data points
// for a stable estimate. With <5% of sessions done, ETA wobbles
// wildly and damages trust.
const minSessionsForETA = 30

// progressRenderer turns a stream of audit.ProgressEvent into a live
// terminal UI. The audit can take minutes — instead of a frozen
// terminal, the user gets a scrolling log of per-project results
// plus a fixed footer with timer, counters, leaked-so-far, and ETA.
//
// In-place redraw strategy: the renderer owns a list of "entries"
// (one per project) and a footer. On every state change, it clears
// the lines it previously drew (ANSI cursor-up + erase) and reprints
// the full block. Entries can upgrade in place — a project starts
// life as "scanning", becomes "scanned" when its transcripts are
// done, and is "final" once findings are aggregated.
//
// On a non-TTY destination (pipe, CI, redirect) the renderer falls
// back to flat one-line-per-event logging so logs stay parseable.
type progressRenderer struct {
	out   io.Writer
	isTTY bool

	// scopeLabel is a short display string injected by cmdaudit when
	// the user passed positional scope paths (e.g. `scope ./`).
	// Empty when no scope filter is active. Surfaced in the
	// ProgressEnumerated announcement so the user sees that the
	// project count is scope-restricted, not the total inventory.
	scopeLabel string

	mu    sync.Mutex
	start time.Time

	enumerated    bool
	projectsTotal int
	sessionsTotal int

	sessionsDone int
	projectsDone int // projects whose transcripts are fully read
	leakedTotal  int
	atRiskTotal  int

	entries    []scrollEntry  // in display order
	entryIndex map[string]int // dashName → index into entries

	drawnLines int // count of lines the live block currently occupies
}

// scrollEntry is one row in the scroll log.
type scrollEntry struct {
	dashName string
	display  string
	sessions int
	leaked   int
	atRisk   int
	state    entryState
	status   auditpkg.Status
}

type entryState int

const (
	entryScanned   entryState = iota // transcripts done, findings not yet aggregated
	entryFinalized                   // status decided, findings counted
)

func newProgressRenderer(out io.Writer, isTTY bool) *progressRenderer {
	return &progressRenderer{
		out:        out,
		isTTY:      isTTY,
		start:      time.Now(),
		entryIndex: make(map[string]int, 32),
	}
}

// withScope returns r with a scope label that will appear in the
// "scanning N projects" announcement. Called by cmdaudit when the
// user passed positional scope paths.
func (r *progressRenderer) withScope(label string) *progressRenderer {
	r.scopeLabel = label
	return r
}

// Handle consumes one progress event from auditpkg.Run. It is safe
// to call from any goroutine.
func (r *progressRenderer) Handle(e auditpkg.ProgressEvent) {
	r.mu.Lock()
	defer r.mu.Unlock()

	switch e.Kind {
	case auditpkg.ProgressEnumerated:
		r.enumerated = true
		r.projectsTotal = e.ProjectsTotal
		r.sessionsTotal = e.SessionsTotal
		scopePrefix := ""
		if r.scopeLabel != "" {
			scopePrefix = " " + r.scopeLabel + ","
		}
		if r.isTTY {
			fmt.Fprintf(r.out, "🛡️  shhh audit —%s scanning %d projects (≈%d sessions)\n\n",
				scopePrefix, e.ProjectsTotal, e.SessionsTotal)
			r.redraw()
		} else {
			fmt.Fprintf(r.out, "🛡️  shhh audit —%s scanning %d projects, ~%d sessions\n",
				scopePrefix, e.ProjectsTotal, e.SessionsTotal)
		}

	case auditpkg.ProgressSessionFinished:
		r.sessionsDone++
		if r.isTTY {
			r.redraw()
		}

	case auditpkg.ProgressProjectScanned:
		r.projectsDone++
		idx, ok := r.entryIndex[e.ProjectDashName]
		if !ok {
			r.entries = append(r.entries, scrollEntry{
				dashName: e.ProjectDashName,
				display:  e.ProjectDisplay,
				sessions: e.Sessions,
				state:    entryScanned,
			})
			r.entryIndex[e.ProjectDashName] = len(r.entries) - 1
		} else {
			r.entries[idx].sessions = e.Sessions
			r.entries[idx].display = e.ProjectDisplay
		}
		if r.isTTY {
			r.redraw()
		} else {
			fmt.Fprintf(r.out, "  scanned  %s  (%d sessions)\n", e.ProjectDisplay, e.Sessions)
		}

	case auditpkg.ProgressFinding:
		r.leakedTotal++
		if r.isTTY {
			r.redraw()
		}

	case auditpkg.ProgressProjectFinished:
		// Upgrade the scroll entry in place with final findings + status.
		// This event arrives in the post-scan loop, after which we'll
		// see ProgressDone and the live UI tears down.
		idx, ok := r.entryIndex[e.ProjectDashName]
		if !ok {
			// Projects with no transcripts never went through Scanned;
			// add them now so they appear in the scroll log.
			r.entries = append(r.entries, scrollEntry{
				dashName: e.ProjectDashName,
				display:  e.ProjectDisplay,
				sessions: e.Sessions,
			})
			idx = len(r.entries) - 1
			r.entryIndex[e.ProjectDashName] = idx
		}
		r.entries[idx].display = e.ProjectDisplay
		r.entries[idx].sessions = e.Sessions
		r.entries[idx].leaked = e.Leaked
		r.entries[idx].atRisk = e.AtRisk
		r.entries[idx].status = e.Status
		r.entries[idx].state = entryFinalized
		r.atRiskTotal += e.AtRisk
		if r.isTTY {
			r.redraw()
		}

	case auditpkg.ProgressDone:
		// Freeze the live block in place as the user's scan summary.
		// Previously we cleared it here, but the post-scan loop that
		// upgrades each entry from ⟳ to ✓ ran in <100ms so the user
		// never saw their finalized results. Leave them on screen;
		// the full per-project report renders below them.
		if r.isTTY {
			r.drawnLines = 0 // release ownership; future text appends below
			fmt.Fprintln(r.out)
			fmt.Fprintf(r.out, "  scan complete in %s — rendering full report…\n\n", r.elapsed())
		} else {
			fmt.Fprintf(r.out, "scan complete in %s\n", r.elapsed())
		}

	case auditpkg.ProgressSourceCount:
		// Tracked-but-unused: per-source line counts. Kept for future
		// debug paths; the user-facing counter is sessions, not events.
	}
}

// redraw is the single source of truth for the live UI. It clears
// whatever was drawn last, then prints scroll entries (capped to the
// most recent maxVisibleEntries) followed by separator + footer.
func (r *progressRenderer) redraw() {
	r.clearLiveBlock()

	// Render only the most recent N entries — older ones are kept in
	// memory but don't fit on a typical terminal.
	visible := r.entries
	if len(visible) > maxVisibleEntries {
		visible = visible[len(visible)-maxVisibleEntries:]
	}
	for _, e := range visible {
		fmt.Fprintln(r.out, formatEntry(e))
	}

	fmt.Fprintln(r.out, "  ────────────────────────────────────────────────────────────")
	fmt.Fprintln(r.out, r.formatFooterBody())

	r.drawnLines = len(visible) + 2
}

// clearLiveBlock walks the cursor up over drawnLines and erases each
// line. After it returns, cursor sits at the start of where the live
// block began and drawnLines is 0.
func (r *progressRenderer) clearLiveBlock() {
	for i := 0; i < r.drawnLines; i++ {
		// \033[1A : cursor up one line
		// \033[2K : erase the whole line
		// \r      : defensive — back to column 0
		fmt.Fprint(r.out, "\033[1A\033[2K\r")
	}
	r.drawnLines = 0
}

func (r *progressRenderer) formatFooterBody() string {
	parts := []string{fmt.Sprintf("  [%s]", r.elapsed())}
	if r.projectsTotal > 0 {
		parts = append(parts, fmt.Sprintf("%d/%d projects",
			r.projectsDone, r.projectsTotal))
	}
	if r.sessionsTotal > 0 {
		parts = append(parts, fmt.Sprintf("%d/%d sessions",
			r.sessionsDone, r.sessionsTotal))
	}
	if r.leakedTotal > 0 {
		parts = append(parts, fmt.Sprintf("🚨 %d leaked", r.leakedTotal))
	}
	if eta := r.eta(); eta != "" {
		parts = append(parts, "ETA "+eta)
	}
	return strings.Join(parts, " · ")
}

// eta returns a formatted estimated-time-to-completion, or "" if it
// is too early to give a meaningful number.
func (r *progressRenderer) eta() string {
	if r.sessionsTotal == 0 || r.sessionsDone < minSessionsForETA {
		return ""
	}
	if r.sessionsDone >= r.sessionsTotal {
		return ""
	}
	elapsed := time.Since(r.start)
	perSession := elapsed / time.Duration(r.sessionsDone)
	remaining := perSession * time.Duration(r.sessionsTotal-r.sessionsDone)
	if remaining < 30*time.Second {
		return "almost done"
	}
	return formatDuration(remaining)
}

func (r *progressRenderer) elapsed() string {
	return formatDuration(time.Since(r.start))
}

func formatDuration(d time.Duration) string {
	d = d.Round(time.Second)
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%02d:%02d", m, s)
}

// formatEntry renders one scroll-log row. The format and width
// match what formatFooterBody produces so columns roughly line up.
func formatEntry(e scrollEntry) string {
	icon, status, findings := "·", "", ""
	switch e.state {
	case entryScanned:
		icon = "⟳"
		findings = "transcripts scanned"
	case entryFinalized:
		status = statusBadge(e.status)
		findings = formatFindings(e.leaked, e.atRisk)
		if e.status == auditpkg.StatusUnprotected && (e.leaked > 0 || e.atRisk > 0) {
			icon = "!"
		} else {
			icon = "✓"
		}
	}
	return fmt.Sprintf("  %s %-50s %5d sess   %-22s  %s",
		icon,
		truncatePath(e.display, 50),
		e.sessions,
		findings,
		status,
	)
}

func statusBadge(s auditpkg.Status) string {
	switch s {
	case auditpkg.StatusProtected:
		return "HOOKED"
	case auditpkg.StatusArchived:
		return "folder gone"
	case auditpkg.StatusUnprotected:
		return "NOT HOOKED"
	case auditpkg.StatusClean:
		return "clean"
	default:
		return ""
	}
}

func formatFindings(leaked, atRisk int) string {
	if leaked == 0 && atRisk == 0 {
		return "0 findings"
	}
	var parts []string
	if leaked > 0 {
		parts = append(parts, fmt.Sprintf("🚨 %d leaked", leaked))
	}
	if atRisk > 0 {
		parts = append(parts, fmt.Sprintf("⚠️ %d on disk", atRisk))
	}
	return strings.Join(parts, ", ")
}

func truncatePath(p string, max int) string {
	if len(p) <= max {
		return p
	}
	if max <= 1 {
		return p[:max]
	}
	return "…" + p[len(p)-max+1:]
}
