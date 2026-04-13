package rules

// KnownExamples is an allowlist of literal secret values that match a
// detection rule by syntax but are universally known public placeholders.
// When the detector finds a match, it checks this map and drops the finding
// if the value is on the list.
//
// The canonical case is AWS's documented example credentials: they are
// syntactically valid access keys and cannot be distinguished from real keys
// by pattern alone, but they appear in every AWS tutorial and `.env.example`
// file in existence. Flagging them would poison the false-positive rate.
//
// This list stays small and conservative. Any addition needs a citation in
// the comment so future reviewers know it is genuinely public.
var KnownExamples = map[string]struct{}{
	// https://docs.aws.amazon.com/IAM/latest/UserGuide/security-creds.html
	"AKIAIOSFODNN7EXAMPLE":                         {},
	"wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY":     {},

	// https://stripe.com/docs/api/authentication
	// Stripe's docs use these exact test keys.
	"sk_test_4eC39HqLyjWDarjtT1zdp7dc":             {},
	"pk_test_TYooMQauvdEDq54NiTphI7jx":             {},
}

// IsKnownExample reports whether value is on the public-placeholder
// allowlist.
func IsKnownExample(value string) bool {
	_, ok := KnownExamples[value]
	return ok
}
