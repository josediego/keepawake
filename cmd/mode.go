package cmd

import (
	"github.com/jose/keepawake/internal/keepawake"
	"github.com/spf13/cobra"
)

// modeFlags holds the caffeinate mode flags shared by until and for.
type modeFlags struct {
	display, idle, system, disk bool
	allowDisplaySleep           bool
}

// addModeFlags adds the mode flags to c; --allow-display-sleep excludes the other four.
func addModeFlags(c *cobra.Command) *modeFlags {
	f := &modeFlags{}
	c.Flags().BoolVarP(&f.display, "display", "d", false, "prevent the display from sleeping (caffeinate -d)")
	c.Flags().BoolVarP(&f.idle, "idle", "i", false, "prevent the system from idle sleeping (caffeinate -i)")
	c.Flags().BoolVarP(&f.system, "system", "s", false, "prevent the system from sleeping, on AC power only (caffeinate -s)")
	c.Flags().BoolVarP(&f.disk, "disk", "m", false, "prevent the disk from idle sleeping (caffeinate -m)")
	c.Flags().BoolVar(&f.allowDisplaySleep, "allow-display-sleep", false, "let the display sleep but keep the system awake (same as --idle)")
	for _, name := range []string{"display", "idle", "system", "disk"} {
		c.MarkFlagsMutuallyExclusive("allow-display-sleep", name)
	}
	return f
}

// mode returns the Mode the flags select; with none given, the zero Mode (the default).
func (f *modeFlags) mode() keepawake.Mode {
	if f.allowDisplaySleep {
		return keepawake.Mode{Idle: true}
	}
	return keepawake.Mode{Display: f.display, Idle: f.idle, System: f.system, Disk: f.disk}
}
