package parser_test

import (
	"strings"
	"testing"

	"github.com/smykla-labs/klaudiush/pkg/parser"
)

func FuzzParseGitCommand(f *testing.F) {
	// Seed corpus from existing tests (tab-separated: name\targ1\targ2...)
	f.Add("git\tcommit\t-sS\t-m\ttest message")
	f.Add("git\tpush\t--force-with-lease\tupstream\tmain")
	f.Add("git\t-C\t/path/to/repo\tcheckout\t-b\tfeat/new-feature")
	f.Add("git\t--git-dir=/custom/.git\tstatus")
	f.Add("git\tbranch\t-m\told-name\tnew-name")
	f.Add("git\tswitch\t-c\tfeat/new-feature")
	f.Add("git\tcommit\t-sSm\tinline-message")
	f.Add("git")           // no args
	f.Add("ls\t-la")       // non-git
	f.Add("git\tadd\t.")   // simple add
	f.Add("git\trm\t-rf")  // rm with flags
	f.Add("git\tmv\ta\tb") // mv with args
	f.Add("git\tfetch\torigin")
	f.Add("git\tpull\torigin\tmain")

	f.Fuzz(func(t *testing.T, input string) {
		parts := strings.Split(input, "\t")
		if len(parts) == 0 {
			return
		}

		cmd := parser.Command{
			Name: parts[0],
			Args: parts[1:],
		}

		result, err := parser.ParseGitCommand(cmd)
		if err == nil {
			if result.Subcommand == "" {
				t.Error("empty subcommand with no error")
			}

			// Exercise all methods - should not panic
			_ = result.HasFlag("-s")
			_ = result.HasFlag("-S")
			_ = result.HasFlag("--signoff")
			_ = result.ExtractCommitMessage()
			_ = result.ExtractRemote()
			_ = result.ExtractBranchName()
			_ = result.ExtractFilePaths()
			_ = result.GetWorkingDirectory()
			_ = result.GetGitDir()
			_ = result.GetFlagValue("-m")
			_ = result.HasGlobalOption("-C")
		}
	})
}
