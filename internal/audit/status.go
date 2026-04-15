package audit

import (
	"path/filepath"
	"strings"
)

// DecideStatus classifies a project into its Status tag.
//
// Rules (checked in order):
//  1. If the project dir no longer exists on disk → StatusArchived
//     (nothing to rotate, but past leaks remain leaked).
//  2. If shhh's installed-paths cover this project's absPath →
//     StatusProtected. (The project may still have findings in
//     fixtures or legacy leaks, but the hook is covering it.)
//  3. If the project has any findings (leaked or at-risk) →
//     StatusUnprotected.
//  4. Otherwise → StatusClean.
//
// shhhInstalledPaths is the list of settings.json file paths shhh
// has been installed into. For the global install, this contains
// exactly one path like "/Users/alice/.claude/settings.json" which
// protects ALL projects. For project-scoped installs, it contains
// paths like "/Users/alice/work/backend/.claude/settings.json"
// which protects only that subtree.
func DecideStatus(p *Project, shhhInstalledPaths []string, shhhScope string) Status {
	if !p.OnDisk {
		return StatusArchived
	}
	if shhhScope == "global" && len(shhhInstalledPaths) > 0 {
		return StatusProtected
	}
	if shhhScope == "project" && coversPath(shhhInstalledPaths, p.AbsPath) {
		return StatusProtected
	}
	if len(p.Leaked) > 0 || len(p.AtRisk) > 0 {
		return StatusUnprotected
	}
	return StatusClean
}

// coversPath returns whether any installed settings.json path
// protects the given project absolute path, under a project-scoped
// install. The settings.json lives at <project>/.claude/settings.json,
// so the grandparent directory is the project root and any project
// absPath under that root is covered.
func coversPath(installedPaths []string, projectAbsPath string) bool {
	cleanProj := filepath.Clean(projectAbsPath)
	for _, settingsPath := range installedPaths {
		projectRoot := filepath.Dir(filepath.Dir(filepath.Clean(settingsPath)))
		if cleanProj == projectRoot || hasPathPrefix(cleanProj, projectRoot) {
			return true
		}
	}
	return false
}

// hasPathPrefix reports whether path is equal to prefix or lives
// under it (in the filesystem-hierarchy sense, not the
// string-prefix sense).
func hasPathPrefix(path, prefix string) bool {
	if prefix == "" || prefix == "." {
		return false
	}
	sep := string(filepath.Separator)
	p := path
	pre := prefix
	if !strings.HasSuffix(pre, sep) {
		pre += sep
	}
	return strings.HasPrefix(p+sep, pre)
}

// ApplyStatus mutates p.Status in place. Returns the computed
// status for caller convenience.
func ApplyStatus(p *Project, shhhInstalledPaths []string, shhhScope string) Status {
	s := DecideStatus(p, shhhInstalledPaths, shhhScope)
	p.Status = s
	return s
}
