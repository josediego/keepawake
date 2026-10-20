package keepawake

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"syscall"
	"testing"
	"time"
)

// hookWait is how long a hook gives the other Manager to reach the lock.
const hookWait = 50 * time.Millisecond

// syncController is a fakeController that is safe to share between
// goroutines, with hooks that run inside IsCaffeinate and Terminate.
type syncController struct {
	dead           bool
	onIsCaffeinate func(pid int)
	onTerminate    func(pid int)

	mu         sync.Mutex
	terminated []int
}

func (c *syncController) IsCaffeinate(pid int) bool {
	if c.onIsCaffeinate != nil {
		c.onIsCaffeinate(pid)
	}
	return !c.dead
}

func (c *syncController) Terminate(pid int) error {
	if c.onTerminate != nil {
		c.onTerminate(pid)
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.terminated = append(c.terminated, pid)
	return nil
}

func (c *syncController) terminatedPIDs() []int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]int(nil), c.terminated...)
}

func newLockManager(t *testing.T, path string, pc ProcessController, binary string) *Manager {
	t.Helper()
	m, err := NewManager(
		WithStateFile(path),
		WithProcessController(pc),
		WithCaffeinateBinary(binary),
	)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	return m
}

func setLockWait(t *testing.T, d time.Duration) {
	t.Helper()
	old := lockWait
	lockWait = d
	t.Cleanup(func() { lockWait = old })
}

type forResult struct {
	pid int
	err error
}

func TestStaleCleanupKeepsNewerSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state")
	writer := newLockManager(t, path, &syncController{}, "/usr/bin/true")
	if err := writer.saveState(fakePID, time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatalf("saveState() error = %v", err)
	}

	done := make(chan forResult, 1)
	reader := newLockManager(t, path, &syncController{
		dead: true,
		onIsCaffeinate: func(pid int) {
			if pid != fakePID {
				return
			}
			go func() {
				pid, _, err := writer.For(time.Hour)
				done <- forResult{pid, err}
			}()
			time.Sleep(hookWait)
		},
	}, "/usr/bin/true")

	if _, err := reader.CurrentState(); !errors.Is(err, ErrStateNotFound) {
		t.Fatalf("CurrentState() error = %v, want ErrStateNotFound", err)
	}
	res := <-done
	if res.err != nil {
		t.Fatalf("For() error = %v", res.err)
	}
	s, err := writer.readState()
	if err != nil {
		t.Fatalf("readState() error = %v", err)
	}
	if s.PID != res.pid {
		t.Fatalf("state PID = %d, want the new session's %d", s.PID, res.pid)
	}
}

func TestReplacementShowsNewSession(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state")
	reader := newLockManager(t, path, &syncController{}, "/usr/bin/true")

	type stateResult struct {
		s   *State
		err error
	}
	done := make(chan stateResult, 1)
	writer := newLockManager(t, path, &syncController{
		onTerminate: func(pid int) {
			go func() {
				s, err := reader.CurrentState()
				done <- stateResult{s, err}
			}()
			time.Sleep(hookWait)
		},
	}, "/usr/bin/true")
	if err := writer.saveState(fakePID, time.Now().Add(time.Hour).Unix()); err != nil {
		t.Fatalf("saveState() error = %v", err)
	}

	pid, _, err := writer.For(time.Hour)
	if err != nil {
		t.Fatalf("For() error = %v", err)
	}
	res := <-done
	if res.err != nil {
		t.Fatalf("CurrentState() error = %v, want the new session", res.err)
	}
	if res.s.PID != pid {
		t.Fatalf("CurrentState() PID = %d, want the new session's %d", res.s.PID, pid)
	}
}

func TestConcurrentForLeavesOneSession(t *testing.T) {
	for round := 0; round < 20; round++ {
		path := filepath.Join(t.TempDir(), "state")
		pc := &syncController{}
		managers := []*Manager{
			newLockManager(t, path, pc, "/usr/bin/true"),
			newLockManager(t, path, pc, "/usr/bin/true"),
		}

		results := make([]forResult, len(managers))
		var wg sync.WaitGroup
		for i, m := range managers {
			wg.Add(1)
			go func() {
				defer wg.Done()
				pid, _, err := m.For(time.Hour)
				results[i] = forResult{pid, err}
			}()
		}
		wg.Wait()

		for _, r := range results {
			if r.err != nil {
				t.Fatalf("round %d: For() error = %v", round, r.err)
			}
		}
		s, err := managers[0].readState()
		if err != nil {
			t.Fatalf("round %d: readState() error = %v", round, err)
		}
		var other int
		switch s.PID {
		case results[0].pid:
			other = results[1].pid
		case results[1].pid:
			other = results[0].pid
		default:
			t.Fatalf("round %d: state PID = %d, want %d or %d", round, s.PID, results[0].pid, results[1].pid)
		}
		if got := pc.terminatedPIDs(); len(got) != 1 || got[0] != other {
			t.Fatalf("round %d: Terminate calls = %v, want [%d]", round, got, other)
		}
	}
}

func TestCaffeinateDoesNotInheritLock(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "caffeinate")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nexec sleep 30\n"), 0o700); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	path := filepath.Join(dir, "state")
	m := newLockManager(t, path, &syncController{}, script)

	pid, _, err := m.For(time.Hour)
	if err != nil {
		t.Fatalf("For() error = %v", err)
	}
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })

	setLockWait(t, hookWait)
	unlock, err := m.lock()
	if err != nil {
		t.Fatalf("lock() after For() error = %v, want the lock free", err)
	}
	unlock()

	info, err := os.Stat(path + ".lock")
	if err != nil {
		t.Fatalf("Stat(lock file) error = %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("lock file mode = %o, want 600", perm)
	}
}

func TestBusyLockChangesNothing(t *testing.T) {
	calls := []struct {
		name string
		call func(m *Manager) error
	}{
		{"CurrentState", func(m *Manager) error { _, err := m.CurrentState(); return err }},
		{"For", func(m *Manager) error { _, _, err := m.For(time.Hour); return err }},
		{"Cancel", func(m *Manager) error { _, err := m.Cancel(); return err }},
	}
	for _, c := range calls {
		t.Run(c.name, func(t *testing.T) {
			setLockWait(t, hookWait)
			pc := &fakeController{caffeinate: true}
			// Expired, so each call would otherwise remove the state file.
			m, path := newManagerWithSession(t, pc, -time.Hour)
			m.caffeinateBinary = "/usr/bin/true"
			before, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile() error = %v", err)
			}

			f, err := os.OpenFile(path+".lock", os.O_RDWR|os.O_CREATE, 0o600)
			if err != nil {
				t.Fatalf("OpenFile(lock file) error = %v", err)
			}
			t.Cleanup(func() { _ = f.Close() })
			if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
				t.Fatalf("Flock() error = %v", err)
			}

			if err := c.call(m); !errors.Is(err, ErrStateBusy) {
				t.Fatalf("%s() error = %v, want ErrStateBusy", c.name, err)
			}
			after, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile() after %s() error = %v", c.name, err)
			}
			if !bytes.Equal(before, after) {
				t.Fatalf("state file = %q after %s(), want %q", after, c.name, before)
			}
			assertTerminated(t, pc, nil)
		})
	}
}
