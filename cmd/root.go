package cmd

import (
	"github.com/spf13/cobra"
)

// Execute runs the CLI root command, reporting version for --version.
func Execute(version string) error {
	return newRootCommand(version).Execute()
}

func newRootCommand(version string) *cobra.Command {
	root := &cobra.Command{
		Use:           "keepawake",
		Short:         "Prevent your Mac from sleeping until a specific time",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.AddCommand(
		newUntilCommand(),
		newForCommand(),
		newCancelCommand(),
		newTrayCommand(),
		newStatusCommand(),
	)

	return root
}
