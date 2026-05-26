// Package cmdhook implements `shhh hook claude-code`, the hook subcommand
// that Claude Code invokes on each tool use. Each firing is its own process,
// so any state that needs to survive across firings (most importantly the
// placeholder map, so the same secret maps to the same placeholder across
// multiple Read/Bash calls in one Claude session) lives in the sessionstore.
package cmdhook

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

	"github.com/Musubi42/shhh/internal/detector"
	"github.com/Musubi42/shhh/internal/redactor"
	"github.com/Musubi42/shhh/internal/session"
)

// sessionIDRe guards the session_id before we use it as a filename. Claude
// Code session_ids are UUID-shaped in practice, but the hook should never
// trust input enough to interpolate it into a path unchecked.
var sessionIDRe = regexp.MustCompile(`^[A-Za-z0-9._-]{1,128}$`)

// storeRoot returns the directory where per-session state lives. Exposed as
// a var so tests can redirect it.
var storeRoot = func() string {
	if v := os.Getenv("SHHH_CACHE_DIR"); v != "" {
		return v
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		cache = filepath.Join(os.TempDir(), "shhh-cache")
	}
	return filepath.Join(cache, "shhh")
}

// sessionDir is the per-session directory; redacted temp files live under
// <sessionDir>/redacted/.
func sessionDir(sessionID string) (string, error) {
	if !sessionIDRe.MatchString(sessionID) {
		return "", fmt.Errorf("invalid session_id")
	}
	return filepath.Join(storeRoot(), "sessions", sessionID), nil
}

func statePath(sessionID string) (string, error) {
	dir, err := sessionDir(sessionID)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "state.json"), nil
}

// diskState is the on-disk shape of the per-session store.
type diskState struct {
	Salt    string          `json:"salt"`
	Entries []session.Entry `json:"entries"`
}

// LoadRedactor returns a Redactor whose session map is hydrated from the
// on-disk per-session state (or a fresh map if nothing on disk yet).
//
// The returned releaser *must* be called to persist changes back to disk.
// Callers that bail on error before mutating the map may skip it — state
// will just be empty next firing, which is a correctness-preserving loss.
func LoadRedactor(sessionID string) (*redactor.Redactor, func() error, error) {
	path, err := statePath(sessionID)
	if err != nil {
		return nil, nil, err
	}

	var sess *session.Map
	buf, err := os.ReadFile(path)
	switch {
	case err == nil:
		var st diskState
		if jerr := json.Unmarshal(buf, &st); jerr != nil {
			// Corrupt store: start fresh rather than failing the hook.
			// Redaction is best-effort-non-blocking (roadmap milestone 1).
			sess = session.New()
		} else {
			loaded, lerr := session.FromSnapshot(st.Salt, st.Entries)
			if lerr != nil {
				sess = session.New()
			} else {
				sess = loaded
			}
		}
	case os.IsNotExist(err):
		sess = session.New()
	default:
		return nil, nil, fmt.Errorf("read session store: %w", err)
	}

	// Honour SHHH_DETECTOR=shhh-native|gitleaks. Hook is the
	// primary integration surface — every Read/Bash call from the
	// agent passes through this redactor. The factory falls back
	// to shhh-native if SHHH_DETECTOR is unset/unknown or if
	// gitleaks fails to initialise. Phase 2 will replace this
	// env-var path with a Config-driven multi-engine selector.
	r := redactor.New(detector.NewFromEnv(), sess)

	save := func() error {
		salt, entries := sess.Snapshot()
		out, merr := json.Marshal(diskState{Salt: salt, Entries: entries})
		if merr != nil {
			return merr
		}
		if mkerr := os.MkdirAll(filepath.Dir(path), 0o700); mkerr != nil {
			return mkerr
		}
		// Atomic write: tmp + rename.
		tmp, terr := os.CreateTemp(filepath.Dir(path), ".state-*.tmp")
		if terr != nil {
			return terr
		}
		tmpName := tmp.Name()
		if _, werr := tmp.Write(out); werr != nil {
			tmp.Close()
			os.Remove(tmpName)
			return werr
		}
		if cerr := tmp.Close(); cerr != nil {
			os.Remove(tmpName)
			return cerr
		}
		if cerr := os.Chmod(tmpName, 0o600); cerr != nil {
			os.Remove(tmpName)
			return cerr
		}
		return os.Rename(tmpName, path)
	}

	return r, save, nil
}

// RedactedPath returns the path where a redacted copy of a source file
// lives in this session's cache. Layout:
//
//	<cache>/shhh/sessions/<sid>/redacted/<sourceHash>/<basename>
//
// The basename is preserved as the leaf so Claude Code's transcript shows
// a recognizable filename (".env", not "abc123.env") even though the
// enclosing directory is a per-source hash that makes collisions
// impossible across distinct source paths.
func RedactedPath(sessionID, absSourcePath string) (string, error) {
	dir, err := sessionDir(sessionID)
	if err != nil {
		return "", err
	}
	bucket := hashName(absSourcePath)[:8]
	base := filepath.Base(absSourcePath)
	if base == "" || base == "/" || base == "." {
		base = "file"
	}
	return filepath.Join(dir, "redacted", bucket, base), nil
}

// WipeSession removes all on-disk state for a session. Called on SessionEnd.
func WipeSession(sessionID string) error {
	dir, err := sessionDir(sessionID)
	if err != nil {
		return err
	}
	return os.RemoveAll(dir)
}
