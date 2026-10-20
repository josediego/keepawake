package cmd

import (
	"context"
	"errors"
	"io"
	"os"
	"regexp"
	"testing"
	"time"

	"github.com/jose/keepawake/internal/tray"
	"github.com/spf13/cobra"
)

// fakeSession returns fixed results, so no test touches the real state file.
type fakeSession struct{}

func (fakeSession) StartUntil(time.Time) (int, time.Duration, error) { return 43, 2 * time.Hour, nil }
func (fakeSession) For(d time.Duration) (int, time.Time, error)      { return 42, time.Now().Add(d), nil }
func (fakeSession) Cancel() (int, error)                             { return 44, nil }

// fakeSessionDeps builds sessionDeps from fakes, counting launcher runs in *launches.
func fakeSessionDeps(launches *int) sessionDeps {
	return sessionDeps{
		manager: func() (sessionManager, error) { return fakeSession{}, nil },
		launchTray: func() error {
			*launches++
			return errors.New("boom")
		},
	}
}

// fakeApp counts its runs instead of starting the tray.
type fakeApp struct{ runs int }

func (a *fakeApp) Run(context.Context) error {
	a.runs++
	return nil
}

// runOnRealStreams executes c with os.Stdout and os.Stderr redirected to pipes, without SetOut or SetErr.
func runOnRealStreams(t *testing.T, c *cobra.Command, args ...string) (stdout, stderr string, execErr error) {
	t.Helper()

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

	c.SetArgs(args)
	execErr = c.Execute()

	restore()
	stdoutW.Close()
	stderrW.Close()

	gotOut, err := io.ReadAll(stdoutR)
	if err != nil {
		t.Fatal(err)
	}
	gotErr, err := io.ReadAll(stderrR)
	if err != nil {
		t.Fatal(err)
	}
	return string(gotOut), string(gotErr), execErr
}

func TestSessionCommandsWriteToRealStreams(t *testing.T) {
	tests := []struct {
		name    string
		newCmd  func(sessionDeps) *cobra.Command
		args    []string
		wantOut *regexp.Regexp
	}{
		{
			name:    "for",
			newCmd:  newForCommand,
			args:    []string{"45m"},
			wantOut: regexp.MustCompile(`^Keepawake active \(PID 42\) for 45m \(until [^)]+\)\n$`),
		},
		{
			name:    "until",
			newCmd:  newUntilCommand,
			args:    []string{"17:00", "--yes"},
			wantOut: regexp.MustCompile(`^Keepawake active \(PID 43\) for 2h \(until (\w{3} \d{1,2} \w{3} )?17:00\)\n$`),
		},
		{
			name:    "cancel",
			newCmd:  newCancelCommand,
			wantOut: regexp.MustCompile(`^Keepawake cancelled \(PID 44\)\n$`),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			launches := 0
			gotOut, gotErr, err := runOnRealStreams(t, tt.newCmd(fakeSessionDeps(&launches)), tt.args...)

			if err != nil {
				t.Fatalf("Execute() error = %v", err)
			}
			if !tt.wantOut.MatchString(gotOut) {
				t.Errorf("stdout = %q, want match for %s", gotOut, tt.wantOut)
			}
			if want := "Warning: could not launch tray: boom\n"; gotErr != want {
				t.Errorf("stderr = %q, want %q", gotErr, want)
			}
			if launches != 1 {
				t.Errorf("launcher ran %d times, want 1", launches)
			}
		})
	}
}

func TestTrayCommandWritesToRealStreams(t *testing.T) {
	t.Run("already running", func(t *testing.T) {
		app := &fakeApp{}
		cmd := newTrayCommand(trayDeps{
			newApp: func(...tray.Option) (trayRunner, error) { return app, nil },
			guard: func(int) (func() error, int, bool, error) {
				return nil, 77, true, nil
			},
		})

		gotOut, gotErr, err := runOnRealStreams(t, cmd)

		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if want := "Keepawake tray already running (PID 77)\n"; gotOut != want {
			t.Errorf("stdout = %q, want %q", gotOut, want)
		}
		if gotErr != "" {
			t.Errorf("stderr = %q, want empty", gotErr)
		}
		if app.runs != 0 {
			t.Errorf("app ran %d times, want 0", app.runs)
		}
	})

	t.Run("launching", func(t *testing.T) {
		app := &fakeApp{}
		cleanups := 0
		cmd := newTrayCommand(trayDeps{
			newApp: func(...tray.Option) (trayRunner, error) { return app, nil },
			guard: func(int) (func() error, int, bool, error) {
				return func() error { cleanups++; return nil }, 0, false, nil
			},
		})

		gotOut, gotErr, err := runOnRealStreams(t, cmd)

		if err != nil {
			t.Fatalf("Execute() error = %v", err)
		}
		if want := "Launching Keepawake in the menu bar. Use the icon to exit.\n"; gotOut != want {
			t.Errorf("stdout = %q, want %q", gotOut, want)
		}
		if gotErr != "" {
			t.Errorf("stderr = %q, want empty", gotErr)
		}
		if app.runs != 1 {
			t.Errorf("app ran %d times, want 1", app.runs)
		}
		if cleanups != 1 {
			t.Errorf("cleanup ran %d times, want 1", cleanups)
		}
	})
}
