// Package scanner walks a directory and reports which files contain secrets.
//
// This is the engine behind `shhh scan`. It intentionally does not write
// anything to disk and does not use the session map — scan is a one-shot
// read-only operation. The session map is reserved for stateful flows (hook,
// proxy, MCP).
package scanner

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/musubi-sasu/shhh/internal/detector"
)

// FileResult is the scan output for a single file.
type FileResult struct {
	Path     string
	Findings []detector.Finding
}

// Scanner walks the filesystem and runs the detector on files that match the
// sensitive-file patterns from PRD §7.4.
type Scanner struct {
	det           *detector.Detector
	maxFileBytes  int64
	skipDirs      map[string]struct{}
	sensitiveName map[string]struct{}
	sensitiveExt  map[string]struct{}
	sensitiveGlob []string
}

// New returns a scanner with default patterns.
func New(det *detector.Detector) *Scanner {
	return &Scanner{
		det:          det,
		maxFileBytes: 10 * 1024 * 1024, // 10 MB
		skipDirs: map[string]struct{}{
			"node_modules": {}, ".git": {}, "vendor": {}, "__pycache__": {},
			".venv": {}, "dist": {}, "build": {}, "target": {}, ".next": {},
			".cache": {}, ".pytest_cache": {}, ".gradle": {},
		},
		sensitiveName: map[string]struct{}{
			".env": {}, ".env.local": {}, ".env.development": {},
			".env.staging": {}, ".env.production": {}, ".env.test": {},
			".env.example": {}, ".dev.vars": {}, ".npmrc": {}, ".pypirc": {},
			".netrc": {}, ".pgpass": {}, ".my.cnf": {},
			"credentials": {}, "credentials.json": {}, "credentials.yml": {},
			"service-account.json": {}, "keyfile.json": {},
		},
		sensitiveExt: map[string]struct{}{
			".pem": {}, ".key": {}, ".p12": {}, ".pfx": {}, ".jks": {},
		},
		sensitiveGlob: []string{
			"*secret*", "*credential*",
		},
	}
}

// IsSensitive reports whether a file path should be treated as sensitive for
// hook interception and scan prioritization.
func (s *Scanner) IsSensitive(path string) bool {
	base := filepath.Base(path)
	if _, ok := s.sensitiveName[base]; ok {
		return true
	}
	if strings.HasPrefix(base, ".env.") {
		return true
	}
	ext := strings.ToLower(filepath.Ext(base))
	if _, ok := s.sensitiveExt[ext]; ok {
		return true
	}
	lower := strings.ToLower(base)
	for _, g := range s.sensitiveGlob {
		ok, _ := filepath.Match(g, lower)
		if ok {
			return true
		}
	}
	return false
}

// Scan walks root and returns one FileResult per file with at least one
// finding. Files in skip directories, files larger than maxFileBytes, and
// binary files are ignored.
func (s *Scanner) Scan(root string) ([]FileResult, error) {
	var out []FileResult
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil // tolerate permission errors mid-walk
		}
		if d.IsDir() {
			if _, skip := s.skipDirs[d.Name()]; skip && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > s.maxFileBytes || info.Size() == 0 {
			return nil
		}
		// Only scan sensitive files or small text files. This keeps scan fast
		// on large repos: we do not scan every .js file for entropy.
		if !s.IsSensitive(path) && !likelyTextSmall(info.Size()) {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		if !isText(content) {
			return nil
		}
		findings := s.det.Detect(content)
		if len(findings) == 0 {
			return nil
		}
		out = append(out, FileResult{Path: path, Findings: findings})
		return nil
	})
	return out, err
}

// likelyTextSmall is a heuristic: small files are cheap to read and scan.
func likelyTextSmall(size int64) bool {
	return size <= 256*1024
}

// isText returns false if the first 512 bytes contain a NUL, which is a strong
// signal of a binary file.
func isText(b []byte) bool {
	n := len(b)
	if n > 512 {
		n = 512
	}
	for i := 0; i < n; i++ {
		if b[i] == 0 {
			return false
		}
	}
	return true
}
