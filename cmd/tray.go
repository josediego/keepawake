package cmd

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func newTrayCommand(deps trayDeps) *cobra.Command {
	return &cobra.Command{
		Use:   "tray",
		Short: "Start a status bar icon to view the status of Keepawake",
		Long: `Starts a persistent process in the status bar (menubar) that shows if Keepawake
is active and until what time the system will stay awake. Quit the app from the icon's menu.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			app, err := deps.newApp()
			if err != nil {
				return err
			}

			cleanup, existingPID, alreadyRunning, err := deps.guard(os.Getpid())
			if err != nil {
				return err
			}
			if alreadyRunning {
				fmt.Fprintf(cmd.OutOrStdout(), "Keepawake tray already running (PID %d)\n", existingPID)
				return nil
			}
			if cleanup != nil {
				defer cleanup()
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Launching Keepawake in the menu bar. Use the icon to exit.")

			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return app.Run(ctx)
		},
	}
}
