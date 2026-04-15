package audit

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ErrSchemaMismatch is returned (wrapped) by LoadLatestSnapshot
// when the snapshot on disk has a SchemaVersion that does not
// match CurrentSchemaVersion. Callers can inspect with errors.Is.
var ErrSchemaMismatch = errors.New("snapshot schema version mismatch")

const latestName = "latest.json"

// SaveSnapshot writes the given Result as a new snapshot and
// updates the latest.json symlink to point at it. Returns the
// absolute path of the snapshot file written.
//
// The filename is derived from result.AuditTime. Two snapshots
// in the same second get a numeric suffix (-2, -3, ...).
func SaveSnapshot(auditDir string, result *Result) (string, error) {
	if result == nil {
		return "", errors.New("SaveSnapshot: nil result")
	}
	if err := os.MkdirAll(auditDir, 0o700); err != nil {
		return "", fmt.Errorf("mkdir audit dir: %w", err)
	}

	base := result.AuditTime.UTC().Format("2006-01-02T15-04-05Z")
	name := base + ".json"
	full := filepath.Join(auditDir, name)
	for i := 2; ; i++ {
		if _, err := os.Lstat(full); os.IsNotExist(err) {
			break
		} else if err != nil {
			return "", fmt.Errorf("stat snapshot candidate: %w", err)
		}
		name = fmt.Sprintf("%s-%d.json", base, i)
		full = filepath.Join(auditDir, name)
	}

	data, err := json.MarshalIndent(result, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal snapshot: %w", err)
	}

	tmp, err := os.CreateTemp(auditDir, ".snap-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temp: %w", err)
	}
	tmpPath := tmp.Name()
	cleanupTmp := func() { _ = os.Remove(tmpPath) }
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		cleanupTmp()
		return "", fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		cleanupTmp()
		return "", fmt.Errorf("chmod temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		cleanupTmp()
		return "", fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpPath, full); err != nil {
		cleanupTmp()
		return "", fmt.Errorf("rename snapshot: %w", err)
	}

	latestPath := filepath.Join(auditDir, latestName)
	_ = os.Remove(latestPath)
	if err := os.Symlink(name, latestPath); err != nil {
		// Windows and some filesystems refuse symlinks. Fall back
		// to a regular-file copy so latest.json still points at
		// the current snapshot semantically.
		if errors.Is(err, os.ErrPermission) || strings.Contains(err.Error(), "not supported") || strings.Contains(err.Error(), "privilege") {
			if werr := writeFileAtomic(latestPath, data, 0o600); werr != nil {
				return "", fmt.Errorf("fallback write latest: %w", werr)
			}
		} else {
			return "", fmt.Errorf("symlink latest: %w", err)
		}
	}

	return full, nil
}

// writeFileAtomic writes data to path via a temp file and rename.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".latest-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		_ = os.Remove(tmpPath)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}
	return os.Rename(tmpPath, path)
}

// LoadLatestSnapshot reads <auditDir>/latest.json (or its symlink
// target) and returns the deserialized Result. Returns (nil, nil)
// if no snapshot exists yet — a missing latest is a normal
// first-run case, not an error.
//
// Schema version mismatch returns an error wrapping
// ErrSchemaMismatch, so callers know to ignore the old snapshot
// rather than silently misinterpret it.
func LoadLatestSnapshot(auditDir string) (*Result, error) {
	latestPath := filepath.Join(auditDir, latestName)
	data, err := os.ReadFile(latestPath)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read latest snapshot: %w", err)
	}
	var r Result
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, fmt.Errorf("decode snapshot: %w", err)
	}
	if r.SchemaVersion != CurrentSchemaVersion {
		return nil, fmt.Errorf("snapshot at %s has version %d, expected %d: %w",
			latestPath, r.SchemaVersion, CurrentSchemaVersion, ErrSchemaMismatch)
	}
	return &r, nil
}

// ListSnapshots returns all snapshot filenames in auditDir sorted
// oldest-first. Excludes "latest.json".
func ListSnapshots(auditDir string) ([]string, error) {
	entries, err := os.ReadDir(auditDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("read audit dir: %w", err)
	}
	var names []string
	for _, e := range entries {
		name := e.Name()
		if name == latestName {
			continue
		}
		if !strings.HasSuffix(name, ".json") {
			continue
		}
		if strings.HasPrefix(name, ".") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}
