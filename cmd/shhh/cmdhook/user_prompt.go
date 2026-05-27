package cmdhook

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/Musubi42/shhh/cmd/shhh/cmdinstall"
	"github.com/Musubi42/shhh/internal/detector"
)

// bypassPrefix is the one-shot inline bypass marker. A prompt that
// starts with this string (literally "!! ", double-bang + space)
// skips shhh detection entirely. The space is required so that
// JavaScript snippets like `!!foo` pasted at the start of a prompt
// do not accidentally trigger the bypass.
//
// We do not strip the prefix from the prompt before it reaches the
// LLM: Claude Code's UserPromptSubmit hook API does not let a hook
// rewrite the prompt text (verified against the official docs).
// The "!! " characters arrive at the model as-is; they are short
// enough to be ignored as noise.
const bypassPrefix = "!! "

// userPromptOutput is the JSON envelope Claude Code expects from a
// UserPromptSubmit hook. `decision` and `reason` live at the top
// level; `hookEventName` lives inside `hookSpecificOutput`. See:
// https://code.claude.com/docs/en/hooks
type userPromptOutput struct {
	Decision           string                       `json:"decision,omitempty"`
	Reason             string                       `json:"reason,omitempty"`
	HookSpecificOutput *userPromptHookSpecific      `json:"hookSpecificOutput,omitempty"`
}

type userPromptHookSpecific struct {
	HookEventName string `json:"hookEventName"`
}

func handleUserPromptSubmit(stdout io.Writer, in *hookInput) {
	// Opportunistic GC: every hook firing is a chance to clean up
	// sessions that never emitted SessionEnd. Best-effort; ignore
	// errors so the user-facing flow is never blocked by cleanup.
	_ = GCStaleSessions()

	prompt := in.Prompt
	if prompt == "" {
		writeEmpty(stdout)
		return
	}

	// Inline bypass: "!! " prefix lets the prompt through untouched.
	if strings.HasPrefix(prompt, bypassPrefix) {
		writeEmpty(stdout)
		return
	}

	sid := in.effectiveSessionID()
	if sid == "" {
		// No session context; we cannot consult the allow list.
		// Fail open (let the prompt through) — staying out of the
		// user's way is more important than blocking on a missing
		// session_id, which would typically indicate an upstream
		// payload-shape regression rather than a real prompt.
		writeEmpty(stdout)
		return
	}

	cfg, _ := cmdinstall.LoadConfig()
	det := detector.NewFromConfig(cfg.EffectiveEngines())
	findings := det.Detect([]byte(prompt))

	if len(findings) == 0 {
		writeEmpty(stdout)
		return
	}

	allowed, err := LoadAllow(sid)
	if err != nil {
		// Treat a broken allow store as an empty allow list rather
		// than failing the hook. Erring on the side of "block when
		// in doubt" is consistent with the secret-never-leaves
		// promise.
		allowed = map[string]struct{}{}
	}

	blocking := filterAllowed(findings, allowed)
	if len(blocking) == 0 {
		writeEmpty(stdout)
		return
	}

	writeJSON(stdout, userPromptOutput{
		Decision: "block",
		Reason:   formatBlockReason(blocking),
		HookSpecificOutput: &userPromptHookSpecific{
			HookEventName: "UserPromptSubmit",
		},
	})
}

// filterAllowed drops findings whose Label appears in the allowed
// set. Returns the remaining "blocking" findings.
func filterAllowed(findings []detector.Finding, allowed map[string]struct{}) []detector.Finding {
	if len(allowed) == 0 {
		return findings
	}
	out := findings[:0:0]
	for _, f := range findings {
		if _, ok := allowed[f.Label]; ok {
			continue
		}
		out = append(out, f)
	}
	return out
}

// formatBlockReason builds the message shown to the user when shhh
// blocks a prompt. It names the placeholders detected and points to
// the two bypass paths.
func formatBlockReason(findings []detector.Finding) string {
	names := distinctLabels(findings)

	var b strings.Builder
	if len(names) == 1 {
		fmt.Fprintf(&b, "shhh detected %s in your prompt and blocked it before it reached the model.\n", names[0])
	} else {
		b.WriteString("shhh detected the following in your prompt and blocked it before it reached the model:\n")
		for _, n := range names {
			fmt.Fprintf(&b, "  - %s\n", n)
		}
	}
	b.WriteString("\n")
	b.WriteString("To send this prompt anyway, either:\n")
	b.WriteString("  • Prefix it with \"!! \" (one-shot bypass), or\n")
	if len(names) == 1 {
		fmt.Fprintf(&b, "  • Type /shhh-allow %s (valid for this session, max 24h), then press ↑ to recall this prompt and re-submit.\n", names[0])
	} else {
		fmt.Fprintf(&b, "  • Type /shhh-allow <NAME> for each name above (valid for this session, max 24h), then press ↑ to recall this prompt and re-submit.\n")
	}
	return b.String()
}

func distinctLabels(findings []detector.Finding) []string {
	seen := map[string]struct{}{}
	for _, f := range findings {
		seen[f.Label] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for n := range seen {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}
