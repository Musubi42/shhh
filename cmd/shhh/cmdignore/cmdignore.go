// Package cmdignore implements `shhh ignore` — inspect, add to, and
// query the layered `.shhhignore` rule set. The detector skip-list,
// nothing to do with the hook bypass (feature B). See
// docs/engine-architecture.md §2.2 for the distinction.
package cmdignore

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"

	"github.com/Musubi42/shhh/internal/ignore"
)

// Run dispatches to one of the three subcommands.
func Run(args []string) error {
	if len(args) == 0 {
		return runList(nil)
	}
	sub := args[0]
	rest := args[1:]
	switch sub {
	case "list":
		return runList(rest)
	case "add":
		return runAdd(rest)
	case "check":
		return runCheck(rest)
	default:
		return fmt.Errorf("unknown subcommand %q (want: list, add, check)", sub)
	}
}

// runList prints every layer in the resolved cascade with its rules.
// The gitleaks layer is summarised with a versioned GitHub link
// rather than listing 30+ regexes inline — readers who care can
// click through.
func runList(_ []string) error {
	cwd, _ := os.Getwd()
	matcher, err := ignore.BuildLayered(cwd, os.Stderr)
	if err != nil {
		return err
	}
	fmt.Println()
	fmt.Println("shhh ignore — active rule cascade (lowest → highest priority)")
	fmt.Println()
	for _, layer := range matcher.Layers() {
		switch l := layer.(type) {
		case *ignore.GitleaksLayer:
			ver := gitleaksVersion()
			fmt.Printf("• gitleaks built-in allowlist (regex, pinned to %s)\n", ver)
			fmt.Printf("  source: https://github.com/gitleaks/gitleaks/blob/%s/config/gitleaks.toml\n", ver)
			fmt.Println()
		case *ignore.FileLayer:
			rules := l.Rules()
			if len(rules) == 0 {
				fmt.Printf("• %s\n  (file does not exist or is empty)\n\n", l.Source())
				continue
			}
			fmt.Printf("• %s\n", l.Source())
			for _, r := range rules {
				fmt.Printf("    %s\n", r)
			}
			fmt.Println()
		default:
			fmt.Printf("• %s\n  (opaque layer)\n\n", layer.Source())
		}
	}
	fmt.Println("Tip: `shhh ignore add <pattern>` appends a rule to your project (or use --global).")
	fmt.Println("     `shhh ignore check <path>` shows which layer decides a given path.")
	return nil
}

// runAdd appends a single pattern to the global or project
// `.shhhignore`. Default is project (the most common case for
// adding repo-specific exemptions); --global writes to ~/.shhh.
func runAdd(args []string) error {
	scope := "project"
	var pattern string
	for _, a := range args {
		switch a {
		case "--global":
			scope = "global"
		case "--project":
			scope = "project"
		default:
			if strings.HasPrefix(a, "-") {
				return fmt.Errorf("unknown flag %q (want --global or --project)", a)
			}
			if pattern != "" {
				return fmt.Errorf("usage: shhh ignore add <pattern> [--global|--project]")
			}
			pattern = a
		}
	}
	if pattern == "" {
		return fmt.Errorf("usage: shhh ignore add <pattern> [--global|--project]")
	}

	cwd, _ := os.Getwd()
	globalPath, projectPath, err := ignore.DefaultPaths(cwd)
	if err != nil {
		return err
	}
	target := projectPath
	if scope == "global" {
		target = globalPath
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}
	f, err := os.OpenFile(target, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("open %s: %w", target, err)
	}
	defer f.Close()
	if _, err := fmt.Fprintln(f, pattern); err != nil {
		return err
	}
	fmt.Printf("shhh: appended %q to %s\n", pattern, target)
	return nil
}

// runCheck reports which layer (and which rule) decides a given
// path's ignore status. Useful for "why is this file getting
// scanned/skipped".
func runCheck(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: shhh ignore check <path>")
	}
	path := args[0]
	cwd, _ := os.Getwd()
	matcher, err := ignore.BuildLayered(cwd, os.Stderr)
	if err != nil {
		return err
	}
	// Path is matched relative to the scan root, so normalise input.
	rel := path
	if abs, aerr := filepath.Abs(path); aerr == nil {
		if r, rerr := filepath.Rel(cwd, abs); rerr == nil {
			rel = r
		}
	}
	src, decision := matcher.Explain(rel)
	switch decision {
	case ignore.Ignored:
		fmt.Printf("IGNORED — %q matched layer: %s\n", rel, src)
	case ignore.Included:
		fmt.Printf("INCLUDED — %q explicitly un-ignored by layer: %s\n", rel, src)
	default:
		fmt.Printf("SCANNED — %q not matched by any layer\n", rel)
	}
	return nil
}

// gitleaksVersion mirrors cmdinstall's helper. Duplicated here to
// keep cmdignore's import surface small (we don't want cmdignore to
// pull cmdinstall transitively).
func gitleaksVersion() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "v8"
	}
	for _, dep := range info.Deps {
		if dep.Path == "github.com/zricethezav/gitleaks/v8" {
			return dep.Version
		}
	}
	return "v8"
}
