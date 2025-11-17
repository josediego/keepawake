package cmd

import (
	"github.com/spf13/cobra"
)

// Execute runs the CLI root command.
func Execute() error {
	return newRootCommand().Execute()
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "keepawake",
		Short:         "Prevent your Mac from sleeping until a specific time",
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(
		newUntilCommand(),
		newCancelCommand(),
		newTrayCommand(),
		newStatusCommand(),
	)

	return root
}
