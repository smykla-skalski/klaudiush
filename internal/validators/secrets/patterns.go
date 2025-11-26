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

	// ErrorCode is the error code to use when this pattern matches.
	ErrorCode validator.ErrorCode
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
		ErrorCode:   validator.ErrSecretsAPIKey,
	},
	{
		Name:        "aws-secret-key",
		Description: "AWS Secret Access Key",
		Regex: regexp.MustCompile(
			`(?i)aws[_-]?secret[_-]?access[_-]?key['"\s:=]+[A-Za-z0-9/+=]{40}`,
		),
		ErrorCode: validator.ErrSecretsAPIKey,
	},

	// GitHub Tokens
	{
		Name:        "github-pat",
		Description: "GitHub Personal Access Token",
		Regex:       regexp.MustCompile(`ghp_[A-Za-z0-9_]{36}`),
		ErrorCode:   validator.ErrSecretsToken,
	},
	{
		Name:        "github-oauth",
		Description: "GitHub OAuth Access Token",
		Regex:       regexp.MustCompile(`gho_[A-Za-z0-9_]{36}`),
		ErrorCode:   validator.ErrSecretsToken,
	},
	{
		Name:        "github-app",
		Description: "GitHub App Token",
		Regex:       regexp.MustCompile(`(?:ghu|ghs)_[A-Za-z0-9_]{36}`),
		ErrorCode:   validator.ErrSecretsToken,
	},
	{
		Name:        "github-refresh",
		Description: "GitHub Refresh Token",
		Regex:       regexp.MustCompile(`ghr_[A-Za-z0-9_]{36}`),
		ErrorCode:   validator.ErrSecretsToken,
	},

	// GitLab Token
	{
		Name:        "gitlab-pat",
		Description: "GitLab Personal Access Token",
		Regex:       regexp.MustCompile(`glpat-[A-Za-z0-9_-]{20}`),
		ErrorCode:   validator.ErrSecretsToken,
	},

	// Slack
	{
		Name:        "slack-token",
		Description: "Slack Token",
		Regex:       regexp.MustCompile(`xox[baprs]-[0-9]{10,13}-[0-9]{10,13}[a-zA-Z0-9-]*`),
		ErrorCode:   validator.ErrSecretsToken,
	},
	{
		Name:        "slack-webhook",
		Description: "Slack Webhook URL",
		Regex: regexp.MustCompile(
			`(?:^|://|[^/a-zA-Z0-9])https://hooks\.slack\.com/services/T[A-Z0-9]{8,20}/B[A-Z0-9]{8,20}/[A-Za-z0-9]{24}\b`,
		),
		ErrorCode: validator.ErrSecretsToken,
	},

	// Google/GCP
	{
		Name:        "google-api-key",
		Description: "Google API Key",
		Regex:       regexp.MustCompile(`AIza[0-9A-Za-z_-]{35}`),
		ErrorCode:   validator.ErrSecretsAPIKey,
	},
	{
		Name:        "gcp-service-account",
		Description: "GCP Service Account Key",
		Regex:       regexp.MustCompile(`"type":\s*"service_account"`),
		ErrorCode:   validator.ErrSecretsAPIKey,
	},

	// Private Keys
	{
		Name:        "private-key-rsa",
		Description: "RSA Private Key",
		Regex:       regexp.MustCompile(`-----BEGIN RSA PRIVATE KEY-----`),
		ErrorCode:   validator.ErrSecretsPrivKey,
	},
	{
		Name:        "private-key-dsa",
		Description: "DSA Private Key",
		Regex:       regexp.MustCompile(`-----BEGIN DSA PRIVATE KEY-----`),
		ErrorCode:   validator.ErrSecretsPrivKey,
	},
	{
		Name:        "private-key-ec",
		Description: "EC Private Key",
		Regex:       regexp.MustCompile(`-----BEGIN EC PRIVATE KEY-----`),
		ErrorCode:   validator.ErrSecretsPrivKey,
	},
	{
		Name:        "private-key-openssh",
		Description: "OpenSSH Private Key",
		Regex:       regexp.MustCompile(`-----BEGIN OPENSSH PRIVATE KEY-----`),
		ErrorCode:   validator.ErrSecretsPrivKey,
	},
	{
		Name:        "private-key-pgp",
		Description: "PGP Private Key Block",
		Regex:       regexp.MustCompile(`-----BEGIN PGP PRIVATE KEY BLOCK-----`),
		ErrorCode:   validator.ErrSecretsPrivKey,
	},

	// Database Connection Strings
	{
		Name:        "mongodb-conn",
		Description: "MongoDB Connection String",
		Regex:       regexp.MustCompile(`mongodb(?:\+srv)?://[^:]+:[^@]+@[^\s"']+`),
		ErrorCode:   validator.ErrSecretsConnString,
	},
	{
		Name:        "postgres-conn",
		Description: "PostgreSQL Connection String",
		Regex:       regexp.MustCompile(`postgres(?:ql)?://[^:]+:[^@]+@[^\s"']+`),
		ErrorCode:   validator.ErrSecretsConnString,
	},
	{
		Name:        "mysql-conn",
		Description: "MySQL Connection String",
		Regex:       regexp.MustCompile(`mysql://[^:]+:[^@]+@[^\s"']+`),
		ErrorCode:   validator.ErrSecretsConnString,
	},
	{
		Name:        "redis-conn",
		Description: "Redis Connection String",
		Regex:       regexp.MustCompile(`redis://[^:]+:[^@]+@[^\s"']+`),
		ErrorCode:   validator.ErrSecretsConnString,
	},

	// Generic Patterns (higher false positive risk, but useful)
	{
		Name:        "generic-password",
		Description: "Hardcoded Password",
		Regex: regexp.MustCompile(
			`(?i)(?:password|passwd|pwd)['"\s]*[:=]['"\s]*[^\s'"]{8,64}['"]`,
		),
		ErrorCode: validator.ErrSecretsPassword,
	},
	{
		Name:        "generic-secret",
		Description: "Generic Secret Assignment",
		Regex: regexp.MustCompile(
			`(?i)(?:secret|api[_-]?key|auth[_-]?token)['"\s]*[:=]['"\s]*[A-Za-z0-9/+=_-]{20,}['"]`,
		),
		ErrorCode: validator.ErrSecretsAPIKey,
	},

	// NPM Token
	{
		Name:        "npm-token",
		Description: "NPM Access Token",
		Regex:       regexp.MustCompile(`npm_[A-Za-z0-9]{36}`),
		ErrorCode:   validator.ErrSecretsToken,
	},

	// Stripe
	{
		Name:        "stripe-api-key",
		Description: "Stripe API Key",
		Regex:       regexp.MustCompile(`(?:sk|pk)_(?:live|test)_[A-Za-z0-9]{24,}`),
		ErrorCode:   validator.ErrSecretsAPIKey,
	},

	// Twilio
	{
		Name:        "twilio-api-key",
		Description: "Twilio API Key",
		Regex:       regexp.MustCompile(`SK[0-9a-fA-F]{32}`),
		ErrorCode:   validator.ErrSecretsAPIKey,
	},

	// SendGrid
	{
		Name:        "sendgrid-api-key",
		Description: "SendGrid API Key",
		Regex:       regexp.MustCompile(`SG\.[A-Za-z0-9_-]{22}\.[A-Za-z0-9_-]{43}`),
		ErrorCode:   validator.ErrSecretsAPIKey,
	},

	// Mailgun
	{
		Name:        "mailgun-api-key",
		Description: "Mailgun API Key",
		Regex:       regexp.MustCompile(`key-[0-9a-zA-Z]{32}`),
		ErrorCode:   validator.ErrSecretsAPIKey,
	},

	// JWT
	{
		Name:        "jwt-token",
		Description: "JSON Web Token",
		Regex:       regexp.MustCompile(`eyJ[A-Za-z0-9_-]*\.eyJ[A-Za-z0-9_-]*\.[A-Za-z0-9_-]*`),
		ErrorCode:   validator.ErrSecretsToken,
	},

	// Heroku
	{
		Name:        "heroku-api-key",
		Description: "Heroku API Key",
		Regex: regexp.MustCompile(
			`[hH]eroku[A-Za-z0-9_-]*['\s:=]+[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}`,
		),
		ErrorCode: validator.ErrSecretsAPIKey,
	},

	// Azure
	{
		Name:        "azure-storage-key",
		Description: "Azure Storage Account Key",
		Regex: regexp.MustCompile(
			`DefaultEndpointsProtocol=https;AccountName=[^;]+;AccountKey=[A-Za-z0-9+/=]{88};`,
		),
		ErrorCode: validator.ErrSecretsAPIKey,
	},
}

// DefaultPatterns returns a copy of the default secret detection patterns.
func DefaultPatterns() []Pattern {
	patterns := make([]Pattern, len(defaultPatterns))
	copy(patterns, defaultPatterns)

	return patterns
}
