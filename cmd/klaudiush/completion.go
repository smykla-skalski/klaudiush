// Package main provides the CLI entry point for klaudiush.
package main

import (
	"os"

	"github.com/cockroachdb/errors"
	"github.com/spf13/cobra"
)

var completionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion script",
	Long: `Generate shell completion script for klaudiush.

To load completions:

Bash:

  $ source <(klaudiush completion bash)

  # To load completions for each session, execute once:
  # Linux:
  $ klaudiush completion bash > /etc/bash_completion.d/klaudiush
  # macOS:
  $ klaudiush completion bash > $(brew --prefix)/etc/bash_completion.d/klaudiush

Zsh:

  # If shell completion is not already enabled in your environment,
  # you will need to enable it.  You can execute the following once:

  $ echo "autoload -U compinit; compinit" >> ~/.zshrc

  # To load completions for each session, execute once:
  $ klaudiush completion zsh > "${fpath[1]}/_klaudiush"

  # You will need to start a new shell for this setup to take effect.

Fish:

  $ klaudiush completion fish | source

  # To load completions for each session, execute once:
  $ klaudiush completion fish > ~/.config/fish/completions/klaudiush.fish

PowerShell:

  PS> klaudiush completion powershell | Out-String | Invoke-Expression

  # To load completions for every new session, run:
  PS> klaudiush completion powershell > klaudiush.ps1
  # and source this file from your PowerShell profile.
`,
	DisableFlagsInUseLine: true,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	RunE:                  runCompletion,
}

func init() {
	rootCmd.AddCommand(completionCmd)
}

func runCompletion(_ *cobra.Command, args []string) error {
	var err error

	switch args[0] {
	case "bash":
		err = rootCmd.GenBashCompletion(os.Stdout)
	case "zsh":
		err = rootCmd.GenZshCompletion(os.Stdout)
	case "fish":
		err = rootCmd.GenFishCompletion(os.Stdout, true)
	case "powershell":
		err = rootCmd.GenPowerShellCompletionWithDesc(os.Stdout)
	}

	if err != nil {
		return errors.Wrap(err, "failed to generate completion script")
	}

	return nil
}
