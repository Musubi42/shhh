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

// Scan walks root in two passes:
//
//  1. Collect strong-enough values from .env* files into a cross-reference
//     set. "Strong enough" means the detector's CheckEnvValue gate (length
//     ≥ 12, entropy ≥ 3.0, not on the denylist). This catches custom or
//     non-standard secrets that no pattern rule would match.
//
//  2. Walk every file and run the detection pipeline. For files that are
//     NOT the .env source themselves, also check for occurrences of the
//     cross-reference set — a match means the value from .env was copy-
//     pasted into code, which is the hardcoded-secret-leak case.
func (s *Scanner) Scan(root string) ([]FileResult, error) {
	crossRef, envFiles := s.collectEnvValues(root)

	var out []FileResult
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
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

		// Cross-reference pass: report .env values found anywhere. For the
		// .env source file itself, this surfaces custom secrets that pass
		// the strength gate but fall below the standalone entropy threshold
		// (the gate is intentionally more permissive — the signal "this is
		// a value in .env" adds confidence). For non-.env files, this
		// catches hardcoded copies. spanAlreadyCovered prevents duplicate
		// findings when a pattern rule already matched.
		_, isEnvSource := envFiles[path]
		text := string(content)
		for value := range crossRef {
			idx := strings.Index(text, value)
			if idx < 0 {
				continue
			}
			if spanAlreadyCovered(findings, idx, idx+len(value)) {
				continue
			}
			description := "Value from a .env file — possible hardcoded credential"
			label := "ENV_CROSSREF"
			if isEnvSource {
				description = "Custom secret (passed strength gate)"
				label = "ENV_CUSTOM_SECRET"
			}
			findings = append(findings, detector.Finding{
				Value:        value,
				Rule:         "env-crossref",
				Label:        label,
				PublicPrefix: "",
				Description:  description,
				Start:        idx,
				End:          idx + len(value),
			})
		}

		if len(findings) == 0 {
			return nil
		}
		out = append(out, FileResult{Path: path, Findings: findings})
		return nil
	})
	return out, err
}

// collectEnvValues walks the tree and parses .env-shaped files (any file
// whose basename is .env or begins with .env.), returning a set of values
// that pass the strength gate and a set of file paths that were processed
// as .env sources.
func (s *Scanner) collectEnvValues(root string) (map[string]struct{}, map[string]struct{}) {
	values := make(map[string]struct{})
	sources := make(map[string]struct{})
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if d.IsDir() {
			if _, skip := s.skipDirs[d.Name()]; skip && path != root {
				return filepath.SkipDir
			}
			return nil
		}
		base := filepath.Base(path)
		if base != ".env" && !strings.HasPrefix(base, ".env.") {
			return nil
		}
		info, err := d.Info()
		if err != nil || info.Size() > s.maxFileBytes {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		sources[path] = struct{}{}
		for _, line := range strings.Split(string(content), "\n") {
			line = strings.TrimSpace(line)
			if line == "" || strings.HasPrefix(line, "#") {
				continue
			}
			eq := strings.IndexByte(line, '=')
			if eq < 0 {
				continue
			}
			value := strings.TrimSpace(line[eq+1:])
			value = strings.Trim(value, `"'`)
			if value == "" {
				continue
			}
			if s.det.CheckEnvValue(value) {
				values[value] = struct{}{}
			}
		}
		return nil
	})
	return values, sources
}

// spanAlreadyCovered reports whether any existing finding overlaps [start,end).
func spanAlreadyCovered(findings []detector.Finding, start, end int) bool {
	for _, f := range findings {
		if start < f.End && f.Start < end {
			return true
		}
	}
	return false
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
