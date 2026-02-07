package secrets_test

import (
	"testing"

	"github.com/smykla-labs/klaudiush/internal/validators/secrets"
)

func FuzzSecretsDetect(f *testing.F) {
	// Seed corpus with various secret patterns
	f.Add("AKIAIOSFODNN7EXAMPLE")
	f.Add("ghp_xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx")
	f.Add("-----BEGIN RSA PRIVATE KEY-----")
	f.Add("mongodb://user:pass@localhost:27017/db")
	f.Add("just some normal text")
	f.Add("")
	f.Add("line 1\nAKIAIOSFODNN7EXAMPLE on line 2\nline 3")
	f.Add("AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY")
	f.Add("postgres://user:password@host:5432/dbname")
	f.Add("GITHUB_TOKEN=ghp_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdef")
	f.Add("-----BEGIN PRIVATE KEY-----\nMIIEvgIBADANBgkqhkiG9w0BAQEF...")
	f.Add("sk-xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx")
	f.Add("api_key = \"sk_live_xxxxxxxxxxxxxxxxxxxxxxxxx\"")
	f.Add("password: supersecret123")
	f.Add("Bearer eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9...")
	f.Add("xoxb-1234567890-1234567890123-ABCDEFGHIJKLMNOPQRSTUVw")

	f.Fuzz(func(t *testing.T, content string) {
		detector := secrets.NewDefaultPatternDetector()
		findings := detector.Detect(content)

		for _, finding := range findings {
			if finding.Line < 1 {
				t.Errorf("invalid line number: %d", finding.Line)
			}

			if finding.Column < 1 {
				t.Errorf("invalid column number: %d", finding.Column)
			}

			if finding.Match == "" {
				t.Error("empty match")
			}

			if finding.Pattern == nil {
				t.Error("nil pattern")
			}
		}
	})
}
