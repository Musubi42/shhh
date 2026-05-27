package cmdinstall

import (
	"strings"
)

// Cursor's hook config (~/.cursor/hooks.json or <repo>/.cursor/hooks.json)
// uses a flatter shape than Claude Code's settings.json. Each entry
// under hooks.<event>[] has command/matcher/timeout directly, with
// no inner `hooks: [{type, command}]` array. Confirmed from
// developers.cursor.com/docs/hooks and reflected in
// docs/dev/cursor-research-2026-05-27.md.
//
// Example we write:
//
//	{
//	  "version": 1,
//	  "hooks": {
//	    "preToolUse": [
//	      { "command": "/abs/path/shhh hook cursor", "matcher": "Read", "timeout": 10 },
//	      { "command": "/abs/path/shhh hook cursor", "matcher": "Shell", "timeout": 10 }
//	    ],
//	    "SessionEnd": [
//	      { "command": "/abs/path/shhh hook cursor", "timeout": 10 }
//	    ]
//	  }
//	}

// installCursorHooks merges shhh's hook entries into a Cursor-style
// hooks.json. Mirrors Install() in shape and diff reporting, but
// emits the flatter envelope Cursor expects.
func installCursorHooks(path, binary string) (string, error) {
	raw, settings, err := loadOrInit(path)
	if err != nil {
		return "", err
	}
	before := string(raw)

	// Ensure the version marker Cursor uses (the docs show
	// "version": 1 in every example). Leave any other top-level
	// keys alone.
	if _, ok := settings["version"]; !ok {
		settings["version"] = 1
	}

	cmd := quoteIfNeeded(binary) + hookSuffixFor("cursor")
	var added []managedHook
	for _, mh := range managedFor("cursor") {
		if ensureCursorHook(settings, mh, cmd) {
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

// ensureCursorHook adds (or no-ops) a flat-shape hook entry. matcher
// is set when non-empty (preToolUse entries); event-only entries
// (SessionEnd) omit the matcher field. Idempotent on command match.
func ensureCursorHook(settings map[string]any, mh managedHook, cmd string) bool {
	hooks, _ := settings["hooks"].(map[string]any)
	if hooks == nil {
		hooks = map[string]any{}
		settings["hooks"] = hooks
	}
	arr, _ := hooks[mh.event].([]any)

	// Check for an existing entry with the same matcher + command.
	for _, e := range arr {
		m, ok := e.(map[string]any)
		if !ok {
			continue
		}
		mm, _ := m["matcher"].(string)
		c, _ := m["command"].(string)
		if mm == mh.matcher && c == cmd {
			return false
		}
	}

	entry := map[string]any{
		"command": cmd,
		"timeout": 10,
	}
	if mh.matcher != "" {
		entry["matcher"] = mh.matcher
	}
	arr = append(arr, entry)
	hooks[mh.event] = arr
	return true
}

// uninstallCursorHooks removes every entry whose command ends in
// ` hook cursor`. Mirrors Uninstall() — leaves other top-level keys
// (including the version marker and unrelated hook entries) alone.
func uninstallCursorHooks(path string) (string, error) {
	raw, settings, err := loadOrInit(path)
	if err != nil {
		return "", err
	}
	before := string(raw)
	hookSuffix := hookSuffixFor("cursor")

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
		kept := make([]any, 0, len(entries))
		for _, e := range entries {
			m, ok := e.(map[string]any)
			if !ok {
				kept = append(kept, e)
				continue
			}
			cmd, _ := m["command"].(string)
			if strings.HasSuffix(cmd, hookSuffix) {
				matcher, _ := m["matcher"].(string)
				removed = append(removed, managedHook{event: event, matcher: matcher})
				removedCmd = cmd
				continue
			}
			kept = append(kept, e)
		}
		if len(kept) == 0 {
			delete(hooks, event)
		} else {
			hooks[event] = kept
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
