// Package cmdinstall implements `shhh install claude-code` and its
// uninstall counterpart. It edits ~/.claude/settings.json idempotently.
package cmdinstall

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// hookSuffix is the trailing substring that marks a hook entry as one of
// ours. We match on command-string suffix rather than a sentinel key on the
// JSON object because Claude Code may reject unknown keys, and the absolute
// path to the shhh binary varies per install.
const hookSuffix = " hook claude-code"

// managedHooks is the set of (event, matcher) pairs we install into
// settings.json. matcher == "" means no matcher field (used for SessionEnd,
// which does not match on tool name).
type managedHook struct {
	event   string
	matcher string
}

func managed() []managedHook {
	return []managedHook{
		{"PreToolUse", "Read"},
		{"PreToolUse", "Bash"},
		{"SessionEnd", ""},
	}
}

// Install merges shhh's hook entries into the settings.json at path.
// binary is the absolute path to the shhh executable that will run on each
// firing. Returns a unified-ish diff string describing the change (empty
// string if no change).
func Install(path, binary string) (string, error) {
	raw, settings, err := loadOrInit(path)
	if err != nil {
		return "", err
	}
	before := string(raw)

	cmd := quoteIfNeeded(binary) + hookSuffix
	var added []managedHook
	for _, mh := range managed() {
		if ensureHook(settings, mh, cmd) {
			added = append(added, mh)
		}
	}

	after, err := marshalIndent(settings)
	if err != nil {
		return "", err
	}
	if string(after) == before {
		return "", nil
	}
	if err := atomicWrite(path, after); err != nil {
		return "", err
	}
	return renderChange(changeReport{
		path:          path,
		direction:     directionAdd,
		hooks:         added,
		command:       cmd,
		preservedKeys: countNonHookKeys(settings),
	}), nil
}

// Uninstall removes all shhh hook entries from the settings.json at path.
// Surrounding configuration is left alone.
func Uninstall(path string) (string, error) {
	raw, settings, err := loadOrInit(path)
	if err != nil {
		return "", err
	}
	before := string(raw)

	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		return "", nil
	}

	var removed []managedHook
	var removedCmd string
	for event, v := range hooks {
		entries, ok := v.([]any)
		if !ok {
			continue
		}
		cleaned := make([]any, 0, len(entries))
		for _, e := range entries {
			m, ok := e.(map[string]any)
			if !ok {
				cleaned = append(cleaned, e)
				continue
			}
			matcher, _ := m["matcher"].(string)
			inner, _ := m["hooks"].([]any)
			newInner := make([]any, 0, len(inner))
			removedHere := false
			for _, h := range inner {
				hm, ok := h.(map[string]any)
				if !ok {
					newInner = append(newInner, h)
					continue
				}
				cmd, _ := hm["command"].(string)
				if strings.HasSuffix(cmd, hookSuffix) {
					removedHere = true
					removedCmd = cmd
					continue
				}
				newInner = append(newInner, h)
			}
			if removedHere {
				removed = append(removed, managedHook{event: event, matcher: matcher})
			}
			if len(newInner) == 0 {
				// Whole matcher entry is ours — drop it.
				continue
			}
			m["hooks"] = newInner
			cleaned = append(cleaned, m)
		}
		if len(cleaned) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = cleaned
		}
	}
	if len(hooks) == 0 {
		delete(settings, "hooks")
	}

	after, err := marshalIndent(settings)
	if err != nil {
		return "", err
	}
	if string(after) == before {
		return "", nil
	}
	if err := atomicWrite(path, after); err != nil {
		return "", err
	}
	return renderChange(changeReport{
		path:          path,
		direction:     directionRemove,
		hooks:         removed,
		command:       removedCmd,
		preservedKeys: countNonHookKeys(settings),
	}), nil
}

// loadOrInit reads path and returns its bytes plus a parsed map. If the
// file doesn't exist, it returns ([]byte("{}\n"), empty map, nil). If the
// file exists but is empty, same treatment.
func loadOrInit(path string) ([]byte, map[string]any, error) {
	buf, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return []byte("{}\n"), map[string]any{}, nil
		}
		return nil, nil, err
	}
	if len(strings.TrimSpace(string(buf))) == 0 {
		return []byte("{}\n"), map[string]any{}, nil
	}
	var m map[string]any
	if err := json.Unmarshal(buf, &m); err != nil {
		return nil, nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if m == nil {
		m = map[string]any{}
	}
	return buf, m, nil
}

// ensureHook makes settings.hooks[mh.event] contain a matcher entry for
// mh.matcher whose inner hooks list includes an entry with the given
// command. Creates missing levels. Idempotent: if an entry already exists
// it's left alone. Returns true when a new hook entry was inserted, false
// when the call was a no-op (already present).
func ensureHook(settings map[string]any, mh managedHook, cmd string) bool {
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		settings["hooks"] = hooks
	}
	arr, _ := hooks[mh.event].([]any)

	// Find a matcher entry that matches.
	var target map[string]any
	for _, e := range arr {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		mm, _ := m["matcher"].(string)
		if mm == mh.matcher {
			target = m
			break
		}
	}
	if target == nil {
		target = map[string]any{}
		if mh.matcher != "" {
			target["matcher"] = mh.matcher
		}
		target["hooks"] = []any{}
		arr = append(arr, target)
		hooks[mh.event] = arr
	}

	inner, _ := target["hooks"].([]any)
	for _, h := range inner {
		hm, ok := h.(map[string]any)
		if !ok {
			continue
		}
		if c, _ := hm["command"].(string); c == cmd {
			return false // already installed
		}
	}
	inner = append(inner, map[string]any{
		"type":    "command",
		"command": cmd,
		"timeout": 10,
	})
	target["hooks"] = inner
	return true
}

// countNonHookKeys returns the number of top-level settings keys other
// than "hooks". Used to tell the user how many of their existing
// settings were preserved unchanged across install/uninstall.
func countNonHookKeys(settings map[string]any) int {
	n := 0
	for k := range settings {
		if k != "hooks" {
			n++
		}
	}
	return n
}

func marshalIndent(v any) ([]byte, error) {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}

func atomicWrite(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".settings-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Chmod(name, 0o600); err != nil {
		os.Remove(name)
		return err
	}
	return os.Rename(name, path)
}

// quoteIfNeeded double-quotes a path that contains whitespace so it
// survives Claude Code's shell invocation. Paths without whitespace are
// left bare for readability in settings.json.
func quoteIfNeeded(p string) string {
	if strings.ContainsAny(p, " \t") {
		return strconv_quote(p)
	}
	return p
}

// strconv_quote is a tiny wrapper so we don't import strconv just for one
// call (and so tests can mock it later if needed).
func strconv_quote(s string) string {
	b := make([]byte, 0, len(s)+2)
	b = append(b, '"')
	for _, r := range s {
		if r == '"' || r == '\\' {
			b = append(b, '\\')
		}
		b = append(b, string(r)...)
	}
	b = append(b, '"')
	return string(b)
}

