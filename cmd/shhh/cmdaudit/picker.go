package cmdaudit

import (
	"fmt"
	"sort"

	"github.com/charmbracelet/huh"

	"github.com/Musubi42/shhh/cmd/shhh/cmdinstall"
	auditpkg "github.com/Musubi42/shhh/internal/audit"
)

// runProjectPicker shows a checklist of every Claude Code project,
// pre-selected except those already in the user's persistent ignore
// list, and writes the user's choices back to ~/.shhh/config.json
// before the scan starts.
//
// Returns the (possibly updated) list of ignored paths the audit
// should honor. A picker cancel (Esc / Ctrl-C) returns the original
// ignore list unchanged and a sentinel ErrPickerCanceled — callers
// translate that into a clean exit.
//
// Skip the picker entirely (non-TTY, --no-select, no projects, or
// no huh runner available) by simply not calling this function.
func runProjectPicker() ([]string, error) {
	projects, err := auditpkg.EnumerateProjects("")
	if err != nil {
		return nil, fmt.Errorf("enumerate projects: %w", err)
	}
	if len(projects) == 0 {
		return nil, nil
	}

	cfg, err := cmdinstall.LoadConfig()
	if err != nil {
		return nil, fmt.Errorf("load shhh config: %w", err)
	}
	if cfg == nil {
		cfg = &cmdinstall.Config{}
	}
	ignored := make(map[string]bool, len(cfg.IgnoredPaths))
	for _, p := range cfg.IgnoredPaths {
		ignored[p] = true
	}

	// Sort by sessions desc (most active first), then by display path.
	sort.SliceStable(projects, func(i, j int) bool {
		if projects[i].SessionsTotal != projects[j].SessionsTotal {
			return projects[i].SessionsTotal > projects[j].SessionsTotal
		}
		return projects[i].DisplayPath < projects[j].DisplayPath
	})

	// Build huh options. Value = abs path (used as the stable key);
	// label = display path + session count + folder-gone marker.
	options := make([]huh.Option[string], 0, len(projects))
	preSelected := make([]string, 0, len(projects))
	for _, p := range projects {
		gone := ""
		if !p.OnDisk {
			gone = "  [folder gone]"
		}
		label := fmt.Sprintf("%s  (%d session%s)%s",
			p.DisplayPath, p.SessionsTotal, plural(p.SessionsTotal), gone)
		options = append(options, huh.NewOption(label, p.AbsPath))
		if !ignored[p.AbsPath] {
			preSelected = append(preSelected, p.AbsPath)
		}
	}

	totalSessions := 0
	for _, p := range projects {
		totalSessions += p.SessionsTotal
	}

	selected := preSelected
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewMultiSelect[string]().
				Title(fmt.Sprintf("Select projects to audit  ·  %d projects · %d sessions total",
					len(projects), totalSessions)).
				Description("Space to toggle · Enter to start · Esc to cancel\nUnchecked projects are saved to your ignore list.").
				Options(options...).
				Value(&selected).
				Height(min(20, len(options)+2)),
		),
	)

	if err := form.Run(); err != nil {
		return cfg.IgnoredPaths, fmt.Errorf("picker: %w", err)
	}

	// Compute the new ignored set: every project NOT selected.
	chosen := make(map[string]bool, len(selected))
	for _, p := range selected {
		chosen[p] = true
	}
	newIgnored := make([]string, 0, len(projects)-len(selected))
	for _, p := range projects {
		if !chosen[p.AbsPath] {
			newIgnored = append(newIgnored, p.AbsPath)
		}
	}
	sort.Strings(newIgnored)

	// Persist only if anything changed.
	if !stringSliceEqual(newIgnored, cfg.IgnoredPaths) {
		cfg.IgnoredPaths = newIgnored
		if err := cmdinstall.SaveConfig(cfg); err != nil {
			return newIgnored, fmt.Errorf("save shhh config: %w", err)
		}
	}

	return newIgnored, nil
}

func stringSliceEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
