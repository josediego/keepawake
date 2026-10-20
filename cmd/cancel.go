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
			manager, err := deps.manager(keepawake.Mode{})
			if err != nil {
				return err
			}

			pid, err := manager.Cancel()
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Keepawake cancelled (PID %d)\n", pid)

			if err := deps.launchTray(); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not launch tray: %v\n", err)
			}
			return nil
		},
	}
}
