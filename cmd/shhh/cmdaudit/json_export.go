package cmdaudit

import (
	"encoding/json"
	"io"
	"time"

	auditpkg "github.com/Musubi42/shhh/internal/audit"
)

// JSON DTO structs. We never marshal auditpkg types directly; the
// JSON shape is a stable external contract and must not drift with
// internal refactors.

type jsonResult struct {
	SchemaVersion  int          `json:"schema_version"`
	AuditTime      time.Time    `json:"audit_time"`
	Agent          string       `json:"agent"`
	ScanDurationMs int64        `json:"scan_duration_ms"`
	Summary        jsonSummary  `json:"summary"`
	DeltaSince     *time.Time   `json:"delta_since,omitempty"`
	Delta          *jsonDelta   `json:"delta,omitempty"`
	Projects       []jsonProj   `json:"projects"`
}

type jsonSummary struct {
	ProjectsTotal       int `json:"projects_total"`
	ProjectsUnprotected int `json:"projects_unprotected"`
	ProjectsProtected   int `json:"projects_protected"`
	ProjectsArchived    int `json:"projects_archived"`
	ProjectsClean       int `json:"projects_clean"`
	SecretsLeaked       int `json:"secrets_leaked"`
	SecretsAtRisk       int `json:"secrets_at_risk"`
	SecretsProtected    int `json:"secrets_protected"`
}

type jsonDelta struct {
	Leaked    jsonDeltaCount `json:"leaked"`
	AtRisk    jsonDeltaCount `json:"at_risk"`
	Protected jsonDeltaCount `json:"protected"`
}

type jsonDeltaCount struct {
	Before int `json:"before"`
	After  int `json:"after"`
	Change int `json:"change"`
}

type jsonProj struct {
	Path          string         `json:"path"`
	DisplayPath   string         `json:"display_path"`
	Status        string         `json:"status"`
	OnDisk        bool           `json:"on_disk"`
	SessionsTotal int            `json:"sessions_total"`
	FirstSeen     time.Time      `json:"first_seen"`
	Leaked        []jsonLeaked   `json:"leaked"`
	AtRisk        []jsonAtRisk   `json:"at_risk"`
}

type jsonLeaked struct {
	Placeholder  string    `json:"placeholder"`
	Label        string    `json:"label"`
	FirstSeen    time.Time `json:"first_seen"`
	LastSeen     time.Time `json:"last_seen"`
	SessionCount int       `json:"session_count"`
	Sources      []string  `json:"sources"`
	RotationURL  string    `json:"rotation_url,omitempty"`
}

type jsonAtRisk struct {
	Placeholder string `json:"placeholder"`
	Label       string `json:"label"`
	Location    string `json:"location"`
	// TODO(v0.3): wire detector rule name through Finding so we can
	// populate this field. For now we always omit it.
	Rule string `json:"rule,omitempty"`
}

// RenderJSON writes a machine-readable JSON representation of the
// Result to w. Safe for CI pipelines and pastable into PR comments:
// contains only placeholders, never raw secret values.
//
// The JSON shape is documented in docs/dev/design/cli-output.md under
// "JSON output mode".
func RenderJSON(w io.Writer, r *auditpkg.Result) error {
	out := jsonResult{
		SchemaVersion:  r.SchemaVersion,
		AuditTime:      r.AuditTime.UTC(),
		Agent:          r.Agent,
		ScanDurationMs: r.ScanDuration.Milliseconds(),
		Summary: jsonSummary{
			ProjectsTotal:       r.Summary.ProjectsTotal,
			ProjectsUnprotected: r.Summary.ProjectsUnprotected,
			ProjectsProtected:   r.Summary.ProjectsProtected,
			ProjectsArchived:    r.Summary.ProjectsArchived,
			ProjectsClean:       r.Summary.ProjectsClean,
			SecretsLeaked:       r.Summary.SecretsLeaked,
			SecretsAtRisk:       r.Summary.SecretsAtRisk,
			SecretsProtected:    r.Summary.SecretsProtected,
		},
		Projects: make([]jsonProj, 0, len(r.Projects)),
	}

	if r.Delta != nil {
		since := r.Delta.Since.UTC()
		out.DeltaSince = &since
		out.Delta = &jsonDelta{
			Leaked:    jsonDeltaCount{r.Delta.Leaked.Before, r.Delta.Leaked.After, r.Delta.Leaked.Change},
			AtRisk:    jsonDeltaCount{r.Delta.AtRisk.Before, r.Delta.AtRisk.After, r.Delta.AtRisk.Change},
			Protected: jsonDeltaCount{r.Delta.Protected.Before, r.Delta.Protected.After, r.Delta.Protected.Change},
		}
	}

	for _, p := range r.Projects {
		jp := jsonProj{
			Path:          p.AbsPath,
			DisplayPath:   p.DisplayPath,
			Status:        string(p.Status),
			OnDisk:        p.OnDisk,
			SessionsTotal: p.SessionsTotal,
			FirstSeen:     p.FirstSeen.UTC(),
			Leaked:        make([]jsonLeaked, 0, len(p.Leaked)),
			AtRisk:        make([]jsonAtRisk, 0, len(p.AtRisk)),
		}
		for _, f := range p.Leaked {
			sessionCount := len(f.SessionIDs)
			if sessionCount == 0 {
				sessionCount = f.Occurrences
			}
			jp.Leaked = append(jp.Leaked, jsonLeaked{
				Placeholder:  f.Placeholder,
				Label:        f.Label,
				FirstSeen:    f.FirstSeen.UTC(),
				LastSeen:     f.LastSeen.UTC(),
				SessionCount: sessionCount,
				Sources:      f.Sources,
				RotationURL:  f.RotationURL,
			})
		}
		for _, f := range p.AtRisk {
			loc := ""
			if len(f.Locations) > 0 {
				loc = f.Locations[0]
			}
			jp.AtRisk = append(jp.AtRisk, jsonAtRisk{
				Placeholder: f.Placeholder,
				Label:       f.Label,
				Location:    loc,
			})
		}
		out.Projects = append(out.Projects, jp)
	}

	buf, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return err
	}
	_, err = w.Write(buf)
	if err != nil {
		return err
	}
	_, err = w.Write([]byte{'\n'})
	return err
}
