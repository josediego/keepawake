package cmd

import (
	"context"
	"os"

	"github.com/jose/keepawake/internal/keepawake"
	"github.com/jose/keepawake/internal/tray"
	"github.com/spf13/cobra"
)

func newTrayCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "tray",
		Short: "Start a status bar icon to view the status of Keepawake",
		Long: `Starts a persistent process in the status bar (menubar) that shows if Keepawake
is active and until what time the system will stay awake. Quit the app from the icon's menu.`,
		RunE: func(cmd *cobra.Command, _ []string) error {
			manager, err := keepawake.NewManager()
			if err != nil {
				return err
			}

			cleanup, existingPID, alreadyRunning, err := tray.ForegroundPIDGuard(os.Getpid())
			if err != nil {
				return err
			}
			if alreadyRunning {
				cmd.Printf("Keepawake tray already running (PID %d)\n", existingPID)
				return nil
			}
			if cleanup != nil {
				defer cleanup()
			}

			app := tray.NewApp(manager)
			cmd.Println("Launching Keepawake in the menu bar. Use the icon to exit.")

			ctx := cmd.Context()
			if ctx == nil {
				ctx = context.Background()
			}
			return app.Run(ctx)
		},
	}
}
