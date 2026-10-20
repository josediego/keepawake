package keepawake

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFor(t *testing.T) {
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
	)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	pid, until, err := m.For(45 * time.Minute)
	if err != nil {
		t.Fatalf("For() error = %v", err)
	}
	t.Cleanup(func() {
		p, _ := os.FindProcess(pid)
		_ = p.Kill()
		_, _ = p.Wait()
	})

	want := now.Add(45 * time.Minute)
	if !until.Equal(want) {
		t.Errorf("For() until = %v, want %v", until, want)
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

	var args []byte
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
	if got := strings.TrimSpace(string(args)); got != "-di -t 2700" {
		t.Errorf("caffeinate args = %q, want %q", got, "-di -t 2700")
	}
}

func TestForOutOfRangeKeepsExistingSession(t *testing.T) {
	for _, d := range []time.Duration{0, 59 * time.Second, 24*time.Hour + time.Minute, -5 * time.Minute} {
		t.Run(d.String(), func(t *testing.T) {
			assertKeepsSession(t, func(m *Manager, now time.Time) error {
				_, _, err := m.For(d)
				return err
			})
		})
	}
}

func TestStartUntilPastTimeKeepsExistingSession(t *testing.T) {
	assertKeepsSession(t, func(m *Manager, now time.Time) error {
		_, _, err := m.StartUntil(now.Add(-time.Hour))
		return err
	})
}

func assertKeepsSession(t *testing.T, call func(m *Manager, now time.Time) error) {
	t.Helper()

	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start child process: %v", err)
	}
	done := make(chan struct{})
	go func() {
		_ = cmd.Wait()
		close(done)
	}()
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		<-done
	})

	stateFile := filepath.Join(t.TempDir(), "state")
	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.Local)

	m, err := NewManager(
		WithStateFile(stateFile),
		WithClock(func() time.Time { return now }),
		WithCaffeinateBinary("/nonexistent/caffeinate"),
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

	if err := call(m, now); err == nil {
		t.Fatalf("expected error, got nil")
	}

	select {
	case <-done:
		t.Errorf("existing session process was terminated")
	case <-time.After(100 * time.Millisecond):
	}

	after, err := os.ReadFile(stateFile)
	if err != nil {
		t.Fatalf("state file missing: %v", err)
	}
	if !bytes.Equal(before, after) {
		t.Errorf("state file changed: before %q, after %q", before, after)
	}
}
