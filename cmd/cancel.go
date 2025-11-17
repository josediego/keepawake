package cmd

import (
	"github.com/jose/keepawake/internal/keepawake"
	"github.com/jose/keepawake/internal/tray"
	"github.com/spf13/cobra"
)

func newCancelCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "cancel",
		Short: "Stop the active keepawake session",
		RunE: func(cmd *cobra.Command, _ []string) error {
			manager, err := keepawake.NewManager()
			if err != nil {
				return err
			}

			pid, err := manager.Cancel()
			if err != nil {
				return err
			}

			cmd.Printf("Keepawake cancelled (PID %d)\n", pid)

			if err := tray.EnsureRunning(); err != nil {
				cmd.Printf("Warning: could not launch tray: %v\n", err)
			}
			return nil
		},
	}
}
