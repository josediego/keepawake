package cmd

import (
	"bytes"
	"errors"
	"testing"

	"github.com/spf13/cobra"
)

// probeRoot builds the root command with a probe subcommand that calls startTray with launch.
func probeRoot(launch func() error) *cobra.Command {
	unused := 0
	root := newRootCommandWith("test", fakeSessionDeps(&unused))
	root.AddCommand(&cobra.Command{
		Use: "probe",
		RunE: func(c *cobra.Command, _ []string) error {
			startTray(c, launch)
			return nil
		},
	})
	return root
}

func TestStartTray(t *testing.T) {
	tests := []struct {
		name         string
		args         []string
		env          string
		wantLaunches int
	}{
		{name: "default", args: []string{"probe"}, wantLaunches: 1},
		{name: "env 0", args: []string{"probe"}, env: "0", wantLaunches: 1},
		{name: "env false", args: []string{"probe"}, env: "false", wantLaunches: 1},
		{name: "env junk", args: []string{"probe"}, env: "maybe", wantLaunches: 1},
		{name: "env 1", args: []string{"probe"}, env: "1"},
		{name: "env true", args: []string{"probe"}, env: "true"},
		{name: "flag before subcommand", args: []string{"--no-tray", "probe"}},
		{name: "flag after subcommand", args: []string{"probe", "--no-tray"}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(noTrayEnv, tc.env)
			launches := 0
			root := probeRoot(func() error {
				launches++
				return nil
			})
			var out, errOut bytes.Buffer
			root.SetOut(&out)
			root.SetErr(&errOut)
			root.SetArgs(tc.args)

			if err := root.Execute(); err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if launches != tc.wantLaunches {
				t.Errorf("launcher ran %d times, want %d", launches, tc.wantLaunches)
			}
			if got := errOut.String(); got != "" {
				t.Errorf("stderr = %q, want empty", got)
			}
		})
	}
}

func TestStartTrayWarnsWhenLaunchFails(t *testing.T) {
	t.Setenv(noTrayEnv, "")
	root := probeRoot(func() error { return errors.New("boom") })
	var out, errOut bytes.Buffer
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs([]string{"probe"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute() error = %v", err)
	}
	if got := out.String(); got != "" {
		t.Errorf("stdout = %q, want empty", got)
	}
	if want := "Warning: could not launch tray: boom\n"; errOut.String() != want {
		t.Errorf("stderr = %q, want %q", errOut.String(), want)
	}
}

func TestSessionCommandsHonourNoTray(t *testing.T) {
	commands := map[string][]string{
		"for":   {"for", "45m"},
		"until": {"until", "17:00", "--yes"},
	}
	tests := []struct {
		name         string
		before       []string
		after        []string
		env          string
		wantLaunches int
		wantErrOut   string
	}{
		{name: "default", wantLaunches: 1, wantErrOut: "Warning: could not launch tray: boom\n"},
		{name: "flag before", before: []string{"--no-tray"}},
		{name: "flag after", after: []string{"--no-tray"}},
		{name: "env 1", env: "1"},
	}

	for cmdName, cmdArgs := range commands {
		for _, tc := range tests {
			t.Run(cmdName+" "+tc.name, func(t *testing.T) {
				t.Setenv(noTrayEnv, tc.env)
				launches := 0
				root := newRootCommandWith("test", fakeSessionDeps(&launches))
				var out, errOut bytes.Buffer
				root.SetOut(&out)
				root.SetErr(&errOut)
				args := append(append(append([]string{}, tc.before...), cmdArgs...), tc.after...)
				root.SetArgs(args)

				if err := root.Execute(); err != nil {
					t.Fatalf("Execute(%q) error = %v", args, err)
				}
				if launches != tc.wantLaunches {
					t.Errorf("launcher ran %d times, want %d", launches, tc.wantLaunches)
				}
				if got := errOut.String(); got != tc.wantErrOut {
					t.Errorf("stderr = %q, want %q", got, tc.wantErrOut)
				}
			})
		}
	}
}
