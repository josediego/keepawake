package cmd

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
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
			name:  "default mode",
			state: &keepawake.State{PID: 123, Until: time.Date(2026, time.March, 10, 17, 30, 0, 0, statusZone), Mode: keepawake.DefaultMode},
			want:  "Active until 17:30 (1h 5m left, PID 123)",
		},
		{
			name:  "idle mode",
			state: &keepawake.State{PID: 7, Until: time.Date(2026, time.March, 10, 17, 30, 0, 0, statusZone), Mode: keepawake.Mode{Idle: true}},
			want:  "Active until 17:30 (1h 5m left, PID 7, mode i)",
		},
		{
			name:  "all modes",
			state: &keepawake.State{PID: 123, Until: time.Date(2026, time.March, 10, 17, 30, 0, 0, statusZone), Mode: keepawake.Mode{Display: true, Idle: true, System: true, Disk: true}},
			want:  "Active until 17:30 (1h 5m left, PID 123, mode dism)",
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

func TestStatusJSON(t *testing.T) {
	tests := []struct {
		name  string
		state *keepawake.State
		want  map[string]any
	}{
		{
			name:  "active",
			state: &keepawake.State{PID: 123, Until: time.Date(2026, time.March, 10, 22, 30, 0, 0, time.UTC)},
			want: map[string]any{
				"active":            true,
				"pid":               float64(123),
				"until":             "2026-03-10T17:30:00-05:00",
				"remaining_seconds": float64(3900),
				"mode":              "di",
			},
		},
		{
			name:  "until already passed",
			state: &keepawake.State{PID: 123, Until: time.Date(2026, time.March, 10, 15, 0, 0, 0, statusZone)},
			want: map[string]any{
				"active":            true,
				"pid":               float64(123),
				"until":             "2026-03-10T15:00:00-05:00",
				"remaining_seconds": float64(0),
				"mode":              "di",
			},
		},
		{
			name:  "default mode",
			state: &keepawake.State{PID: 123, Until: time.Date(2026, time.March, 10, 17, 30, 0, 0, statusZone), Mode: keepawake.DefaultMode},
			want: map[string]any{
				"active":            true,
				"pid":               float64(123),
				"until":             "2026-03-10T17:30:00-05:00",
				"remaining_seconds": float64(3900),
				"mode":              "di",
			},
		},
		{
			name:  "idle and system mode",
			state: &keepawake.State{PID: 123, Until: time.Date(2026, time.March, 10, 17, 30, 0, 0, statusZone), Mode: keepawake.Mode{Idle: true, System: true}},
			want: map[string]any{
				"active":            true,
				"pid":               float64(123),
				"until":             "2026-03-10T17:30:00-05:00",
				"remaining_seconds": float64(3900),
				"mode":              "is",
			},
		},
		{
			name:  "inactive",
			state: nil,
			want:  map[string]any{"active": false},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			out, err := statusJSON(tc.state, statusNow)
			if err != nil {
				t.Fatalf("statusJSON() error = %v", err)
			}
			var got map[string]any
			if err := json.Unmarshal(out, &got); err != nil {
				t.Fatalf("json.Unmarshal(%s): %v", out, err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("statusJSON() = %v, want %v", got, tc.want)
			}
		})
	}
}

// fakeStatusCommand builds the status command from fakes; it has no launcher, so it cannot start the tray.
func fakeStatusCommand(state *keepawake.State, stateErr error) *cobra.Command {
	cmd := newStatusCommandWith(statusDeps{
		currentState: func() (*keepawake.State, error) { return state, stateErr },
		now:          func() time.Time { return statusNow },
	})
	cmd.SetArgs([]string{})
	return cmd
}

func TestStatusCommand(t *testing.T) {
	active := &keepawake.State{PID: 123, Until: time.Date(2026, time.March, 10, 17, 30, 0, 0, statusZone)}
	readErr := errors.New("permission denied")

	tests := []struct {
		name     string
		args     []string
		state    *keepawake.State
		stateErr error
		wantOut  string
		wantErr  error
	}{
		{
			name:    "active",
			state:   active,
			wantOut: "Active until 17:30 (1h 5m left, PID 123)\n",
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
		{
			name:    "json active",
			args:    []string{"--json"},
			state:   active,
			wantOut: `{"active":true,"pid":123,"until":"2026-03-10T17:30:00-05:00","remaining_seconds":3900,"mode":"di"}` + "\n",
		},
		{
			name:     "json inactive",
			args:     []string{"--json"},
			stateErr: keepawake.ErrStateNotFound,
			wantOut:  `{"active":false}` + "\n",
		},
		{
			name:     "json read error",
			args:     []string{"--json"},
			stateErr: readErr,
			wantErr:  readErr,
		},
		{
			name:  "quiet active",
			args:  []string{"-q"},
			state: active,
		},
		{
			name:     "quiet inactive",
			args:     []string{"--quiet"},
			stateErr: keepawake.ErrStateNotFound,
			wantErr:  ErrInactive,
		},
		{
			name:     "quiet read error",
			args:     []string{"--quiet"},
			stateErr: readErr,
			wantErr:  readErr,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var out, errOut bytes.Buffer
			cmd := fakeStatusCommand(tc.state, tc.stateErr)
			if tc.args != nil {
				cmd.SetArgs(tc.args)
			}
			cmd.SetOut(&out)
			cmd.SetErr(&errOut)

			if err := cmd.Execute(); err != tc.wantErr {
				t.Errorf("Execute() error = %v, want %v", err, tc.wantErr)
			}
			if got := out.String(); got != tc.wantOut {
				t.Errorf("stdout = %q, want %q", got, tc.wantOut)
			}
			if got := errOut.String(); got != "" {
				t.Errorf("stderr = %q, want empty", got)
			}
		})
	}
}

func TestStatusCommandJSONAndQuietExclusive(t *testing.T) {
	var out, errOut bytes.Buffer
	active := &keepawake.State{PID: 123, Until: time.Date(2026, time.March, 10, 17, 30, 0, 0, statusZone)}
	cmd := fakeStatusCommand(active, nil)
	cmd.SetArgs([]string{"--json", "--quiet"})
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)

	if err := cmd.Execute(); err == nil {
		t.Error("Execute() error = nil, want an error")
	}
	if got := out.String(); got != "" {
		t.Errorf("stdout = %q, want empty", got)
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

	active := &keepawake.State{PID: 123, Until: time.Date(2026, time.March, 10, 17, 30, 0, 0, statusZone)}
	execErr := fakeStatusCommand(active, nil).Execute()

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
	if len(gotErr) != 0 {
		t.Errorf("stderr = %q, want empty", gotErr)
	}
}
