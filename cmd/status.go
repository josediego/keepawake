package cmd

import (
	"errors"
	"fmt"
	"time"

	"github.com/jose/keepawake/internal/keepawake"
	"github.com/jose/keepawake/internal/tray"
	"github.com/spf13/cobra"
)

// statusDeps holds what the status command reads and launches, so tests can fake them.
type statusDeps struct {
	currentState func() (*keepawake.State, error)
	launchTray   func() error
	now          func() time.Time
}

func newStatusCommand() *cobra.Command {
	return newStatusCommandWith(statusDeps{
		currentState: func() (*keepawake.State, error) {
			manager, err := keepawake.NewManager()
			if err != nil {
				return nil, err
			}
			return manager.CurrentState()
		},
		launchTray: tray.EnsureRunning,
		now:        time.Now,
	})
}

func newStatusCommandWith(deps statusDeps) *cobra.Command {
	return &cobra.Command{
		Use:           "status",
		Short:         "Show the current keepawake status",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			state, err := deps.currentState()
			if errors.Is(err, keepawake.ErrStateNotFound) {
				state = nil
			} else if err != nil {
				return err
			}

			fmt.Fprintln(cmd.OutOrStdout(), formatStatus(state, deps.now()))

			if state != nil {
				if err := deps.launchTray(); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not launch tray: %v\n", err)
				}
			}

			return nil
		},
	}
}

// formatStatus describes state as seen at now, showing the end time in now's location.
func formatStatus(state *keepawake.State, now time.Time) string {
	if state == nil {
		return "Inactive"
	}

	until := state.Until.In(now.Location())
	remaining := max(until.Sub(now), 0)

	return fmt.Sprintf("Active until %s (%s left, PID %d)",
		formatUntil(now, until),
		humanDuration(remaining),
		state.PID)
}
