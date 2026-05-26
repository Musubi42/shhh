package ignore

import (
	"fmt"
	"io"

	"github.com/Musubi42/shhh/internal/detector"
)

// BuildLayered assembles the standard 3-layer matcher: gitleaks
// defaults (read from the linked gitleaks module) → user-global
// `.shhhignore` → project `.shhhignore`. Either file may be
// missing; that's not an error. The gitleaks layer is best-effort:
// if the gitleaks detector fails to construct, the function logs
// to logOut (typically os.Stderr) and continues with the user
// layers only.
//
// projectRoot is the working directory of the scan (used to
// locate `<projectRoot>/.shhhignore`). Passing "" omits the
// project layer.
func BuildLayered(projectRoot string, logOut io.Writer) (*LayeredMatcher, error) {
	var layers []Matcher

	patterns, err := detector.GitleaksDefaultAllowlistPaths()
	if err != nil {
		if logOut != nil {
			fmt.Fprintf(logOut, "shhh: gitleaks defaults unavailable (%v); .shhhignore layer continues without them\n", err)
		}
	} else if len(patterns) > 0 {
		layers = append(layers, NewGitleaksLayer(patterns))
	}

	globalPath, projectPath, err := DefaultPaths(projectRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve ignore paths: %w", err)
	}
	if globalPath != "" {
		gl, gerr := NewFileLayer(globalPath)
		if gerr != nil && logOut != nil {
			fmt.Fprintf(logOut, "shhh: %s unreadable (%v); skipping\n", globalPath, gerr)
		} else if gl != nil {
			layers = append(layers, gl)
		}
	}
	if projectPath != "" {
		pl, perr := NewFileLayer(projectPath)
		if perr != nil && logOut != nil {
			fmt.Fprintf(logOut, "shhh: %s unreadable (%v); skipping\n", projectPath, perr)
		} else if pl != nil {
			layers = append(layers, pl)
		}
	}

	return NewLayered(layers...), nil
}
