package cmd

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/jose/keepawake/internal/keepawake"
	"github.com/spf13/cobra"
)

// untilStarter starts a session until a given time; *keepawake.Manager satisfies it.
type untilStarter interface {
	StartUntil(time.Time) (int, time.Duration, error)
}

// untilDeps holds what runUntil needs from the outside world.
type untilDeps struct {
	now        func() time.Time
	starter    untilStarter
	in         io.Reader
	out        io.Writer
	isTerminal func() bool
}

func newUntilCommand(deps sessionDeps) *cobra.Command {
	var yes bool

	cmd := &cobra.Command{
		Use:   "until <time>",
		Short: "Keep your Mac awake until the provided time",
		Long: `Use 24-hour or 12-hour formats (e.g., "17:30", "5pm", "5:30 pm").
A time that already passed today means tomorrow, after confirmation (or --yes).
The command delegates to macOS caffeinate under the hood.`,
		Args: cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			manager, err := deps.manager()
			if err != nil {
				return err
			}

			ud := untilDeps{
				now:        time.Now,
				starter:    manager,
				in:         cmd.InOrStdin(),
				out:        cmd.OutOrStdout(),
				isTerminal: func() bool { return isTerminal(os.Stdin) },
			}
			if err := runUntil(ud, strings.Join(args, " "), yes); err != nil {
				return err
			}

			startTray(cmd, deps.launchTray)
			return nil
		},
		Example: `  keepawake until 17:00
  keepawake until "5pm"
  keepawake until "5:30 pm"
  keepawake until 08:00 --yes`,
	}

	cmd.Flags().BoolVarP(&yes, "yes", "y", false, "roll a passed time over to tomorrow without asking")

	return cmd
}

// runUntil resolves target, asks before rolling a passed time over to
// tomorrow, and only then starts the session.
func runUntil(d untilDeps, target string, yes bool) error {
	now := d.now()
	t, rolledOver, err := keepawake.ResolveTarget(now, target)
	if err != nil {
		return err
	}

	if rolledOver && !yes {
		clock := t.Format("15:04")
		if !d.isTerminal() {
			return fmt.Errorf("%s already passed today; re-run with --yes to keep awake until tomorrow", clock)
		}
		fmt.Fprintf(d.out, "%s already passed today. Keep awake until tomorrow %s (%s)? [y/N] ", clock, clock, t.Format("Mon 2 Jan"))
		// On end of input, line holds whatever was read, usually nothing.
		line, _ := bufio.NewReader(d.in).ReadString('\n')
		if answer := strings.ToLower(strings.TrimSpace(line)); answer != "y" && answer != "yes" {
			return errors.New("aborted: nothing was changed")
		}
	}

	pid, duration, err := d.starter.StartUntil(t)
	if err != nil {
		return err
	}

	fmt.Fprintf(d.out, "Keepawake active (PID %d) for %s (until %s)\n", pid, humanDuration(duration), formatUntil(now, t))
	return nil
}
