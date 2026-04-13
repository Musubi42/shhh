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
