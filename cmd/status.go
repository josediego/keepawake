package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jose/keepawake/internal/keepawake"
	"github.com/spf13/cobra"
)

// ErrInactive is returned by `status --quiet` when no session is active.
var ErrInactive = errors.New("keepawake is inactive")

// statusDeps holds what the status command reads, so tests can fake them.
type statusDeps struct {
	currentState func() (*keepawake.State, error)
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
		now: time.Now,
	})
}

func newStatusCommandWith(deps statusDeps) *cobra.Command {
	var asJSON, quiet bool

	c := &cobra.Command{
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

			if quiet {
				if state == nil {
					return ErrInactive
				}
				return nil
			}

			if asJSON {
				out, err := statusJSON(state, deps.now())
				if err != nil {
					return err
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(out))
				return nil
			}

			fmt.Fprintln(cmd.OutOrStdout(), formatStatus(state, deps.now()))
			return nil
		},
	}

	c.Flags().BoolVar(&asJSON, "json", false, "print status as a JSON object")
	c.Flags().BoolVarP(&quiet, "quiet", "q", false, "print nothing; exit 1 if inactive")
	c.MarkFlagsMutuallyExclusive("json", "quiet")

	return c
}

// activeStatus is the JSON form of an active session; no omitempty, so a 0 is kept.
type activeStatus struct {
	Active           bool   `json:"active"`
	PID              int    `json:"pid"`
	Until            string `json:"until"`
	RemainingSeconds int64  `json:"remaining_seconds"`
	Mode             string `json:"mode"`
}

// inactiveStatus is the JSON form when no session is active.
type inactiveStatus struct {
	Active bool `json:"active"`
}

// statusJSON encodes state as seen at now for scripts, showing the end time in now's location.
func statusJSON(state *keepawake.State, now time.Time) ([]byte, error) {
	if state == nil {
		return json.Marshal(inactiveStatus{})
	}

	until := state.Until.In(now.Location())
	remaining := max(until.Sub(now), 0)

	return json.Marshal(activeStatus{
		Active:           true,
		PID:              state.PID,
		Until:            until.Format(time.RFC3339),
		RemainingSeconds: int64(remaining / time.Second),
		Mode:             statusMode(state.Mode).String(),
	})
}

// formatStatus describes state as seen at now, showing the end time in now's location.
func formatStatus(state *keepawake.State, now time.Time) string {
	if state == nil {
		return "Inactive"
	}

	until := state.Until.In(now.Location())
	remaining := max(until.Sub(now), 0)

	s := fmt.Sprintf("Active until %s (%s left, PID %d",
		formatUntil(now, until),
		humanDuration(remaining),
		state.PID)
	if mode := statusMode(state.Mode); mode != keepawake.DefaultMode {
		s += ", mode " + mode.String()
	}
	return s + ")"
}

// statusMode returns the mode to show for m: the zero Mode, as in a State built by hand, is DefaultMode.
func statusMode(m keepawake.Mode) keepawake.Mode {
	if m == (keepawake.Mode{}) {
		return keepawake.DefaultMode
	}
	return m
}
