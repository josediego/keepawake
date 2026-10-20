package cmd

import (
	"bytes"
	"testing"

	"github.com/jose/keepawake/internal/keepawake"
	"github.com/spf13/cobra"
)

func TestModeFlags(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want keepawake.Mode
	}{
		{name: "none", want: keepawake.Mode{}},
		{name: "display", args: []string{"-d"}, want: keepawake.Mode{Display: true}},
		{name: "idle", args: []string{"-i"}, want: keepawake.Mode{Idle: true}},
		{name: "system", args: []string{"-s"}, want: keepawake.Mode{System: true}},
		{name: "disk", args: []string{"-m"}, want: keepawake.Mode{Disk: true}},
		{name: "long display", args: []string{"--display"}, want: keepawake.Mode{Display: true}},
		{name: "long idle", args: []string{"--idle"}, want: keepawake.Mode{Idle: true}},
		{name: "long system", args: []string{"--system"}, want: keepawake.Mode{System: true}},
		{name: "long disk", args: []string{"--disk"}, want: keepawake.Mode{Disk: true}},
		{name: "idle and system", args: []string{"-is"}, want: keepawake.Mode{Idle: true, System: true}},
		{name: "all", args: []string{"-dism"}, want: keepawake.Mode{Display: true, Idle: true, System: true, Disk: true}},
		{name: "allow display sleep", args: []string{"--allow-display-sleep"}, want: keepawake.Mode{Idle: true}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c := &cobra.Command{Use: "probe", RunE: func(*cobra.Command, []string) error { return nil }}
			modes := addModeFlags(c)
			c.SetArgs(append([]string{}, tc.args...))

			if err := c.Execute(); err != nil {
				t.Fatalf("Execute(%q) error = %v", tc.args, err)
			}
			if got := modes.mode(); got != tc.want {
				t.Errorf("mode() = %+v, want %+v", got, tc.want)
			}
		})
	}
}

// recordingSessionDeps builds sessionDeps whose manager records each Mode it is given in *modes.
func recordingSessionDeps(modes *[]keepawake.Mode, launches *int) sessionDeps {
	deps := fakeSessionDeps(launches)
	deps.manager = func(m keepawake.Mode) (sessionManager, error) {
		*modes = append(*modes, m)
		return fakeSession{}, nil
	}
	return deps
}

func TestModeFlagsAllowDisplaySleepExclusive(t *testing.T) {
	commands := map[string][]string{
		"for":   {"for", "45m"},
		"until": {"until", "17:00", "--yes"},
	}
	for cmdName, cmdArgs := range commands {
		for _, flag := range []string{"-d", "-i", "-s", "-m"} {
			t.Run(cmdName+" "+flag, func(t *testing.T) {
				t.Setenv(noTrayEnv, "")
				var modes []keepawake.Mode
				launches := 0
				root := newRootCommandWith("test", recordingSessionDeps(&modes, &launches))
				var out, errOut bytes.Buffer
				root.SetOut(&out)
				root.SetErr(&errOut)
				root.SetArgs(append(append([]string{}, cmdArgs...), "--allow-display-sleep", flag))

				if err := root.Execute(); err == nil {
					t.Error("Execute() error = nil, want an error")
				}
				if len(modes) != 0 {
					t.Errorf("manager built %d times, want 0", len(modes))
				}
				if launches != 0 {
					t.Errorf("launcher ran %d times, want 0", launches)
				}
				if out.Len() != 0 {
					t.Errorf("stdout = %q, want empty", out.String())
				}
			})
		}
	}
}

func TestSessionCommandsModeFlags(t *testing.T) {
	commands := map[string]func(sessionDeps) *cobra.Command{
		"for":   newForCommand,
		"until": newUntilCommand,
	}
	flags := []struct{ name, shorthand string }{
		{"display", "d"},
		{"idle", "i"},
		{"system", "s"},
		{"disk", "m"},
		{"allow-display-sleep", ""},
	}

	for cmdName, newCmd := range commands {
		launches := 0
		c := newCmd(fakeSessionDeps(&launches))
		for _, f := range flags {
			flag := c.Flags().Lookup(f.name)
			if flag == nil {
				t.Errorf("%s has no --%s flag", cmdName, f.name)
				continue
			}
			if flag.Shorthand != f.shorthand {
				t.Errorf("%s --%s shorthand = %q, want %q", cmdName, f.name, flag.Shorthand, f.shorthand)
			}
		}
	}
}

func TestSessionCommandsPassModeToManager(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want keepawake.Mode
	}{
		{name: "for default", args: []string{"for", "45m"}, want: keepawake.Mode{}},
		{name: "for idle", args: []string{"for", "-i", "45m"}, want: keepawake.Mode{Idle: true}},
		{name: "until system", args: []string{"until", "-s", "17:00", "--yes"}, want: keepawake.Mode{System: true}},
		{name: "until allow display sleep", args: []string{"until", "--allow-display-sleep", "17:00", "--yes"}, want: keepawake.Mode{Idle: true}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(noTrayEnv, "1")
			var modes []keepawake.Mode
			launches := 0
			root := newRootCommandWith("test", recordingSessionDeps(&modes, &launches))
			var out, errOut bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&errOut)
			root.SetArgs(tc.args)

			if err := root.Execute(); err != nil {
				t.Fatalf("Execute(%q) error = %v", tc.args, err)
			}
			if len(modes) != 1 || modes[0] != tc.want {
				t.Errorf("manager modes = %+v, want [%+v]", modes, tc.want)
			}
		})
	}
}

func TestHelpAndVersionStillWork(t *testing.T) {
	for _, args := range [][]string{
		{"until", "--help"},
		{"until", "-h"},
		{"for", "--help"},
		{"for", "-h"},
		{"status", "--help"},
		{"--version"},
		{"-v"},
	} {
		launches := 0
		root := newRootCommandWith("test", fakeSessionDeps(&launches))
		var out, errOut bytes.Buffer
		root.SetOut(&out)
		root.SetErr(&errOut)
		root.SetArgs(args)

		if err := root.Execute(); err != nil {
			t.Errorf("Execute(%q) error = %v", args, err)
		}
		if out.Len() == 0 {
			t.Errorf("Execute(%q) printed nothing", args)
		}
		if launches != 0 {
			t.Errorf("Execute(%q) launched the tray %d times", args, launches)
		}
	}
}
