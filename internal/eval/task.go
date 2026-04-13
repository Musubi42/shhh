package eval

// Mode is the combination of redaction layers applied to a task run. The four
// modes are specified in PRD §10 Phase 0:
//
//  1. no-redaction — baseline, agent sees raw secrets.
//  2. redact — content is redacted but no rehydration or compensatory tools.
//  3. redact-rehydrate — placeholders are rehydrated in tool_use arguments.
//  4. redact-rehydrate-compensatory — same plus compensatory MCP tools.
//
// Some tasks ignore the mode entirely (e.g. task 7 placeholder entropy is a
// direct crypto measurement). Tasks declare which modes they run via
// SupportedModes.
type Mode string

const (
	ModeNoRedaction           Mode = "no-redaction"
	ModeRedact                Mode = "redact"
	ModeRedactRehydrate       Mode = "redact-rehydrate"
	ModeRedactRehydrateCompen Mode = "redact-rehydrate-compensatory"
)

// AllModes returns the four standard modes in canonical order.
func AllModes() []Mode {
	return []Mode{
		ModeNoRedaction,
		ModeRedact,
		ModeRedactRehydrate,
		ModeRedactRehydrateCompen,
	}
}

// Tier classifies a task by what signal it produces when it fails. Tier 1
// failures invalidate the central product bet; Tier 2 reveal product holes;
// Tier 3 are calibration.
type Tier int

const (
	Tier1 Tier = 1
	Tier2 Tier = 2
	Tier3 Tier = 3
)

// Expected declares what outcome a task is *designed* to produce in a given
// mode. Some tasks are Tier-1 demonstrations of "pure redaction breaks
// reasoning" — they are *designed* to fail in `redact` mode and pass in
// `redact+compensatory`. Treating both as "failures" would make the
// matrix meaningless; we need to distinguish "the design held" from "a
// regression happened."
type Expected int

const (
	// ExpectedPass means the task should pass in this mode. A failure is a
	// real regression.
	ExpectedPass Expected = iota

	// ExpectedFail means the task is designed to fail in this mode. A pass
	// is surprising and means either the task is not testing what we
	// thought, or the product became unexpectedly good — either outcome
	// deserves investigation.
	ExpectedFail
)

func (e Expected) String() string {
	if e == ExpectedPass {
		return "pass"
	}
	return "fail"
}

// Task is one benchmark task. Implementations live in internal/eval/tasks.
type Task interface {
	// ID is a short stable identifier (e.g. "t07-placeholder-entropy").
	ID() string

	// Title is the human-readable one-line description.
	Title() string

	// Tier classifies the task for reporting.
	Tier() Tier

	// SupportedModes lists the modes this task runs in. Tasks that are mode-
	// agnostic (direct measurements) return a single element.
	SupportedModes() []Mode

	// Expected returns what outcome the task is designed to produce in the
	// given mode. Used by the runner to distinguish expected failures
	// (design-validated) from unexpected failures (regressions).
	Expected(mode Mode) Expected

	// Run executes the task in the given mode against the given redactor.
	// The returned Result reports pass/fail and optional metrics.
	Run(r Redactor, mode Mode) Result
}

// Result is a single task run outcome.
type Result struct {
	Pass    bool
	Reason  string            // short explanation when Pass is false
	Metrics map[string]string // optional free-form metrics
}

// PassResult is a convenience constructor.
func PassResult(metrics map[string]string) Result {
	return Result{Pass: true, Metrics: metrics}
}

// FailResult is a convenience constructor.
func FailResult(reason string, metrics map[string]string) Result {
	return Result{Pass: false, Reason: reason, Metrics: metrics}
}
