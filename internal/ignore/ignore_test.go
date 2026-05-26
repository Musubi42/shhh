package ignore

import (
	"os"
	"path/filepath"
	"regexp"
	"testing"
)

func TestGitleaksLayerMatchesRegex(t *testing.T) {
	pats := []*regexp.Regexp{
		regexp.MustCompile(`^go\.sum$`),
		regexp.MustCompile(`/node_modules/`),
	}
	g := NewGitleaksLayer(pats)
	if g.Decision("go.sum") != Ignored {
		t.Errorf("go.sum should be Ignored by gitleaks layer")
	}
	if g.Decision("src/foo/node_modules/x.js") != Ignored {
		t.Errorf("node_modules path should be Ignored")
	}
	if g.Decision("src/foo.go") != Neutral {
		t.Errorf("source file should be Neutral")
	}
}

func TestFileLayerIgnoresAndUnIgnores(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".shhhignore")
	body := "# comment\n*.tmp\nbuild/\n!build/keep.txt\n"
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := NewFileLayer(path)
	if err != nil {
		t.Fatal(err)
	}
	if d := f.Decision("scratch.tmp"); d != Ignored {
		t.Errorf("scratch.tmp = %v, want Ignored", d)
	}
	if d := f.Decision("build/output.bin"); d != Ignored {
		t.Errorf("build/output.bin = %v, want Ignored", d)
	}
	if d := f.Decision("build/keep.txt"); d != Included {
		t.Errorf("build/keep.txt = %v, want Included (negated)", d)
	}
	if d := f.Decision("src/main.go"); d != Neutral {
		t.Errorf("src/main.go = %v, want Neutral", d)
	}
}

func TestLayeredCrossLayerNegation(t *testing.T) {
	// gitleaks-style layer ignores go.sum unconditionally.
	gl := NewGitleaksLayer([]*regexp.Regexp{regexp.MustCompile(`^go\.sum$`)})
	// Project file un-ignores it.
	project := NewInlineFileLayer("project/.shhhignore", []string{"!go.sum"})

	lm := NewLayered(gl, project)
	if lm.IsIgnored("go.sum") {
		t.Errorf("project !go.sum must override gitleaks ignore")
	}
	src, d := lm.Explain("go.sum")
	if d != Included {
		t.Errorf("Explain decision = %v, want Included", d)
	}
	if src != "project/.shhhignore" {
		t.Errorf("Explain source = %q, want project label", src)
	}
}

func TestLayeredOrderingLastWins(t *testing.T) {
	// Two file layers where the latter overrides the former.
	user := NewInlineFileLayer("user", []string{"*.log"})
	project := NewInlineFileLayer("project", []string{"!debug.log"})
	lm := NewLayered(user, project)
	if lm.IsIgnored("debug.log") {
		t.Errorf("debug.log should be un-ignored by project layer")
	}
	if !lm.IsIgnored("other.log") {
		t.Errorf("other.log should still be ignored by user layer")
	}
}

func TestFileLayerMissingFileReturnsEmpty(t *testing.T) {
	f, err := NewFileLayer("/nonexistent/path/.shhhignore")
	if err != nil {
		t.Fatalf("missing file should not be an error, got %v", err)
	}
	if d := f.Decision("anything"); d != Neutral {
		t.Errorf("empty layer must always be Neutral, got %v", d)
	}
}

func TestDefaultPathsRespectsEnvOverride(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("SHHH_CONFIG_DIR", tmp)
	g, p, err := DefaultPaths("/some/project")
	if err != nil {
		t.Fatal(err)
	}
	wantG := filepath.Join(tmp, ".shhhignore")
	if g != wantG {
		t.Errorf("global = %q, want %q", g, wantG)
	}
	if p != "/some/project/.shhhignore" {
		t.Errorf("project = %q, want %q", p, "/some/project/.shhhignore")
	}
}
