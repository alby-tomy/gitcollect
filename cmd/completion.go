package cmd

import (
	"fmt"
	"os"

	"golang.org/x/term"

	"github.com/spf13/cobra"
)

var completionIsTerminalFn = func() bool { return term.IsTerminal(int(os.Stdout.Fd())) }

var completionCmd = &cobra.Command{
	Use:       "completion [bash|zsh|fish|powershell]",
	Short:     "Generate shell completion scripts",
	ValidArgs: []string{"bash", "zsh", "fish", "powershell"},
	Args:      cobra.ExactArgs(1),
	RunE:      runCompletion,
}

func runCompletion(cmd *cobra.Command, args []string) error {
	shell := args[0]

	switch shell {
	case "bash":
		if err := rootCmd.GenBashCompletion(os.Stdout); err != nil {
			return err
		}
	case "zsh":
		if err := rootCmd.GenZshCompletion(os.Stdout); err != nil {
			return err
		}
	case "fish":
		if err := rootCmd.GenFishCompletion(os.Stdout, true); err != nil {
			return err
		}
	case "powershell":
		if err := rootCmd.GenPowerShellCompletion(os.Stdout); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported shell %q — try bash, zsh, fish, or powershell", shell)
	}

	if completionIsTerminalFn() {
		fmt.Fprintf(os.Stderr, "Pipe this to your shell profile: gitcollect completion %s >> %s\n",
			shell, completionProfileFile(shell))
	} else {
		fmt.Fprintf(os.Stderr, "# To activate completions: %s\n", completionActivateHint(shell))
	}
	return nil
}

func completionProfileFile(shell string) string {
	switch shell {
	case "bash":
		return "~/.bashrc"
	case "zsh":
		return "~/.zshrc"
	case "fish":
		return "~/.config/fish/completions/gitcollect.fish"
	default:
		return "$PROFILE"
	}
}

func completionActivateHint(shell string) string {
	switch shell {
	case "bash":
		return "source ~/.bashrc"
	case "zsh":
		return "source ~/.zshrc"
	case "fish":
		return "(fish reloads completions automatically)"
	case "powershell":
		return ". $PROFILE"
	default:
		return "reload your shell"
	}
}

func init() {
	rootCmd.AddCommand(completionCmd)
}
