package cmdaudit

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/Musubi42/shhh/cmd/shhh/cmdinstall"
)

// runIgnore handles `shhh audit ignore <path>` and the symmetric
// `shhh audit unignore <path>`. add=true → add, add=false → remove.
//
// The path is resolved to an absolute path before storing, so
// `shhh audit ignore .` from inside the project does the right thing.
// Storing only absolute paths keeps matching exact and predictable.
func runIgnore(args []string, add bool) error {
	if len(args) != 1 {
		verb := "ignore"
		if !add {
			verb = "unignore"
		}
		return fmt.Errorf("usage: shhh audit %s <project-path>", verb)
	}
	target, err := filepath.Abs(args[0])
	if err != nil {
		return fmt.Errorf("resolve path: %w", err)
	}

	cfg, err := cmdinstall.LoadConfig()
	if err != nil {
		return fmt.Errorf("load shhh config: %w", err)
	}
	if cfg == nil {
		// No config yet — create a minimal one so the ignore list has a
		// place to live. We don't set Scope/Paths; those belong to the
		// install flow.
		cfg = &cmdinstall.Config{}
	}

	if add {
		cfg.AddIgnoredPath(target)
		if err := cmdinstall.SaveConfig(cfg); err != nil {
			return fmt.Errorf("save shhh config: %w", err)
		}
		fmt.Printf("shhh audit: ignoring %s\n", target)
		fmt.Printf("  (next `shhh audit` will skip this project entirely.)\n")
		return nil
	}

	removed := cfg.RemoveIgnoredPath(target)
	if !removed {
		fmt.Fprintf(os.Stderr, "shhh audit: %s was not in the ignore list (no change)\n", target)
		return nil
	}
	if err := cmdinstall.SaveConfig(cfg); err != nil {
		return fmt.Errorf("save shhh config: %w", err)
	}
	fmt.Printf("shhh audit: %s is back in the audit scope.\n", target)
	return nil
}

// runIgnoredList prints the persistent ignore list to stdout.
func runIgnoredList() error {
	cfg, err := cmdinstall.LoadConfig()
	if err != nil {
		return fmt.Errorf("load shhh config: %w", err)
	}
	if cfg == nil || len(cfg.IgnoredPaths) == 0 {
		fmt.Println("shhh audit: no projects in the ignore list.")
		fmt.Println("  Add one with:  shhh audit ignore <project-path>")
		return nil
	}
	fmt.Printf("shhh audit: %d project(s) ignored\n\n", len(cfg.IgnoredPaths))
	for _, p := range cfg.IgnoredPaths {
		fmt.Printf("  %s\n", p)
	}
	fmt.Println()
	fmt.Println("  Remove one with:  shhh audit unignore <project-path>")
	return nil
}
