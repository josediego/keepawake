package cmd

import (
	"time"

	"github.com/jose/keepawake/internal/keepawake"
	"github.com/jose/keepawake/internal/tray"
	"github.com/spf13/cobra"
)

func newStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show the current keepawake status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			manager, err := keepawake.NewManager()
			if err != nil {
				return err
			}

			state, err := manager.CurrentState()
			if err != nil {
				if err == keepawake.ErrStateNotFound {
					cmd.Println("No keepawake session is active.")
					return nil
				}
				return err
			}

			remaining := time.Until(state.Until)
			if remaining < 0 {
				remaining = 0
			}

			cmd.Printf("Keepawake active (PID %d) for %s\n",
				state.PID,
				humanDuration(remaining))

			if err := tray.EnsureRunning(); err != nil {
				cmd.Printf("Warning: could not launch tray: %v\n", err)
			}

			return nil
		},
	}
}
