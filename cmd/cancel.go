package cmd

import (
	"fmt"

	"github.com/jose/keepawake/internal/keepawake"
	"github.com/spf13/cobra"
)

func newCancelCommand(deps sessionDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "cancel",
		Short: "Stop the active keepawake session",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cancel := func() (int, error) {
				manager, err := deps.manager(keepawake.Mode{})
				if err != nil {
					return 0, err
				}
				return manager.Cancel()
			}
			return runCancel(cmd, cancel, deps.stopTray)
		},
	}
}

// runCancel stops the session, then closes the tray; a tray failure is only a warning.
func runCancel(cmd *cobra.Command, cancel func() (int, error), stopTray func() error) error {
	pid, err := cancel()
	if err == nil {
		fmt.Fprintf(cmd.OutOrStdout(), "Keepawake cancelled (PID %d)\n", pid)
	}

	if trayErr := stopTray(); trayErr != nil {
		fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not close tray: %v\n", trayErr)
	}
	return err
}
