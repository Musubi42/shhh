// Package rules defines the built-in secret detection rules.
//
// Phase 0 ships a curated set of high-signal rules covering the most common
// secret formats developers hit in .env files and config. Gitleaks integration
// is deferred until after eval baseline.
package rules

import "regexp"

// Rule is a single pattern-based secret detector.
type Rule struct {
	Name         string
	Pattern      *regexp.Regexp
	Label        string
	PublicPrefix string
	Description  string
}

var builtins = []Rule{
	{
		Name:         "stripe-live-key",
		Pattern:      regexp.MustCompile(`\bsk_live_[A-Za-z0-9]{24,}\b`),
		Label:        "STRIPE_LIVE_KEY",
		PublicPrefix: "sk_live_",
		Description:  "Stripe live secret key",
	},
	{
		Name:         "stripe-test-key",
		Pattern:      regexp.MustCompile(`\bsk_test_[A-Za-z0-9]{24,}\b`),
		Label:        "STRIPE_TEST_KEY",
		PublicPrefix: "sk_test_",
		Description:  "Stripe test secret key",
	},
	{
		Name:         "aws-access-key",
		Pattern:      regexp.MustCompile(`\b(AKIA|ASIA)[0-9A-Z]{16}\b`),
		Label:        "AWS_ACCESS_KEY",
		PublicPrefix: "AKIA",
		Description:  "AWS access key ID",
	},
	{
		Name:         "github-pat",
		Pattern:      regexp.MustCompile(`\bghp_[A-Za-z0-9]{36}\b`),
		Label:        "GITHUB_PAT",
		PublicPrefix: "ghp_",
		Description:  "GitHub personal access token",
	},
	{
		Name:         "github-oauth",
		Pattern:      regexp.MustCompile(`\bgho_[A-Za-z0-9]{36}\b`),
		Label:        "GITHUB_OAUTH",
		PublicPrefix: "gho_",
		Description:  "GitHub OAuth token",
	},
	{
		Name:         "openai-project",
		Pattern:      regexp.MustCompile(`\bsk-proj-[A-Za-z0-9_\-]{20,}\b`),
		Label:        "OPENAI_PROJECT_KEY",
		PublicPrefix: "sk-proj-",
		Description:  "OpenAI project API key",
	},
	{
		Name:         "openai-legacy",
		Pattern:      regexp.MustCompile(`\bsk-[A-Za-z0-9]{48}\b`),
		Label:        "OPENAI_KEY",
		PublicPrefix: "sk-",
		Description:  "OpenAI API key",
	},
	{
		Name:         "anthropic-key",
		Pattern:      regexp.MustCompile(`\bsk-ant-[A-Za-z0-9_\-]{20,}\b`),
		Label:        "ANTHROPIC_KEY",
		PublicPrefix: "sk-ant-",
		Description:  "Anthropic API key",
	},
	{
		Name:         "slack-bot-token",
		Pattern:      regexp.MustCompile(`\bxoxb-[0-9]+-[0-9]+-[A-Za-z0-9]+\b`),
		Label:        "SLACK_BOT_TOKEN",
		PublicPrefix: "xoxb-",
		Description:  "Slack bot token",
	},
	{
		Name:         "slack-user-token",
		Pattern:      regexp.MustCompile(`\bxoxp-[0-9]+-[0-9]+-[0-9]+-[A-Za-z0-9]+\b`),
		Label:        "SLACK_USER_TOKEN",
		PublicPrefix: "xoxp-",
		Description:  "Slack user token",
	},
	{
		Name:         "google-api-key",
		Pattern:      regexp.MustCompile(`\bAIza[0-9A-Za-z_\-]{35}\b`),
		Label:        "GOOGLE_API_KEY",
		PublicPrefix: "AIza",
		Description:  "Google API key",
	},
	{
		Name:         "jwt",
		Pattern:      regexp.MustCompile(`\beyJ[A-Za-z0-9_\-]+\.eyJ[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+\b`),
		Label:        "JWT_TOKEN",
		PublicPrefix: "eyJ",
		Description:  "JSON Web Token",
	},
	{
		Name:         "postgres-url",
		Pattern:      regexp.MustCompile(`\bpostgres(?:ql)?://[^\s"']+\b`),
		Label:        "POSTGRES_CONNSTRING",
		PublicPrefix: "postgresql://",
		Description:  "PostgreSQL connection string",
	},
	{
		Name:         "mongodb-url",
		Pattern:      regexp.MustCompile(`\bmongodb(?:\+srv)?://[^\s"']+\b`),
		Label:        "MONGODB_CONNSTRING",
		PublicPrefix: "mongodb://",
		Description:  "MongoDB connection string",
	},
	{
		Name:         "rsa-private-key",
		Pattern:      regexp.MustCompile(`-----BEGIN (?:RSA |EC |OPENSSH |DSA |PGP )?PRIVATE KEY-----`),
		Label:        "PRIVATE_KEY",
		PublicPrefix: "-----BEGIN",
		Description:  "Private key (PEM)",
	},
}

// Builtins returns the embedded rule set. The returned slice is safe to iterate
// but callers must not mutate entries.
func Builtins() []Rule {
	return builtins
}
