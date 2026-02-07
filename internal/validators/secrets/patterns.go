// Package secrets provides validators for detecting secrets in file content.
package secrets

import (
	"regexp"

	"github.com/smykla-labs/klaudiush/internal/validator"
)

// Pattern represents a secret detection pattern with metadata.
type Pattern struct {
	// Name is a human-readable identifier for the pattern.
	Name string

	// Description explains what type of secret this pattern detects.
	Description string

	// Regex is the regular expression pattern for detection.
	Regex *regexp.Regexp

	// Reference is the URL that uniquely identifies this error type.
	Reference validator.Reference
}

// Finding represents a detected secret in content.
type Finding struct {
	// Pattern is the pattern that matched.
	Pattern *Pattern

	// Match is the actual matched text.
	Match string

	// Line is the line number where the match was found (1-indexed).
	Line int

	// Column is the column number where the match starts (1-indexed).
	Column int
}

// defaultPatterns contains the built-in secret detection patterns.
var defaultPatterns = []Pattern{
	// AWS Credentials
	{
		Name:        "aws-access-key-id",
		Description: "AWS Access Key ID",
		Regex:       regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
		Reference:   validator.RefSecretsAPIKey,
	},
	{
		Name:        "aws-secret-key",
		Description: "AWS Secret Access Key",
		Regex: regexp.MustCompile(
			`(?i)aws[_-]?secret[_-]?access[_-]?key['"\s:=]+[A-Za-z0-9/+=]{40}`,
		),
		Reference: validator.RefSecretsAPIKey,
	},

	// GitHub Tokens
	{
		Name:        "github-pat",
		Description: "GitHub Personal Access Token",
		Regex:       regexp.MustCompile(`ghp_[A-Za-z0-9_]{36}`),
		Reference:   validator.RefSecretsToken,
	},
	{
		Name:        "github-oauth",
		Description: "GitHub OAuth Access Token",
		Regex:       regexp.MustCompile(`gho_[A-Za-z0-9_]{36}`),
		Reference:   validator.RefSecretsToken,
	},
	{
		Name:        "github-app",
		Description: "GitHub App Token",
		Regex:       regexp.MustCompile(`(?:ghu|ghs)_[A-Za-z0-9_]{36}`),
		Reference:   validator.RefSecretsToken,
	},
	{
		Name:        "github-refresh",
		Description: "GitHub Refresh Token",
		Regex:       regexp.MustCompile(`ghr_[A-Za-z0-9_]{36}`),
		Reference:   validator.RefSecretsToken,
	},

	// GitLab Token
	{
		Name:        "gitlab-pat",
		Description: "GitLab Personal Access Token",
		Regex:       regexp.MustCompile(`glpat-[A-Za-z0-9_-]{20}`),
		Reference:   validator.RefSecretsToken,
	},

	// Slack
	{
		Name:        "slack-token",
		Description: "Slack Token",
		Regex:       regexp.MustCompile(`xox[baprs]-[0-9]{10,13}-[0-9]{10,13}[a-zA-Z0-9-]*`),
		Reference:   validator.RefSecretsToken,
	},
	{
		Name:        "slack-webhook",
		Description: "Slack Webhook URL",
		Regex: regexp.MustCompile(
			`(?:^|://|[^/a-zA-Z0-9])https://hooks\.slack\.com/services/T[A-Z0-9]{8,20}/B[A-Z0-9]{8,20}/[A-Za-z0-9]{24}\b`,
		),
		Reference: validator.RefSecretsToken,
	},

	// Google/GCP
	{
		Name:        "google-api-key",
		Description: "Google API Key",
		Regex:       regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`),
		Reference:   validator.RefSecretsAPIKey,
	},
	{
		Name:        "gcp-service-account",
		Description: "GCP Service Account Key",
		Regex:       regexp.MustCompile(`"type":\s*"service_account"`),
		Reference:   validator.RefSecretsAPIKey,
	},

	// Private Keys
	{
		Name:        "private-key-rsa",
		Description: "RSA Private Key",
		Regex:       regexp.MustCompile(`-----BEGIN RSA PRIVATE KEY-----`),
		Reference:   validator.RefSecretsPrivKey,
	},
	{
		Name:        "private-key-dsa",
		Description: "DSA Private Key",
		Regex:       regexp.MustCompile(`-----BEGIN DSA PRIVATE KEY-----`),
		Reference:   validator.RefSecretsPrivKey,
	},
	{
		Name:        "private-key-ec",
		Description: "EC Private Key",
		Regex:       regexp.MustCompile(`-----BEGIN EC PRIVATE KEY-----`),
		Reference:   validator.RefSecretsPrivKey,
	},
	{
		Name:        "private-key-openssh",
		Description: "OpenSSH Private Key",
		Regex:       regexp.MustCompile(`-----BEGIN OPENSSH PRIVATE KEY-----`),
		Reference:   validator.RefSecretsPrivKey,
	},
	{
		Name:        "private-key-pgp",
		Description: "PGP Private Key Block",
		Regex:       regexp.MustCompile(`-----BEGIN PGP PRIVATE KEY BLOCK-----`),
		Reference:   validator.RefSecretsPrivKey,
	},

	// Database Connection Strings
	{
		Name:        "mongodb-conn",
		Description: "MongoDB Connection String",
		Regex:       regexp.MustCompile(`mongodb(?:\+srv)?://[^:]+:[^@]+@[^\s"']+`),
		Reference:   validator.RefSecretsConnString,
	},
	{
		Name:        "postgres-conn",
		Description: "PostgreSQL Connection String",
		Regex:       regexp.MustCompile(`postgres(?:ql)?://[^:]+:[^@]+@[^\s"']+`),
		Reference:   validator.RefSecretsConnString,
	},
	{
		Name:        "mysql-conn",
		Description: "MySQL Connection String",
		Regex:       regexp.MustCompile(`mysql://[^:]+:[^@]+@[^\s"']+`),
		Reference:   validator.RefSecretsConnString,
	},
	{
		Name:        "redis-conn",
		Description: "Redis Connection String",
		Regex:       regexp.MustCompile(`redis://[^:]+:[^@]+@[^\s"']+`),
		Reference:   validator.RefSecretsConnString,
	},

	// Generic Patterns (higher false positive risk, but useful)
	{
		Name:        "generic-password",
		Description: "Hardcoded Password",
		Regex: regexp.MustCompile(
			`(?i)(?:password|passwd|pwd)['"\s]*[:=]['"\s]*[^\s'"]{8,64}['"]`,
		),
		Reference: validator.RefSecretsPassword,
	},
	{
		Name:        "generic-secret",
		Description: "Generic Secret Assignment",
		Regex: regexp.MustCompile(
			`(?i)(?:secret|api[_-]?key|auth[_-]?token)['"\s]*[:=]['"\s]*[A-Za-z0-9/+=_-]{20,}['"]`,
		),
		Reference: validator.RefSecretsAPIKey,
	},

	// NPM Token
	{
		Name:        "npm-token",
		Description: "NPM Access Token",
		Regex:       regexp.MustCompile(`npm_[A-Za-z0-9]{36}`),
		Reference:   validator.RefSecretsToken,
	},

	// Stripe
	{
		Name:        "stripe-api-key",
		Description: "Stripe API Key",
		Regex:       regexp.MustCompile(`(?:sk|pk)_(?:live|test)_[A-Za-z0-9]{24,}`),
		Reference:   validator.RefSecretsAPIKey,
	},

	// Twilio
	{
		Name:        "twilio-api-key",
		Description: "Twilio API Key",
		Regex:       regexp.MustCompile(`SK[0-9a-fA-F]{32}`),
		Reference:   validator.RefSecretsAPIKey,
	},

	// SendGrid
	{
		Name:        "sendgrid-api-key",
		Description: "SendGrid API Key",
		Regex:       regexp.MustCompile(`SG\.[A-Za-z0-9_-]{22}\.[A-Za-z0-9_-]{43}`),
		Reference:   validator.RefSecretsAPIKey,
	},

	// Mailgun
	{
		Name:        "mailgun-api-key",
		Description: "Mailgun API Key",
		Regex:       regexp.MustCompile(`key-[0-9a-zA-Z]{32}`),
		Reference:   validator.RefSecretsAPIKey,
	},

	// JWT
	{
		Name:        "jwt-token",
		Description: "JSON Web Token",
		Regex:       regexp.MustCompile(`eyJ[A-Za-z0-9_-]*\.eyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]*`),
		Reference:   validator.RefSecretsToken,
	},

	// Heroku
	{
		Name:        "heroku-api-key",
		Description: "Heroku API Key",
		Regex: regexp.MustCompile(
			`[hH]eroku[A-Za-z0-9_-]*['\s:=]+[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`,
		),
		Reference: validator.RefSecretsAPIKey,
	},

	// Azure
	{
		Name:        "azure-storage-key",
		Description: "Azure Storage Account Key",
		Regex: regexp.MustCompile(
			`DefaultEndpointsProtocol=https;AccountName=[^;]+;AccountKey=[A-Za-z0-9+/=]{88};`,
		),
		Reference: validator.RefSecretsAPIKey,
	},
}

// DefaultPatterns returns a copy of the default secret detection patterns.
func DefaultPatterns() []Pattern {
	patterns := make([]Pattern, len(defaultPatterns))
	copy(patterns, defaultPatterns)

	return patterns
}
