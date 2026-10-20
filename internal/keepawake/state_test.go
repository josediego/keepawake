package keepawake

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

var invalidStateCases = []struct {
	name     string
	contents string
}{
	{name: "empty", contents: ""},
	{name: "truncated", contents: "{"},
	{name: "null", contents: "null"},
	{name: "zero pid", contents: `{"pid":0,"until":123}`},
	{name: "zero until", contents: `{"pid":123,"until":0}`},
	{name: "negative pid", contents: `{"pid":-1,"until":123}`},
	{name: "negative process group pid", contents: `{"pid":-4242,"until":123}`},
	{name: "negative until", contents: `{"pid":123,"until":-5}`},
	{name: "unknown mode", contents: `{"pid":123,"until":123,"mode":"x"}`},
	{name: "repeated mode", contents: `{"pid":123,"until":123,"mode":"dd"}`},
	{name: "out-of-order mode", contents: `{"pid":123,"until":123,"mode":"id"}`},
}

func newManagerWithState(t *testing.T, contents string) (*Manager, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state")
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("failed to write state file: %v", err)
	}
	m, err := NewManager(WithStateFile(path), WithProcessController(&fakeController{}))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	return m, path
}

func assertStateFileRemoved(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected state file to be removed, stat err = %v", err)
	}
}

func TestReadStateInvalidState(t *testing.T) {
	for _, tc := range invalidStateCases {
		t.Run(tc.name, func(t *testing.T) {
			m, _ := newManagerWithState(t, tc.contents)
			if _, err := m.readState(); !errors.Is(err, ErrInvalidState) {
				t.Fatalf("readState() error = %v, want ErrInvalidState", err)
			}
		})
	}
}

func TestCurrentStateInvalidState(t *testing.T) {
	for _, tc := range invalidStateCases {
		t.Run(tc.name, func(t *testing.T) {
			m, path := newManagerWithState(t, tc.contents)
			if _, err := m.CurrentState(); !errors.Is(err, ErrStateNotFound) {
				t.Fatalf("CurrentState() error = %v, want ErrStateNotFound", err)
			}
			assertStateFileRemoved(t, path)
		})
	}
}

func TestCancelInvalidState(t *testing.T) {
	for _, tc := range invalidStateCases {
		t.Run(tc.name, func(t *testing.T) {
			m, path := newManagerWithState(t, tc.contents)
			if _, err := m.Cancel(); !errors.Is(err, ErrStateNotFound) {
				t.Fatalf("Cancel() error = %v, want ErrStateNotFound", err)
			}
			assertStateFileRemoved(t, path)
		})
	}
}

func TestCancelExistingSessionInvalidState(t *testing.T) {
	for _, tc := range invalidStateCases {
		t.Run(tc.name, func(t *testing.T) {
			m, path := newManagerWithState(t, tc.contents)
			if err := m.cancelExistingSession(); err != nil {
				t.Fatalf("cancelExistingSession() error = %v, want nil", err)
			}
			assertStateFileRemoved(t, path)
		})
	}
}

func TestSaveStateAtomicPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state")
	m, err := NewManager(WithStateFile(path), WithProcessController(&fakeController{}))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}

	// The second write replaces an existing file.
	for _, pid := range []int{1234, 5678} {
		if err := m.saveState(pid, time.Now().Add(time.Hour).Unix()); err != nil {
			t.Fatalf("saveState() error = %v", err)
		}
		leftovers, err := filepath.Glob(filepath.Join(dir, "*.tmp-*"))
		if err != nil {
			t.Fatalf("Glob() error = %v", err)
		}
		if len(leftovers) != 0 {
			t.Fatalf("temp files left behind: %v", leftovers)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatalf("Stat() error = %v", err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Fatalf("state file mode = %o, want 600", perm)
		}
		s, err := m.readState()
		if err != nil {
			t.Fatalf("readState() error = %v", err)
		}
		if s.PID != pid {
			t.Fatalf("readState() PID = %d, want %d", s.PID, pid)
		}
	}
}

func TestNegativePIDNeverSignalsAProcessGroup(t *testing.T) {
	cmd := exec.Command("sleep", "30")
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
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

	contents := fmt.Sprintf(`{"pid":%d,"until":%d}`, -cmd.Process.Pid, time.Now().Add(time.Hour).Unix())
	m, path := newManagerWithState(t, contents)

	assertAlive := func(op string) {
		t.Helper()
		select {
		case <-done:
			t.Fatalf("child process group was signalled by %s", op)
		case <-time.After(200 * time.Millisecond):
		}
	}

	if _, err := m.Cancel(); !errors.Is(err, ErrStateNotFound) {
		t.Fatalf("Cancel() error = %v, want ErrStateNotFound", err)
	}
	assertStateFileRemoved(t, path)
	assertAlive("Cancel()")

	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("failed to write state file: %v", err)
	}
	if err := m.cancelExistingSession(); err != nil {
		t.Fatalf("cancelExistingSession() error = %v, want nil", err)
	}
	assertStateFileRemoved(t, path)
	assertAlive("cancelExistingSession()")
}
