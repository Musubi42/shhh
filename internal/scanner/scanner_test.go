package scanner

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/musubi-sasu/shhh/internal/detector"
)

func TestIsSensitive(t *testing.T) {
	s := New(detector.New())
	sensitive := []string{
		".env", ".env.local", ".env.production",
		"config/credentials.json", "keys/server.pem", "server.key",
		"my-secret-config.yaml", "credentials",
	}
	safe := []string{
		"index.js", "README.md", "package.json", "go.mod",
	}
	for _, p := range sensitive {
		if !s.IsSensitive(p) {
			t.Errorf("%q should be sensitive", p)
		}
	}
	for _, p := range safe {
		if s.IsSensitive(p) {
			t.Errorf("%q should not be sensitive", p)
		}
	}
}

func TestScanEnvCrossReference(t *testing.T) {
	// An internal custom token that no pattern rule would match but that
	// passes the CheckEnvValue strength gate.
	token := "Mk9zPwXr7AqN4bVtC2yL"

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(
		"INTERNAL_TOKEN="+token+"\nHOST=localhost\n",
	), 0o600); err != nil {
		t.Fatal(err)
	}
	// The same value appears hardcoded in a source file — this is what the
	// cross-reference should catch.
	if err := os.WriteFile(filepath.Join(dir, "config.js"), []byte(
		"const token = '"+token+"';\nmodule.exports = { token };\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	s := New(detector.New())
	results, err := s.Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	// Expect config.js to show up with a cross-reference finding.
	var configJSResult *FileResult
	for i := range results {
		if filepath.Base(results[i].Path) == "config.js" {
			configJSResult = &results[i]
			break
		}
	}
	if configJSResult == nil {
		t.Fatalf("config.js not flagged by cross-reference; results=%+v", results)
	}
	found := false
	for _, f := range configJSResult.Findings {
		if f.Rule == "env-crossref" && f.Value == token {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("env-crossref finding for %q not in config.js results: %+v", token, configJSResult.Findings)
	}
}

func TestScanEnvCrossReference_MultipleOccurrences(t *testing.T) {
	// A hardcoded .env value is copy-pasted into the source file twice.
	// The cross-reference pass must report both occurrences, not just the
	// first — the whole point of the feature is to surface every hardcoded
	// copy so the user can delete all of them.
	token := "Mk9zPwXr7AqN4bVtC2yL"

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(
		"INTERNAL_TOKEN="+token+"\n",
	), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config.js"), []byte(
		"const tokenA = '"+token+"';\nconst tokenB = '"+token+"';\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}

	s := New(detector.New())
	results, err := s.Scan(dir)
	if err != nil {
		t.Fatal(err)
	}

	var configJSResult *FileResult
	for i := range results {
		if filepath.Base(results[i].Path) == "config.js" {
			configJSResult = &results[i]
			break
		}
	}
	if configJSResult == nil {
		t.Fatalf("config.js not flagged; results=%+v", results)
	}
	hits := 0
	for _, f := range configJSResult.Findings {
		if f.Rule == "env-crossref" && f.Value == token {
			hits++
		}
	}
	if hits != 2 {
		t.Errorf("expected 2 cross-ref hits for %q in config.js, got %d (findings: %+v)", token, hits, configJSResult.Findings)
	}
	// And they must be reported in file order.
	prev := -1
	for _, f := range configJSResult.Findings {
		if f.Start < prev {
			t.Errorf("findings not sorted by offset: %+v", configJSResult.Findings)
			break
		}
		prev = f.Start
	}
}

func TestScanFixture(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(
		"STRIPE_KEY=sk_live_4eC39HqLyjWDarjtT1zdp7dcfakeKeyForTesting\n"+
			"AWS_KEY=AKIA3EXAMPLE7XYZABC4\n"+
			"HOST=localhost\n",
	), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(
		"# A project\nNothing sensitive here.\n",
	), 0o644); err != nil {
		t.Fatal(err)
	}
	// Skip-dir: this file should not be scanned.
	nm := filepath.Join(dir, "node_modules")
	if err := os.MkdirAll(nm, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nm, "leaked.env"), []byte(
		"FAKE=sk_live_ShouldNotBeScannedInsideNodeModules12345\n",
	), 0o600); err != nil {
		t.Fatal(err)
	}

	s := New(detector.New())
	results, err := s.Scan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 file with findings, got %d: %+v", len(results), results)
	}
	if filepath.Base(results[0].Path) != ".env" {
		t.Errorf("wrong file matched: %s", results[0].Path)
	}
	if len(results[0].Findings) < 2 {
		t.Errorf("expected ≥2 findings in .env, got %d", len(results[0].Findings))
	}
}
