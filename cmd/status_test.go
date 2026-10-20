package cmd

import (
	"bytes"
	"errors"
	"io"
	"os"
	"testing"
	"time"

	"github.com/jose/keepawake/internal/keepawake"
	"github.com/spf13/cobra"
)

// statusZone and statusNow pin the clock: Tuesday 10 March 2026, 16:25 at UTC-5.
var (
	statusZone = time.FixedZone("UTC-5", -5*3600)
	statusNow  = time.Date(2026, time.March, 10, 16, 25, 0, 0, statusZone)
)

func TestFormatStatus(t *testing.T) {
	tests := []struct {
		name  string
		state *keepawake.State
		want  string
	}{
		{
			name:  "active today",
			state: &keepawake.State{PID: 123, Until: time.Date(2026, time.March, 10, 17, 30, 0, 0, statusZone)},
			want:  "Active until 17:30 (1h 5m left, PID 123)",
		},
		{
			name:  "active tomorrow",
			state: &keepawake.State{PID: 123, Until: time.Date(2026, time.March, 11, 9, 0, 0, 0, statusZone)},
			want:  "Active until Wed 11 Mar 09:00 (16h 35m left, PID 123)",
		},
		{
			name:  "until in another zone",
			state: &keepawake.State{PID: 123, Until: time.Date(2026, time.March, 10, 22, 30, 0, 0, time.UTC)},
			want:  "Active until 17:30 (1h 5m left, PID 123)",
		},
		{
			name:  "tomorrow in UTC but today in now's zone",
			state: &keepawake.State{PID: 123, Until: time.Date(2026, time.March, 11, 2, 0, 0, 0, time.UTC)},
			want:  "Active until 21:00 (4h 35m left, PID 123)",
		},
		{
			name:  "until already passed",
			state: &keepawake.State{PID: 123, Until: time.Date(2026, time.March, 10, 15, 0, 0, 0, statusZone)},
			want:  "Active until 15:00 (0s left, PID 123)",
		},
		{
			name:  "inactive",
			state: nil,
			want:  "Inactive",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := formatStatus(tc.state, statusNow); got != tc.want {
				t.Errorf("formatStatus() = %q, want %q", got, tc.want)
			}
		})
	}
}

// fakeStatusCommand builds the status command from fakes, counting launcher runs in *launches.
func fakeStatusCommand(state *keepawake.State, stateErr, launchErr error, launches *int) *cobra.Command {
	cmd := newStatusCommandWith(statusDeps{
		currentState: func() (*keepawake.State, error) { return state, stateErr },
		launchTray: func() error {
			*launches++
			return launchErr
		},
		now: func() time.Time { return statusNow },
	})
	cmd.SetArgs([]string{})
	return cmd
}

func TestStatusCommand(t *testing.T) {
	active := &keepawake.State{PID: 123, Until: time.Date(2026, time.March, 10, 17, 30, 0, 0, statusZone)}
	readErr := errors.New("permission denied")

	tests := []struct {
		name         string
		state        *keepawake.State
		stateErr     error
		launchErr    error
		wantOut      string
		wantErrOut   string
		wantErr      error
		wantLaunches int
	}{
		{
			name:         "active",
			state:        active,
			launchErr:    errors.New("boom"),
			wantOut:      "Active until 17:30 (1h 5m left, PID 123)\n",
			wantErrOut:   "Warning: could not launch tray: boom\n",
			wantLaunches: 1,
		},
		{
			name:     "inactive",
			stateErr: keepawake.ErrStateNotFound,
			wantOut:  "Inactive\n",
		},
		{
			name:     "inactive with wrapped not found",
			stateErr: errors.Join(errors.New("cleanup"), keepawake.ErrStateNotFound),
			wantOut:  "Inactive\n",
		},
		{
			name:     "read error",
			stateErr: readErr,
			wantErr:  readErr,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			launches := 0
			cmd := fakeStatusCommand(tc.state, tc.stateErr, tc.launchErr, &launches)
			cmd.SetOut(&out)
			cmd.SetErr(&errOut)

			if err := cmd.Execute(); err != tc.wantErr {
				t.Errorf("Execute() error = %v, want %v", err, tc.wantErr)
			}
			if got := out.String(); got != tc.wantOut {
				t.Errorf("stdout = %q, want %q", got, tc.wantOut)
			}
			if got := errOut.String(); got != tc.wantErrOut {
				t.Errorf("stderr = %q, want %q", got, tc.wantErrOut)
			}
			if launches != tc.wantLaunches {
				t.Errorf("launcher ran %d times, want %d", launches, tc.wantLaunches)
			}
		})
	}
}

func TestStatusCommandWritesToRealStreams(t *testing.T) {
	stdoutR, stdoutW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderrR, stderrW, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}

	origStdout, origStderr := os.Stdout, os.Stderr
	restore := func() { os.Stdout, os.Stderr = origStdout, origStderr }
	t.Cleanup(restore)
	os.Stdout, os.Stderr = stdoutW, stderrW

	launches := 0
	active := &keepawake.State{PID: 123, Until: time.Date(2026, time.March, 10, 17, 30, 0, 0, statusZone)}
	execErr := fakeStatusCommand(active, nil, errors.New("boom"), &launches).Execute()

	restore()
	stdoutW.Close()
	stderrW.Close()

	if execErr != nil {
		t.Fatalf("Execute() error = %v", execErr)
	}

	gotOut, err := io.ReadAll(stdoutR)
	if err != nil {
		t.Fatal(err)
	}
	gotErr, err := io.ReadAll(stderrR)
	if err != nil {
		t.Fatal(err)
	}

	if want := "Active until 17:30 (1h 5m left, PID 123)\n"; string(gotOut) != want {
		t.Errorf("stdout = %q, want %q", gotOut, want)
	}
	if want := "Warning: could not launch tray: boom\n"; string(gotErr) != want {
		t.Errorf("stderr = %q, want %q", gotErr, want)
	}
}
