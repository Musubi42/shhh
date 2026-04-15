package cmdhook

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/musubi-sasu/shhh/internal/detector"
	"github.com/musubi-sasu/shhh/internal/redactor"
)

// readToolInput is the subset of Claude Code's Read tool_input we preserve.
// We pass unknown fields through verbatim so Claude Code can keep any it
// adds later without the hook silently dropping them.
type readToolInput struct {
	FilePath string `json:"file_path"`
	// Offset and Limit are optional; kept as json.Number so we don't
	// guess an integer type the caller didn't use.
	Offset json.RawMessage `json:"offset,omitempty"`
	Limit  json.RawMessage `json:"limit,omitempty"`
}

// maxRedactFileSize caps the bytes we'll read from a file before giving up
// on redaction. Matches a reasonable ceiling for "source file or config
// file" — binaries and multi-MB logs fall through unmodified. Hook is
// best-effort-non-blocking: anything over the cap emits `{}`.
const maxRedactFileSize = 4 << 20 // 4 MiB

// isEnvLikePath reports whether a file path looks like a dotenv-style
// secret store. .env files are known to contain custom-named high-value
// tokens that the generic entropy detector is tuned to skip, so the hook
// applies the looser env-aware redaction pass to them.
func isEnvLikePath(p string) bool {
	base := filepath.Base(p)
	if base == ".env" || base == ".envrc" {
		return true
	}
	if strings.HasPrefix(base, ".env.") {
		return true
	}
	// `env` as a sibling filename is too common (Python virtualenv, etc.)
	// to treat as env-like. Stick to dotfile conventions.
	return false
}

