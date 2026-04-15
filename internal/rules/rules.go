// Package rules defines the built-in secret detection rules.
//
// Phase 0 ships a curated set of high-signal rules covering the most common
// secret formats developers hit in .env files and config.
//
// Rule provenance: most patterns were written from service docs, then
// cross-checked against gitleaks' default config (v8.30.1) to adopt any
// stricter prefix anchors gitleaks had converged on. A handful of
// additional anchored-prefix rules were transcribed directly from gitleaks
// (see `// from gitleaks:` comments). The gitleaks project is MIT-licensed;
// we thank the gitleaks authors for the upstream rule research. We do not
// depend on the gitleaks Go package — its transitive dep graph (wazero
// WASM runtime, viper, cobra, sprig, 7 archive libraries) is too heavy
// for a zero-dep scanner with a "screenshot-safe, auditable binary"
// goal (see implementation-log entry 10 for the decision).
//
// All rules in this file are **anchored** — they match a fixed public
// prefix followed by a charset-constrained body, and they use word
// boundaries where appropriate. We deliberately do not transcribe
// gitleaks' "context-dependent" rules (the ones that look for a
// service name within 50 chars of an assignment operator) because
// those need a keyword-prefilter and a capture-group API that our
// detector does not have today.
package rules

import (
	"net/url"
	"regexp"
	"strings"
)

// Rule is a single pattern-based secret detector.
type Rule struct {
	Name         string
	Pattern      *regexp.Regexp
	Label        string
	PublicPrefix string
	Description  string

	// Normalize, if non-nil, is called on a matched value and returns a
	// structural description that is embedded verbatim in the placeholder
	// (instead of `PublicPrefix...`). This is how connection-string rules
	// implement PRD §5's "preserve structure (user, host, port, database)
	// in the context passed to the LLM but strip query-string parameters
	// that frequently hold tokens." A postgres URL gets
	// `[POSTGRES_CONNSTRING:admin@prod-db:5432/myapp:suffix]` instead of
	// an opaque `[POSTGRES_CONNSTRING:postgresql://...:suffix]`.
	//
	// Returning the empty string means "no structural form available;
	// fall back to the default PublicPrefix+... rendering." This lets
	// the normalizer fail safely on malformed URLs without breaking the
	// detection path.
	Normalize func(value string) string
}

