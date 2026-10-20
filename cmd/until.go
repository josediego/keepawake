package cmd

import (
	"strings"

	"github.com/jose/keepawake/internal/keepawake"
	"github.com/jose/keepawake/internal/tray"
	"github.com/spf13/cobra"
)

func newUntilCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "until <time>",
		Short: "Keep your Mac awake until the provided time",
		Long: `Use 24-hour or 12-hour formats (e.g., "17:30", "5pm", "5:30 pm").
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
