package keepawake

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func TestUntilPastTimeKeepsExistingSession(t *testing.T) {
	assertUntilKeepsSession(t, "08:00")
}

func TestUntilInvalidInputKeepsExistingSession(t *testing.T) {
	assertUntilKeepsSession(t, "garbage")
}

func assertUntilKeepsSession(t *testing.T, input string) {
	t.Helper()

	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start child process: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	stateFile := filepath.Join(t.TempDir(), "state")
	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.Local)

	m, err := NewManager(
		WithStateFile(stateFile),
		WithClock(func() time.Time { return now }),
		WithCaffeinateBinary("/nonexistent/caffeinate"),
		WithProcessController(&fakeController{}),
	)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	if err := m.saveState(cmd.Process.Pid, now.Add(time.Hour).Unix()); err != nil {
		t.Fatalf("saveState() error = %v", err)
	}
	before, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("failed to read state file: %v", err)
	}

	if _, _, err := m.Until(input); err == nil {
		t.Fatalf("Until(%q) expected error, got nil", input)
	}

	if err := cmd.Process.Signal(syscall.Signal(0)); err != nil {
		t.Errorf("existing session process was terminated: %v", err)
	}

	after, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("state file missing after Until(%q): %v", input, err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("state file changed: before %q, after %q", before, after)
	}
}
