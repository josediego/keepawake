package cmd

import (
	"fmt"
	"strings"
	"time"

	"github.com/jose/keepawake/internal/keepawake"
	"github.com/jose/keepawake/internal/tray"
	"github.com/spf13/cobra"
)

func newUntilCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "until <time>",
		Short: "Keep your Mac awake until the provided time",
		Long: `Use 24-hour or 12-hour formats (e.g., "17:30", "5pm", "5:30pm").
The command delegates to macOS caffeinate under the hood.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := keepawake.NewManager()
			if err != nil {
				return err
			}

			target := strings.Join(args, " ")
			pid, duration, err := manager.Until(target)
			if err != nil {
				return err
			}

			cmd.Printf("Keepawake active (PID %d) for %s\n", pid, humanDuration(duration))
			if err := tray.EnsureRunning(); err != nil {
				cmd.Printf("Warning: could not launch tray: %v\n", err)
			}
			return nil
		},
		Example: `  keepawake until 17:00
  keepawake until "5pm"
  keepawake until "5:30 pm"`,
	}

	return cmd
}

func humanDuration(d time.Duration) string {
	if d < time.Minute {
		return d.Round(time.Second).String()
	}

	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60

	switch {
	case hours > 0 && minutes > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	case hours > 0:
		return fmt.Sprintf("%dh", hours)
	default:
		return fmt.Sprintf("%dm", minutes)
	}
}
