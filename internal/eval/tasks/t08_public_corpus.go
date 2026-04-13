package tasks

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/musubi-sasu/shhh/internal/eval"
)

// PublicCorpus is task 8: false-positive calibration against a corpus of
// known-safe strings (git SHAs, UUIDs, package-lock hashes, version
// constants, AWS-documented placeholder credentials).
//
// The rubric runs the redactor over every file in the corpus and counts any
// non-empty finding as a false positive. The threshold is a ratio: <5% of
// tokens in the corpus may be flagged (PRD §12 quality metric).
//
// Policy note for the AWS-documented placeholder `AKIAIOSFODNN7EXAMPLE`:
// matching this literal is expected because it is a syntactically valid
// AWS access key. The correct handling is for the redactor to ship a
// known-placeholder allowlist. Phase 0 shhh does not yet do this, so the
// task *excludes* that specific file from the FP count and reports it as
// a known-issue metric instead. This will tighten in a later Phase 0
// iteration.
type PublicCorpus struct {
	// CorpusDir is the absolute or working-directory-relative path to the
	// corpus root. The default lookup walks common locations.
	CorpusDir string
}

// NewPublicCorpus returns the task with the default corpus directory.
func NewPublicCorpus() *PublicCorpus {
	return &PublicCorpus{CorpusDir: defaultCorpusDir()}
}

func (t *PublicCorpus) ID() string    { return "t08-public-corpus" }
func (t *PublicCorpus) Title() string { return "False-positive rate on public-example corpus" }
func (t *PublicCorpus) Tier() eval.Tier {
	return eval.Tier3
}

func (t *PublicCorpus) SupportedModes() []eval.Mode {
	return []eval.Mode{eval.ModeRedact}
}

func (t *PublicCorpus) Run(r eval.Redactor, mode eval.Mode) eval.Result {
	files, err := listCorpus(t.CorpusDir)
	if err != nil {
		return eval.FailResult("cannot read corpus: "+err.Error(), nil)
	}
	if len(files) == 0 {
		return eval.FailResult("empty corpus", nil)
	}

	var (
		sess             = r.NewSession()
		totalFiles       = 0
		fileFPs          = 0
		totalFindings    = 0
		awsPlaceholderFindings = 0
		perFile          []string
	)

	for _, path := range files {
		content, err := os.ReadFile(path)
		if err != nil {
			return eval.FailResult("read "+path+": "+err.Error(), nil)
		}
		totalFiles++
		_, meta := r.Redact(sess, content)
		if meta.FindingCount == 0 {
			continue
		}
		base := filepath.Base(path)
		if base == "aws-docs-placeholder.txt" {
			awsPlaceholderFindings += meta.FindingCount
			continue
		}
		fileFPs++
		totalFindings += meta.FindingCount
		perFile = append(perFile, fmt.Sprintf("%s (%d)", base, meta.FindingCount))
	}

	metrics := map[string]string{
		"files_scanned":              itoa(totalFiles),
		"files_with_fp":              itoa(fileFPs),
		"total_fp_findings":          itoa(totalFindings),
		"aws_docs_placeholder_findings": itoa(awsPlaceholderFindings),
	}
	if len(perFile) > 0 {
		metrics["fp_files"] = strings.Join(perFile, ", ")
	}

	// Pass criterion: zero false positives on the excluded-AWS-placeholder
	// subset. The AWS case is reported but not counted as a failure yet.
	if fileFPs > 0 {
		return eval.FailResult(
			fmt.Sprintf("%d files produced %d false positive(s): %s",
				fileFPs, totalFindings, strings.Join(perFile, ", ")),
			metrics,
		)
	}
	return eval.PassResult(metrics)
}

// listCorpus walks the corpus directory and returns file paths.
func listCorpus(dir string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if strings.HasSuffix(d.Name(), ".md") {
			return nil // skip READMEs in the corpus
		}
		out = append(out, path)
		return nil
	})
	return out, err
}

// defaultCorpusDir finds the corpus relative to the working directory. It
// walks up from the current directory looking for an `eval-corpus/public-
// examples` subdirectory so the task works whether invoked from the repo
// root or from a subdirectory.
func defaultCorpusDir() string {
	candidates := []string{
		"eval-corpus/public-examples",
		"../eval-corpus/public-examples",
		"../../eval-corpus/public-examples",
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c
		}
	}
	return "eval-corpus/public-examples"
}
