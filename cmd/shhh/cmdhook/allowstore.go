package cmdhook

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// allowTTL is the maximum age of a session allow file before it is
// considered expired. Matches the user-facing message ("max 24h or
// until session ends") shown by the /shhh-allow slash command.
const allowTTL = 24 * time.Hour

// allowFileName is the per-session allow store filename. Lives at
// <sessionDir>/allow.json alongside state.json and the redacted/
// subdirectory.
const allowFileName = "allow.json"

// diskAllow is the on-disk shape of the per-session allow store.
type diskAllow struct {
	// Allowed is the list of placeholder names (e.g. "ANTHROPIC_API_KEY")
	// whose findings should pass through redaction for this session.
	Allowed []string `json:"allowed"`
}

func allowPath(sessionID string) (string, error) {
	dir, err := sessionDir(sessionID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, allowFileName), nil
}

// LoadAllow returns the set of placeholder names allowed for the given
// session. Returns an empty set if no allow file exists, if the file is
// stale (>allowTTL), or if it is corrupt. A corrupt or stale file is
// removed as a side effect.
func LoadAllow(sessionID string) (map[string]struct{}, error) {
	path, err := allowPath(sessionID)
	if err != nil {
		return nil, err
	}

	info, err := os.Stat(path)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return map[string]struct{}{}, nil
	case err != nil:
		return nil, fmt.Errorf("stat allow file: %w", err)
	}

	if time.Since(info.ModTime()) > allowTTL {
		_ = os.Remove(path)
		return map[string]struct{}{}, nil
	}

	buf, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read allow file: %w", err)
	}

	var d diskAllow
	if err := json.Unmarshal(buf, &d); err != nil {
		_ = os.Remove(path)
		return map[string]struct{}{}, nil
	}

	out := make(map[string]struct{}, len(d.Allowed))
	for _, n := range d.Allowed {
		out[n] = struct{}{}
	}
	return out, nil
}

// AddAllow appends a placeholder name to the session's allow file,
// creating the file (and the session directory) if needed. Idempotent:
// adding the same name twice is a no-op.
func AddAllow(sessionID, name string) error {
	if name == "" {
		return errors.New("empty allow name")
	}
	current, err := LoadAllow(sessionID)
	if err != nil {
		return err
	}
	if _, ok := current[name]; ok {
		return nil
	}
	current[name] = struct{}{}

	names := make([]string, 0, len(current))
	for n := range current {
		names = append(names, n)
	}

	path, err := allowPath(sessionID)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}

	out, err := json.Marshal(diskAllow{Allowed: names})
	if err != nil {
		return err
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), ".allow-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(out); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Chmod(tmpName, 0o600); err != nil {
		os.Remove(tmpName)
		return err
	}
	return os.Rename(tmpName, path)
}

// GCStaleSessions removes session directories whose mtime is older than
// allowTTL. Best-effort: errors are returned but the caller should not
// fail the hook on them (cleanup is opportunistic).
//
// Called from hook entrypoints so that long-running users do not
// accumulate session state from sessions that never emitted a clean
// SessionEnd.
func GCStaleSessions() error {
	root := filepath.Join(storeRoot(), "sessions")
	entries, err := os.ReadDir(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	cutoff := time.Now().Add(-allowTTL)
	var firstErr error
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		info, ierr := e.Info()
		if ierr != nil {
			if firstErr == nil {
				firstErr = ierr
			}
			continue
		}
		if info.ModTime().Before(cutoff) {
			if rerr := os.RemoveAll(filepath.Join(root, e.Name())); rerr != nil && firstErr == nil {
				firstErr = rerr
			}
		}
	}
	return firstErr
}
