package eval

import (
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
)

// TestCompareSecrets_CrossRepresentation pins the base64 fallback in
// CompareSecrets that unblocks task 3. The tool must treat a raw value
// and its base64-encoded form as "same secret" when both live in the
// session map under distinct placeholders.
func TestCompareSecrets_CrossRepresentation(t *testing.T) {
	adapter := NewShhhAdapter()
	sess := adapter.NewSession()

	secret := "sk_live_AbC7f9Kd2nE8xR4tL1qM0pH5sW3vZ6yB"
	encodedRaw := base64.RawStdEncoding.EncodeToString([]byte(secret))
	encodedPadded := base64.StdEncoding.EncodeToString([]byte(secret))

	// Redact three different content blobs in the same session so the
	// session map carries placeholders for each.
	redactedRaw, _ := adapter.Redact(sess, []byte("API_KEY="+secret+"\n"))
	redactedEncRaw, _ := adapter.Redact(sess, []byte("api-key: "+encodedRaw+"\n"))
	redactedEncPadded, _ := adapter.Redact(sess, []byte("api-key: "+encodedPadded+"\n"))

	rawPlaceholder := extractAfter(string(redactedRaw), "API_KEY=")
	encRawPlaceholder := extractAfter(string(redactedEncRaw), "api-key: ")
	encPaddedPlaceholder := extractAfter(string(redactedEncPadded), "api-key: ")

	for name, p := range map[string]string{
		"raw":            rawPlaceholder,
		"raw-encoded":    encRawPlaceholder,
		"padded-encoded": encPaddedPlaceholder,
	} {
		if p == "" || p[0] != '[' {
			t.Fatalf("%s: expected a placeholder, got %q", name, p)
		}
	}

	tools := NewCompensatoryTools(adapter, sess)

	if !tools.CompareSecrets(rawPlaceholder, rawPlaceholder) {
		t.Error("raw == raw should be true")
	}
	if !tools.CompareSecrets(rawPlaceholder, encRawPlaceholder) {
		t.Error("raw == base64(raw, unpadded) should be true")
	}
	if !tools.CompareSecrets(encRawPlaceholder, rawPlaceholder) {
		t.Error("base64(raw) == raw should be true (order-independent)")
	}
	if !tools.CompareSecrets(rawPlaceholder, encPaddedPlaceholder) {
		t.Error("raw == base64(raw, padded) should be true")
	}

	// Unknown placeholder fails closed.
	if tools.CompareSecrets(rawPlaceholder, "[UNKNOWN:deadbeef]") {
		t.Error("unknown placeholder should compare false")
	}
}

// TestGrepHardcoded pins the task-5 compensatory tool against a real
// tempdir. The raw value must be locatable in every file that
// contains it (sorted) and the tool must fail closed on unknown
// placeholders.
func TestGrepHardcoded(t *testing.T) {
	adapter := NewShhhAdapter()
	sess := adapter.NewSession()

	secret := "sk_live_3aB9Kq2dPr7nT8vXwY5zC6hL"
	dir := t.TempDir()
	hitPaths := []string{
		filepath.Join(dir, "app.go"),
		filepath.Join(dir, "sub", "nested.py"),
	}
	decoyPath := filepath.Join(dir, "README.md")

	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hitPaths[0], []byte("const apiKey = \""+secret+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(hitPaths[1], []byte("API_KEY = \""+secret+"\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(decoyPath, []byte("# project\nno secrets here\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Redact a separate content blob so the session map has a
	// placeholder for the secret.
	redacted, _ := adapter.Redact(sess, []byte("KEY="+secret+"\n"))
	placeholder := extractAfter(string(redacted), "KEY=")
	if placeholder == "" || placeholder[0] != '[' {
		t.Fatalf("expected a placeholder, got %q", placeholder)
	}

	tools := NewCompensatoryTools(adapter, sess)

	hits := tools.GrepHardcoded(placeholder, dir)
	if len(hits) != 2 {
		t.Fatalf("expected 2 hits, got %d: %v", len(hits), hits)
	}
	// Results are sorted; first must be app.go, second nested.py.
	if hits[0] != hitPaths[0] || hits[1] != hitPaths[1] {
		t.Errorf("unexpected hit list: %v (want %v)", hits, hitPaths)
	}
	// Decoy must not appear.
	for _, h := range hits {
		if h == decoyPath {
			t.Errorf("decoy %q appeared in hits", decoyPath)
		}
	}

	// Unknown placeholder → nil (fail closed).
	if got := tools.GrepHardcoded("[UNKNOWN:deadbeef]", dir); got != nil {
		t.Errorf("unknown placeholder should return nil, got %v", got)
	}
}

// extractAfter returns everything on the first line after sep, trimmed
// of trailing padding (`=`) and newlines. The scan CLI's redacted
// output line-wraps at the end of the original content; we only need
// the placeholder span.
func extractAfter(content, sep string) string {
	start := -1
	for i := 0; i+len(sep) <= len(content); i++ {
		if content[i:i+len(sep)] == sep {
			start = i + len(sep)
			break
		}
	}
	if start < 0 {
		return ""
	}
	end := start
	for end < len(content) && content[end] != '\n' {
		end++
	}
	out := content[start:end]
	// Trim trailing padding that may remain when the redactor's token
	// regex stops before `=` in a padded base64 blob.
	for len(out) > 0 && out[len(out)-1] == '=' {
		out = out[:len(out)-1]
	}
	return out
}
