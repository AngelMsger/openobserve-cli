package app

import "github.com/spf13/cobra"

// enumComplete registers static shell-completion candidates for a flag.
func enumComplete(cmd *cobra.Command, flag string, values ...string) {
	_ = cmd.RegisterFlagCompletionFunc(flag,
		func(*cobra.Command, []string, string) ([]string, cobra.ShellCompDirective) {
			return values, cobra.ShellCompDirectiveNoFileComp
		})
}