func handleRead(stdout io.Writer, in *hookInput) {
	var ti readToolInput
	if err := json.Unmarshal(in.ToolInput, &ti); err != nil || ti.FilePath == "" {
		writeEmpty(stdout)
		return
	}

	// Absolute path — needed because we key the cache by abspath and we
	// want a deterministic name per file regardless of Claude Code's cwd.
	abs, err := filepath.Abs(ti.FilePath)
	if err != nil {
		writeEmpty(stdout)
		return
	}

	f, err := os.Open(abs)
	if err != nil {
		writeEmpty(stdout)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || !st.Mode().IsRegular() || st.Size() > maxRedactFileSize {
		writeEmpty(stdout)
		return
	}

	content, err := io.ReadAll(f)
	if err != nil {
		writeEmpty(stdout)
		return
	}

	// Skip binary files — NUL byte heuristic. The detector would return
	// zero findings on binary anyway, but reading large binaries into
	// memory is wasted work.
	if bytes.IndexByte(content, 0) >= 0 {
		writeEmpty(stdout)
		return
	}

	red, save, err := LoadRedactor(in.SessionID)
	if err != nil {
		writeEmpty(stdout)
		return
	}

	var (
		out      []byte
		findings []detector.Finding
	)
	envMode := isEnvLikePath(abs)
	if envMode {
		out, findings = red.RedactEnvFile(content)
	} else {
		out, findings = red.Redact(content)
	}
	if len(findings) == 0 {
		writeEmpty(stdout)
		return
	}

	// Write redacted copy to the per-session cache, then point Claude at
	// it via updatedInput.file_path.
	destPath, err := RedactedPath(in.SessionID, abs)
	if err != nil {
		writeEmpty(stdout)
		return
	}
	if err := os.MkdirAll(filepath.Dir(destPath), 0o700); err != nil {
		writeEmpty(stdout)
		return
	}
	if err := os.WriteFile(destPath, out, 0o600); err != nil {
		writeEmpty(stdout)
		return
	}

	if err := save(); err != nil {
		// Placeholder map didn't persist — the current firing's
		// redaction still goes through, but future firings won't
		// share the same map. Still better than blocking.
	}

	updated := map[string]any{"file_path": destPath}
	if len(ti.Offset) > 0 {
		updated["offset"] = json.RawMessage(ti.Offset)
	}
	if len(ti.Limit) > 0 {
		updated["limit"] = json.RawMessage(ti.Limit)
	}
	updatedJSON, err := json.Marshal(updated)
	if err != nil {
		writeEmpty(stdout)
		return
	}

	writeJSON(stdout, hookResponse{
		HookSpecificOutput: &hookSpecificOutput{
			HookEventName:      "PreToolUse",
			PermissionDecision: "allow",
			UpdatedInput:       updatedJSON,
			AdditionalContext:  narrateRedactions(ti.FilePath, findings, content, red, envMode),
		},
	})
}

// narrateRedactions builds the natural-language note that rides along with
// the rewritten Read call. The goal is threefold:
//
//  1. Tell Claude the real source path (updatedInput.file_path points at a
//     cache location, so without this note Claude wouldn't know which file
//     it actually read).
//  2. Nudge Claude to tell the USER that shhh protected their secrets,
//     using natural language rather than the raw placeholder tokens.
//  3. Warn Claude that the Edit tool will fail on this file for the rest
//     of the session, and point it at Bash as the working alternative.
//     See docs/design/read-edit-tracking.md for why this is necessary —
//     Claude Code's Read-ledger records the cache path, not the original.
//
// The note is intentionally brief and framed as instructions to Claude,
// not as content Claude should quote back verbatim.
//
// envMode suppresses the per-finding line listing: .env files are
// line-preserving under redaction, so Claude can already infer the
// variable→placeholder mapping directly from the redacted content, and
// duplicating it in the narration would just inflate context.
func narrateRedactions(originalPath string, findings []detector.Finding, content []byte, red *redactor.Redactor, envMode bool) string {
	if len(findings) == 0 {
		return ""
	}
	labels := uniqueLabels(findings)

	var b strings.Builder
	b.WriteString("shhh (local secret-redaction tool) intercepted this Read of ")
	b.WriteString(originalPath)
	b.WriteString(" and replaced ")
	if len(findings) == 1 {
		b.WriteString("1 secret")
	} else {
		fmt.Fprintf(&b, "%d secrets", len(findings))
	}
	b.WriteString(" with placeholders before you saw the file. The real secret values never reached you and never left the user's machine.\n\n")
	b.WriteString("Redacted secret types: ")
	b.WriteString(strings.Join(labels, ", "))
	b.WriteString(".\n\n")

	if !envMode {
		b.WriteString("shhh redacted these secrets:\n")
		for _, f := range findings {
			line := lineNumberAt(content, f.Start)
			placeholder := red.PlaceholderForFinding(f)
			fmt.Fprintf(&b, "  - %s at line %d (placeholder: %s)\n", f.Label, line, placeholder)
		}
		b.WriteString("\n")
	}

	b.WriteString("IMPORTANT — how to modify this file:\n")
	b.WriteString("The Edit and Write tools will fail on ")
	b.WriteString(originalPath)
	b.WriteString(" with \"File has not been read yet\" for the rest of this session. This is a known limitation of Claude Code's hook API, not an error in your usage. To modify this file, use the Bash tool instead (sed -i, tee, printf >>, python -c, etc.) — do not retry Edit or Read. The Bash tool output is also redacted by shhh, so this is safe.\n\n")
	b.WriteString("When you respond to the user, briefly tell them that shhh protected their secrets during this Read (one short sentence is enough — don't lecture). Do not try to un-redact placeholders; they are opaque tokens by design.")
	return b.String()
}

// lineNumberAt returns the 1-based line number of byte offset off in content.
// If off is past the end, it returns the last line (clamped). Counts LF only,
// which is correct for UTF-8 since CR and LF are both single bytes.
func lineNumberAt(content []byte, off int) int {
	if off < 0 {
		off = 0
	}
	if off > len(content) {
		off = len(content)
	}
	return 1 + bytes.Count(content[:off], []byte{'\n'})
}

// uniqueLabels returns the distinct Finding.Label values in order of first
// appearance. Used to build a stable, non-repeating list for the narration.
func uniqueLabels(findings []detector.Finding) []string {
	seen := make(map[string]struct{}, len(findings))
	out := make([]string, 0, len(findings))
	for _, f := range findings {
		if _, ok := seen[f.Label]; ok {
			continue
		}
		seen[f.Label] = struct{}{}
		out = append(out, f.Label)
	}
	return out
}
