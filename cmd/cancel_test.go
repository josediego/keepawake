package cmd

import (
	"bytes"
	"errors"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/jose/keepawake/internal/keepawake"
	"github.com/spf13/cobra"
)

func TestRunCancel(t *testing.T) {
	sessionErr := errors.New("x")
	tests := []struct {
		name       string
		pid        int
		cancelErr  error
		trayErr    error
		wantErr    error
		wantOut    string
		wantErrOut string
	}{
		{name: "active session", pid: 44, wantOut: "Keepawake cancelled (PID 44)\n"},
		{name: "no session", cancelErr: keepawake.ErrStateNotFound, wantErr: keepawake.ErrStateNotFound},
		{name: "failing session stop", cancelErr: sessionErr, wantErr: sessionErr},
		{
			name:       "tray error is a warning",
			pid:        44,
			trayErr:    errors.New("boom"),
			wantOut:    "Keepawake cancelled (PID 44)\n",
			wantErrOut: "Warning: could not close tray: boom\n",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var calls []string
			cancel := func() (int, error) {
				calls = append(calls, "stop-session")
				return tt.pid, tt.cancelErr
			}
			stopTray := func() error {
				calls = append(calls, "stop-tray")
				return tt.trayErr
			}
			cmd := &cobra.Command{}
			var out, errOut bytes.Buffer
			cmd.SetOut(&out)
			cmd.SetErr(&errOut)

			err := runCancel(cmd, cancel, stopTray)

			if !errors.Is(err, tt.wantErr) {
				t.Errorf("runCancel() error = %v, want %v", err, tt.wantErr)
			}
			if want := []string{"stop-session", "stop-tray"}; !slices.Equal(calls, want) {
				t.Errorf("calls = %q, want %q", calls, want)
			}
			if got := out.String(); got != tt.wantOut {
				t.Errorf("stdout = %q, want %q", got, tt.wantOut)
			}
			if got := errOut.String(); got != tt.wantErrOut {
				t.Errorf("stderr = %q, want %q", got, tt.wantErrOut)
			}
		})
	}
}

func TestCancelNeverLaunchesTray(t *testing.T) {
	src, err := os.ReadFile("cancel.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"EnsureRunning", "launchTray"} {
		if strings.Contains(string(src), name) {
			t.Errorf("cancel.go refers to %s", name)
		}
	}
}
