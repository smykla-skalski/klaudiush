package exceptions_test

import (
	"testing"

	"github.com/smykla-labs/klaudiush/internal/exceptions"
)

func FuzzTokenParse(f *testing.F) {
	// Seed with valid token patterns
	f.Add("git push # EXC:GIT022")
	f.Add("git push # EXC:GIT022:reason")
	f.Add("git push # EXC:GIT022:Emergency+hotfix")
	f.Add("git push # EXC:SEC001:Test%20fixture")
	f.Add(`KLACK="EXC:GIT022:reason" git push`)
	f.Add(`KLACK='EXC:SEC001' git commit`)
	f.Add(`KLACK=EXC:FILE001 touch file.txt`)

	// Seed with edge cases
	f.Add("")
	f.Add("   \t\n")
	f.Add("git push")
	f.Add("git push # regular comment")
	f.Add("git push # EXC:")
	f.Add("git push # EXC::")
	f.Add("git push # EXC:INVALID")
	f.Add("git push # EXC:git001")
	f.Add("git push # EXC:12345")
	f.Add("git push # EXC:GIT")
	f.Add("git push # EXC:GIT022:reason:extra:parts")
	f.Add("git push # notEXC:GIT022")
	f.Add(`KLACK="" git push`)
	f.Add(`OTHER_VAR="EXC:GIT022" git push`)

	// Seed with complex commands
	f.Add("git add . && git push # EXC:GIT022")
	f.Add("git push; echo done # EXC:GIT022")
	f.Add("(git push) # EXC:GIT022:reason")
	f.Add("echo test | git push # EXC:GIT022")
	f.Add(`git commit -m "$(date)" # EXC:GIT001`)

	// Seed with URL-encoded reasons
	f.Add("git push # EXC:GIT022:Hello%20World")
	f.Add("git push # EXC:GIT022:Test%2Bvalue")
	f.Add("git push # EXC:GIT022:Special%3A%21%40")
	f.Add("git push # EXC:GIT022:%E2%9C%93")

	// Seed with malformed tokens
	f.Add("git push # EXC:GIT022:%invalid%")
	f.Add("git push # EXC:GIT022:%%")
	f.Add("git push # EXC:GIT022:%")
	f.Add("git push # EXC:GIT022:%0")
	f.Add("git push # EXC:GIT022:%zz")

	f.Fuzz(func(_ *testing.T, command string) {
		p := exceptions.NewParser()
		result, err := p.Parse(command)

		// If parsing succeeded, access all fields to ensure no panics
		if err == nil && result != nil {
			_ = result.Found
			_ = result.Source

			if result.Token != nil {
				_ = result.Token.Prefix
				_ = result.Token.ErrorCode
				_ = result.Token.Reason
				_ = result.Token.Raw
			}
		}
	})
}

func FuzzTokenParseWithOptions(f *testing.F) {
	// Seed with commands using custom prefix
	f.Add("git push # ACK:GIT022:reason")
	f.Add(`MY_VAR="ACK:GIT022" git push`)
	f.Add("git push # OVERRIDE:SEC001")

	f.Fuzz(func(_ *testing.T, command string) {
		// Test with custom prefix
		p1 := exceptions.NewParser(exceptions.WithTokenPrefix("ACK"))
		result1, _ := p1.Parse(command)

		if result1 != nil && result1.Token != nil {
			_ = result1.Token.Prefix
			_ = result1.Token.ErrorCode
		}

		// Test with custom env var name
		p2 := exceptions.NewParser(exceptions.WithEnvVarName("MY_VAR"))
		result2, _ := p2.Parse(command)

		if result2 != nil && result2.Token != nil {
			_ = result2.Token.Prefix
			_ = result2.Token.ErrorCode
		}
	})
}
