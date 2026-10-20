package keepawake

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jose/keepawake/internal/procutil"
)

// newExtendManager returns a Manager whose caffeinate is a fake script that
// records its arguments, the path those arguments are written to, and its clock.
func newExtendManager(t *testing.T, pc *fakeController) (*Manager, string, time.Time) {
	t.Helper()
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "args")
	script := filepath.Join(dir, "caffeinate")
	body := fmt.Sprintf("#!/bin/sh\necho \"$@\" > '%[1]s.tmp' && mv '%[1]s.tmp' '%[1]s'\nexec sleep 5\n", argsFile)
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatalf("failed to write fake caffeinate: %v", err)
	}

	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.Local)
	m, err := NewManager(
		WithStateFile(filepath.Join(dir, "state")),
		WithClock(func() time.Time { return now }),
		WithCaffeinateBinary(script),
		WithProcessController(pc),
	)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	return m, argsFile, now
}

func killOnCleanup(t *testing.T, pid int) {
	t.Helper()
	t.Cleanup(func() {
		p, _ := os.FindProcess(pid)
		_ = p.Kill()
		_, _ = p.Wait()
	})
}

func assertArgs(t *testing.T, argsFile, want string) {
	t.Helper()
	var args []byte
	var err error
	deadline := time.Now().Add(2 * time.Second)
	for {
		args, err = os.ReadFile(argsFile)
		if err == nil || time.Now().After(deadline) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("fake caffeinate did not record its arguments: %v", err)
	}
	if got := strings.TrimSpace(string(args)); got != want {
		t.Errorf("caffeinate args = %q, want %q", got, want)
	}
}

func assertExtended(t *testing.T, m *Manager, pid int, until, want time.Time) {
	t.Helper()
	if !until.Equal(want) {
		t.Errorf("Extend() until = %v, want %v", until, want)
	}
	s, err := m.readState()
	if err != nil {
		t.Fatalf("readState() error = %v", err)
	}
	if s.PID != pid {
		t.Errorf("state pid = %d, want %d", s.PID, pid)
	}
	if s.Until != want.Unix() {
		t.Errorf("state until = %d, want %d", s.Until, want.Unix())
	}
}

func TestExtendActiveSession(t *testing.T) {
	pc := &fakeController{caffeinate: true}
	m, argsFile, now := newExtendManager(t, pc)
	if err := m.saveState(fakePID, now.Add(time.Hour).Unix()); err != nil {
		t.Fatalf("saveState() error = %v", err)
	}

	pid, until, err := m.Extend(30 * time.Minute)
	if err != nil {
		t.Fatalf("Extend() error = %v", err)
	}
	killOnCleanup(t, pid)

	assertExtended(t, m, pid, until, now.Add(90*time.Minute))
	assertTerminated(t, pc, []int{fakePID})
	assertArgs(t, argsFile, "-di -t 5400")
}

func TestExtendWithoutRunningSessionStartsNow(t *testing.T) {
	tests := []struct {
		name       string
		caffeinate bool
		session    time.Duration // 0 means no state file
	}{
		{"no session", true, 0},
		{"stale pid", false, time.Hour},
		{"expired", true, -time.Hour},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pc := &fakeController{caffeinate: tt.caffeinate}
			m, argsFile, now := newExtendManager(t, pc)
			if tt.session != 0 {
				if err := m.saveState(fakePID, now.Add(tt.session).Unix()); err != nil {
					t.Fatalf("saveState() error = %v", err)
				}
			}

			pid, until, err := m.Extend(30 * time.Minute)
			if err != nil {
				t.Fatalf("Extend() error = %v", err)
			}
			killOnCleanup(t, pid)

			assertExtended(t, m, pid, until, now.Add(30*time.Minute))
			assertTerminated(t, pc, nil)
			assertArgs(t, argsFile, "-di -t 1800")
		})
	}
}

func TestExtendToExactlyMaxDuration(t *testing.T) {
	pc := &fakeController{caffeinate: true}
	m, argsFile, now := newExtendManager(t, pc)
	if err := m.saveState(fakePID, now.Add(23*time.Hour+30*time.Minute).Unix()); err != nil {
		t.Fatalf("saveState() error = %v", err)
	}

	pid, until, err := m.Extend(30 * time.Minute)
	if err != nil {
		t.Fatalf("Extend() error = %v", err)
	}
	killOnCleanup(t, pid)

	assertExtended(t, m, pid, until, now.Add(MaxDuration))
	assertTerminated(t, pc, []int{fakePID})
	assertArgs(t, argsFile, "-di -t 86400")
}

func TestExtendOutOfRangeKeepsExistingSession(t *testing.T) {
	for _, d := range []time.Duration{0, 59 * time.Second, 24*time.Hour + time.Minute} {
		t.Run(d.String(), func(t *testing.T) {
			assertKeepsSession(t, func(m *Manager, now time.Time) error {
				_, _, err := m.Extend(d)
				return err
			})
		})
	}
}

func TestExtendPastMaxDurationKeepsExistingSession(t *testing.T) {
	// assertKeepsSession's session ends in 1h, so this would end in 24h30m.
	assertKeepsSession(t, func(m *Manager, now time.Time) error {
		_, _, err := m.Extend(23*time.Hour + 30*time.Minute)
		return err
	})
}

func TestStartReapsCaffeinate(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "caffeinate")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexit 0\n"), 0o755); err != nil {
		t.Fatalf("failed to write fake caffeinate: %v", err)
	}

	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.Local)
	m, err := NewManager(
		WithStateFile(filepath.Join(dir, "state")),
		WithClock(func() time.Time { return now }),
		WithCaffeinateBinary(script),
		WithProcessController(&fakeController{}),
	)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	pid, _, err := m.For(30 * time.Minute)
	if err != nil {
		t.Fatalf("For() error = %v", err)
	}

	deadline := time.Now().Add(3 * time.Second)
	for procutil.Alive(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("caffeinate %d was not reaped after it exited", pid)
		}
		time.Sleep(20 * time.Millisecond)
	}
}
