package cmd

import (
	"fmt"
	"os"
	"strconv"

	"github.com/spf13/cobra"
)

// noTrayEnv skips launching the tray when it parses as true, like --no-tray.
const noTrayEnv = "KEEPAWAKE_NO_TRAY"

// Execute runs the CLI root command, reporting version for --version.
func Execute(version string) error {
	return newRootCommand(version).Execute()
}

func newRootCommand(version string) *cobra.Command {
	return newRootCommandWith(version, defaultSessionDeps())
}

func newRootCommandWith(version string, session sessionDeps) *cobra.Command {
	root := &cobra.Command{
		Use:           "keepawake",
		Short:         "Prevent your Mac from sleeping until a specific time",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	root.PersistentFlags().Bool("no-tray", false, "do not launch the menu bar tray when a session starts (also "+noTrayEnv+"=1)")

	root.AddCommand(
		newUntilCommand(session),
		newForCommand(session),
		newCancelCommand(session),
		newTrayCommand(defaultTrayDeps()),
		newStatusCommand(),
	)

	return root
}

// startTray runs launch unless --no-tray or KEEPAWAKE_NO_TRAY says not to, warning through cmd if it fails.
func startTray(cmd *cobra.Command, launch func() error) {
	// A command built outside the root has no such flag, which means launch.
	if noTray, _ := cmd.Flags().GetBool("no-tray"); noTray {
		return
	}
	if skip, err := strconv.ParseBool(os.Getenv(noTrayEnv)); err == nil && skip {
		return
	}
	if err := launch(); err != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not launch tray: %v\n", err)
	}
}
