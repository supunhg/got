// Copyright 2026 The GOT Authors. MIT License.
package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// newCompletionCmd returns the `got completion` subcommand.
// It uses Cobra's built-in shell completion generators for bash, zsh,
// fish, and powershell.
func newCompletionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "completion [bash|zsh|fish|powershell]",
		Short: "Generate shell completion scripts",
		Long: `Generate shell completion scripts for GOT.

To load completions:

Bash:

  # Load for the current session
  source <(got completion bash)

  # Load on every login (Linux)
  got completion bash > /etc/bash_completion.d/got

  # Load on every login (macOS with Homebrew)
  got completion bash > $(brew --prefix)/etc/bash_completion.d/got

Zsh:

  # Load for the current session
  source <(got completion zsh)

  # Load on every login (first ensure compinit is loaded)
  got completion zsh > "${fpath[1]}/_got"

Fish:

  # Load for the current session
  got completion fish | source

  # Load on every login
  got completion fish > ~/.config/fish/completions/got.fish

PowerShell:

  # Load for the current session
  got completion powershell | Out-String | Invoke-Expression

  # Load on every login (add to profile)
  got completion powershell > got.ps1
  # and add ". ./got.ps1" to your PowerShell profile
`,
		DisableFlagsInUseLine: true,
		ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
		Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
		RunE: func(cmd *cobra.Command, args []string) error {
			out := cmd.OutOrStdout()
			switch args[0] {
			case "bash":
				return cmd.Root().GenBashCompletionV2(out, true)
			case "zsh":
				return cmd.Root().GenZshCompletion(out)
			case "fish":
				return cmd.Root().GenFishCompletion(out, true)
			case "powershell":
				return cmd.Root().GenPowerShellCompletionWithDesc(out)
			default:
				return fmt.Errorf("unsupported shell: %s", args[0])
			}
		},
	}

	// Add examples.
	cmd.Example = `  got completion bash > got.bash
  got completion zsh > _got
  got completion fish > got.fish
  got completion powershell > got.ps1`

	return cmd
}

