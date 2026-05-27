package cmdhook

import (
	"strings"
	"testing"
)

// userPromptFixtureKey is a fake Stripe live key used only to drive
// the detector in tests. Real-shape so shhh-native catches it.
const userPromptFixtureKey = "sk_live_" + "4eC39HqLyjWDarjtT1zdp7dc"

func userPromptPayload(sessionID, prompt string) map[string]any {
	return map[string]any{
		"session_id":      sessionID,
		"hook_event_name": "UserPromptSubmit",
		"prompt":          prompt,
	}
}

func TestUserPrompt_CleanPromptPassesThrough(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	resp := runClaude(t, userPromptPayload("sess-clean", "please refactor the cache layer"))
	if _, has := resp["decision"]; has {
		t.Errorf("clean prompt should not produce a decision, got %v", resp)
	}
}

func TestUserPrompt_EmptyPromptIsNoop(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	resp := runClaude(t, userPromptPayload("sess-empty", ""))
	if _, has := resp["decision"]; has {
		t.Errorf("empty prompt should not produce a decision, got %v", resp)
	}
}

func TestUserPrompt_SecretBlocksWithReason(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	prompt := "here's the key i was using: " + userPromptFixtureKey
	resp := runClaude(t, userPromptPayload("sess-block", prompt))

	dec, _ := resp["decision"].(string)
	if dec != "block" {
		t.Fatalf("expected decision=block, got %v (full resp: %v)", dec, resp)
	}
	reason, _ := resp["reason"].(string)
	if !strings.Contains(reason, "STRIPE_LIVE_KEY") {
		t.Errorf("reason should name the detected placeholder, got %q", reason)
	}
	if !strings.Contains(reason, "!!") {
		t.Errorf("reason should mention the !! bypass, got %q", reason)
	}
	if !strings.Contains(reason, "/shhh-allow") {
		t.Errorf("reason should mention /shhh-allow, got %q", reason)
	}

	hso, _ := resp["hookSpecificOutput"].(map[string]any)
	if hso == nil || hso["hookEventName"] != "UserPromptSubmit" {
		t.Errorf("hookSpecificOutput.hookEventName missing or wrong: %v", hso)
	}
}

func TestUserPrompt_BypassPrefixPassesThrough(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	prompt := "!! here's the key: " + userPromptFixtureKey
	resp := runClaude(t, userPromptPayload("sess-bypass", prompt))
	if _, has := resp["decision"]; has {
		t.Errorf("!! prefix should bypass detection, got %v", resp)
	}
}

func TestUserPrompt_BypassRequiresSpaceAfterBangs(t *testing.T) {
	// "!!foo" (no space) is a JS double-bang operator, not a bypass.
	// A prompt that starts with that and contains a secret should
	// still block.
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	prompt := "!!key = " + userPromptFixtureKey
	resp := runClaude(t, userPromptPayload("sess-no-space", prompt))
	if dec, _ := resp["decision"].(string); dec != "block" {
		t.Errorf("expected block for !! without space, got %v", resp)
	}
}

func TestUserPrompt_AllowedSecretPasses(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	sid := "sess-allowed"
	if err := AddAllow(sid, "STRIPE_LIVE_KEY"); err != nil {
		t.Fatalf("AddAllow: %v", err)
	}
	prompt := "here it is: " + userPromptFixtureKey
	resp := runClaude(t, userPromptPayload(sid, prompt))
	if _, has := resp["decision"]; has {
		t.Errorf("allowed secret should pass, got %v", resp)
	}
}

func TestUserPrompt_AllowedNameDoesNotMatchOtherNames(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	sid := "sess-different-name"
	if err := AddAllow(sid, "ANTHROPIC_API_KEY"); err != nil {
		t.Fatalf("AddAllow: %v", err)
	}
	prompt := "stripe: " + userPromptFixtureKey
	resp := runClaude(t, userPromptPayload(sid, prompt))
	if dec, _ := resp["decision"].(string); dec != "block" {
		t.Errorf("allow for different name should not unblock, got %v", resp)
	}
}

func TestUserPrompt_MissingSessionIDFailsOpen(t *testing.T) {
	t.Setenv("SHHH_CACHE_DIR", t.TempDir())
	resp := runClaude(t, map[string]any{
		"hook_event_name": "UserPromptSubmit",
		"prompt":          "any prompt with " + userPromptFixtureKey,
	})
	if _, has := resp["decision"]; has {
		t.Errorf("missing session_id should fail open (no block), got %v", resp)
	}
}
