package cmd

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jose/keepawake/internal/keepawake"
	"github.com/spf13/cobra"
)

func newForCommand(deps sessionDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "for <duration>",
		Short: "Keep your Mac awake for the provided duration",
		Long: `Use Go durations (e.g., "45m", "1h30m", "2h") or a bare number of minutes ("90" = 90m).
Several arguments are joined, so "keepawake for 1h 30m" works.
The duration must be between 1m and 24h.
The command delegates to macOS caffeinate under the hood.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			d, err := durationFromArgs(args)
			if err != nil {
				return err
			}

			manager, err := deps.manager()
			if err != nil {
				return err
			}

			pid, end, err := manager.For(d)
			if err != nil {
				return err
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Keepawake active (PID %d) for %s (until %s)\n", pid, humanDuration(d), formatUntil(time.Now(), end))
			if err := deps.launchTray(); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "Warning: could not launch tray: %v\n", err)
			}
			return nil
		},
		Example: `  keepawake for 45m
  keepawake for 1h30m
  keepawake for 1h 30m
  keepawake for 90`,
	}

	return cmd
}

// durationFromArgs joins args with no separator and parses the result as a duration.
func durationFromArgs(args []string) (time.Duration, error) {
	return parseDurationArg(strings.Join(args, ""))
}

// parseDurationArg parses a Go duration or a whole number of minutes within the allowed range.
func parseDurationArg(arg string) (time.Duration, error) {
	s := strings.ToLower(strings.TrimSpace(arg))
	d, err := time.ParseDuration(s)
	if err != nil {
		n, convErr := strconv.Atoi(s)
		if convErr != nil {
			return 0, fmt.Errorf("invalid duration %q: use a Go duration like 45m or 1h30m, or minutes like 90", arg)
		}
		// Check the range in minutes first so a huge n cannot overflow the multiplication.
		if time.Duration(n) < keepawake.MinDuration/time.Minute || time.Duration(n) > keepawake.MaxDuration/time.Minute {
			return 0, fmt.Errorf("duration %q out of range (%s to %s)", arg, keepawake.MinDuration, keepawake.MaxDuration)
		}
		d = time.Duration(n) * time.Minute
	}

	if d < keepawake.MinDuration || d > keepawake.MaxDuration {
		return 0, fmt.Errorf("duration %q out of range (%s to %s)", arg, keepawake.MinDuration, keepawake.MaxDuration)
	}
	return d, nil
}