// NormalizeConnString parses a URL-shaped connection string and returns
// a structural description — username (no password), host:port, path —
// suitable for embedding in a placeholder. Query string is stripped
// entirely because query params commonly carry secrets.
//
// On any parse failure or unexpected shape the function returns the
// empty string, which causes the caller to fall back to the opaque
// placeholder form. This is the right tradeoff for a redactor: a
// connection string we cannot parse structurally is still worth
// redacting as a unit rather than leaking.
func NormalizeConnString(raw string) string {
	// Quote URLs are sometimes wrapped in whitespace or trailing
	// punctuation that url.Parse doesn't strip.
	raw = strings.TrimSpace(raw)
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	var sb strings.Builder
	if u.User != nil {
		if name := u.User.Username(); name != "" {
			sb.WriteString(name)
			sb.WriteString("@")
		}
	}
	sb.WriteString(u.Host)
	if u.Path != "" {
		sb.WriteString(u.Path)
	}
	out := sb.String()
	// Sanitize: the placeholder grammar uses `[` and `]` as delimiters,
	// so a structural description that contains them (e.g. IPv6 literal
	// `[::1]`) would confuse the rehydration regex. Replace with round
	// brackets — the agent still sees a readable host form.
	out = strings.ReplaceAll(out, "[", "(")
	out = strings.ReplaceAll(out, "]", ")")
	return out
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
		// from gitleaks aws-access-token: widened to the full set of
		// AWS access-key prefixes (permanent, temporary, bedrock, etc.)
		// with the RFC 4648 base32 body the access-key format actually uses.
		Name:         "aws-access-key",
		Pattern:      regexp.MustCompile(`\b(?:A3T[A-Z0-9]|AKIA|ASIA|ABIA|ACCA)[A-Z2-7]{16}\b`),
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
		// from gitleaks github-app-token
		Name:         "github-app-token",
		Pattern:      regexp.MustCompile(`\b(?:ghu|ghs)_[A-Za-z0-9]{36}\b`),
		Label:        "GITHUB_APP_TOKEN",
		PublicPrefix: "ghs_",
		Description:  "GitHub app installation / user token",
	},
	{
		// from gitleaks github-refresh-token
		Name:         "github-refresh-token",
		Pattern:      regexp.MustCompile(`\bghr_[A-Za-z0-9]{36}\b`),
		Label:        "GITHUB_REFRESH_TOKEN",
		PublicPrefix: "ghr_",
		Description:  "GitHub refresh token",
	},
	{
		// from gitleaks github-fine-grained-pat. `\w` = [A-Za-z0-9_].
		Name:         "github-fine-grained-pat",
		Pattern:      regexp.MustCompile(`\bgithub_pat_\w{82}\b`),
		Label:        "GITHUB_FINE_GRAINED_PAT",
		PublicPrefix: "github_pat_",
		Description:  "GitHub fine-grained personal access token",
	},
	{
		// from gitleaks gitlab-pat. gitleaks uses `glpat-[\w-]{20}`
		// without anchoring — we add \b on both sides to avoid matching
		// inside longer words.
		Name:         "gitlab-pat",
		Pattern:      regexp.MustCompile(`\bglpat-[A-Za-z0-9_\-]{20}\b`),
		Label:        "GITLAB_PAT",
		PublicPrefix: "glpat-",
		Description:  "GitLab personal access token",
	},
	{
		// from gitleaks gitlab-ptt (pipeline trigger token)
		Name:         "gitlab-ptt",
		Pattern:      regexp.MustCompile(`\bglptt-[a-f0-9]{40}\b`),
		Label:        "GITLAB_PTT",
		PublicPrefix: "glptt-",
		Description:  "GitLab pipeline trigger token",
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
		// from gitleaks huggingface-access-token
		Name:         "huggingface-access-token",
		Pattern:      regexp.MustCompile(`\bhf_[a-zA-Z]{34}\b`),
		Label:        "HUGGINGFACE_TOKEN",
		PublicPrefix: "hf_",
		Description:  "Hugging Face access token",
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
		// from gitleaks slack-app-token
		Name:         "slack-app-token",
		Pattern:      regexp.MustCompile(`\bxapp-\d-[A-Z0-9]+-\d+-[a-zA-Z0-9]+\b`),
		Label:        "SLACK_APP_TOKEN",
		PublicPrefix: "xapp-",
		Description:  "Slack app-level token",
	},
	{
		// from gitleaks slack-webhook-url. Anchored on the full URL.
		Name:         "slack-webhook-url",
		Pattern:      regexp.MustCompile(`https://hooks\.slack\.com/(?:services|workflows|triggers)/[A-Za-z0-9+/]{43,56}`),
		Label:        "SLACK_WEBHOOK_URL",
		PublicPrefix: "https://hooks.slack.com/",
		Description:  "Slack incoming webhook URL",
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
		Normalize:    NormalizeConnString,
	},
	{
		Name:         "mongodb-url",
		Pattern:      regexp.MustCompile(`\bmongodb(?:\+srv)?://[^\s"']+\b`),
		Label:        "MONGODB_CONNSTRING",
		PublicPrefix: "mongodb://",
		Description:  "MongoDB connection string",
		Normalize:    NormalizeConnString,
	},
	{
		Name:         "rsa-private-key",
		Pattern:      regexp.MustCompile(`-----BEGIN (?:RSA |EC |OPENSSH |DSA |PGP )?PRIVATE KEY-----`),
		Label:        "PRIVATE_KEY",
		PublicPrefix: "-----BEGIN",
		Description:  "Private key (PEM)",
	},
	{
		// from gitleaks 1password-secret-key
		Name:         "1password-secret-key",
		Pattern:      regexp.MustCompile(`\bA3-[A-Z0-9]{6}-(?:[A-Z0-9]{11}|[A-Z0-9]{6}-[A-Z0-9]{5})-[A-Z0-9]{5}-[A-Z0-9]{5}-[A-Z0-9]{5}\b`),
		Label:        "ONEPASSWORD_SECRET_KEY",
		PublicPrefix: "A3-",
		Description:  "1Password secret key (account recovery)",
	},
	{
		// from gitleaks age-secret-key. Bech32 alphabet.
		Name:         "age-secret-key",
		Pattern:      regexp.MustCompile(`AGE-SECRET-KEY-1[QPZRY9X8GF2TVDW0S3JN54KHCE6MUA7L]{58}`),
		Label:        "AGE_SECRET_KEY",
		PublicPrefix: "AGE-SECRET-KEY-1",
		Description:  "Age encryption secret key",
	},
	{
		// from gitleaks npm-access-token
		Name:         "npm-access-token",
		Pattern:      regexp.MustCompile(`\bnpm_[A-Za-z0-9]{36}\b`),
		Label:        "NPM_ACCESS_TOKEN",
		PublicPrefix: "npm_",
		Description:  "npm access token",
	},
	{
		// from gitleaks pypi-upload-token
		Name:         "pypi-upload-token",
		Pattern:      regexp.MustCompile(`\bpypi-AgEIcHlwaS5vcmc[A-Za-z0-9_\-]{50,1000}\b`),
		Label:        "PYPI_UPLOAD_TOKEN",
		PublicPrefix: "pypi-",
		Description:  "PyPI upload token",
	},
	{
		// from gitleaks sendgrid-api-token
		Name:         "sendgrid-api-token",
		Pattern:      regexp.MustCompile(`\bSG\.[A-Za-z0-9_\-]{22}\.[A-Za-z0-9_\-]{43}\b`),
		Label:        "SENDGRID_API_TOKEN",
		PublicPrefix: "SG.",
		Description:  "SendGrid API token",
	},
	{
		// from gitleaks databricks-api-token
		Name:         "databricks-api-token",
		Pattern:      regexp.MustCompile(`\bdapi[a-f0-9]{32}(?:-\d)?\b`),
		Label:        "DATABRICKS_API_TOKEN",
		PublicPrefix: "dapi",
		Description:  "Databricks API token",
	},
	{
		// from gitleaks digitalocean-pat
		Name:         "digitalocean-pat",
		Pattern:      regexp.MustCompile(`\bdop_v1_[a-f0-9]{64}\b`),
		Label:        "DIGITALOCEAN_PAT",
		PublicPrefix: "dop_v1_",
		Description:  "DigitalOcean personal access token",
	},
	{
		// from gitleaks digitalocean-access-token
		Name:         "digitalocean-access-token",
		Pattern:      regexp.MustCompile(`\bdoo_v1_[a-f0-9]{64}\b`),
		Label:        "DIGITALOCEAN_OAUTH_TOKEN",
		PublicPrefix: "doo_v1_",
		Description:  "DigitalOcean OAuth access token",
	},
	{
		// from gitleaks doppler-api-token
		Name:         "doppler-api-token",
		Pattern:      regexp.MustCompile(`\bdp\.pt\.[a-zA-Z0-9]{43}\b`),
		Label:        "DOPPLER_API_TOKEN",
		PublicPrefix: "dp.pt.",
		Description:  "Doppler API token",
	},
	{
		// from gitleaks linear-api-key
		Name:         "linear-api-key",
		Pattern:      regexp.MustCompile(`\blin_api_[a-zA-Z0-9]{40}\b`),
		Label:        "LINEAR_API_KEY",
		PublicPrefix: "lin_api_",
		Description:  "Linear API key",
	},
	{
		// from gitleaks notion-api-token
		Name:         "notion-api-token",
		Pattern:      regexp.MustCompile(`\bntn_[0-9]{11}[A-Za-z0-9]{35}\b`),
		Label:        "NOTION_API_TOKEN",
		PublicPrefix: "ntn_",
		Description:  "Notion API token",
	},
	{
		// from gitleaks postman-api-token
		Name:         "postman-api-token",
		Pattern:      regexp.MustCompile(`\bPMAK-[a-f0-9]{24}-[a-f0-9]{34}\b`),
		Label:        "POSTMAN_API_TOKEN",
		PublicPrefix: "PMAK-",
		Description:  "Postman API token",
	},
	{
		// from gitleaks shopify-access-token family
		Name:         "shopify-access-token",
		Pattern:      regexp.MustCompile(`\bshpat_[a-fA-F0-9]{32}\b`),
		Label:        "SHOPIFY_ACCESS_TOKEN",
		PublicPrefix: "shpat_",
		Description:  "Shopify access token",
	},
	{
		Name:         "shopify-shared-secret",
		Pattern:      regexp.MustCompile(`\bshpss_[a-fA-F0-9]{32}\b`),
		Label:        "SHOPIFY_SHARED_SECRET",
		PublicPrefix: "shpss_",
		Description:  "Shopify shared secret",
	},
	{
		// from gitleaks vault-service-token (hvs. variant only — the
		// `s.` alternative is too generic to anchor safely).
		Name:         "vault-service-token",
		Pattern:      regexp.MustCompile(`\bhvs\.[A-Za-z0-9_\-]{90,120}\b`),
		Label:        "VAULT_SERVICE_TOKEN",
		PublicPrefix: "hvs.",
		Description:  "HashiCorp Vault service token",
	},
	{
		// from gitleaks grafana-service-account-token
		Name:         "grafana-service-account-token",
		Pattern:      regexp.MustCompile(`\bglsa_[A-Za-z0-9]{32}_[A-Fa-f0-9]{8}\b`),
		Label:        "GRAFANA_SERVICE_ACCOUNT",
		PublicPrefix: "glsa_",
		Description:  "Grafana service-account token",
	},
}

// Builtins returns the embedded rule set. The returned slice is safe to iterate
// but callers must not mutate entries.
func Builtins() []Rule {
	return builtins
}
