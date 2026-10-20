package keepawake

import (
	"errors"
	"path/filepath"
	"testing"
	"time"
)

// fakeController records Terminate calls so tests never run ps or send signals.
type fakeController struct {
	caffeinate bool
	terminated []int
}

func (f *fakeController) IsCaffeinate(pid int) bool { return f.caffeinate }

func (f *fakeController) Terminate(pid int) error {
	f.terminated = append(f.terminated, pid)
	return nil
}

const fakePID = 4242

func newManagerWithSession(t *testing.T, pc *fakeController, until time.Duration) (*Manager, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "state")
	now := time.Date(2026, 1, 15, 10, 0, 0, 0, time.Local)
	m, err := NewManager(
		WithStateFile(path),
		WithClock(func() time.Time { return now }),
		WithProcessController(pc),
	)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	if err := m.saveState(fakePID, now.Add(until).Unix()); err != nil {
		t.Fatalf("saveState() error = %v", err)
	}
	return m, path
}

func assertTerminated(t *testing.T, pc *fakeController, want []int) {
	t.Helper()
	if len(pc.terminated) != len(want) {
		t.Fatalf("Terminate calls = %v, want %v", pc.terminated, want)
	}
	for i := range want {
		if pc.terminated[i] != want[i] {
			t.Fatalf("Terminate calls = %v, want %v", pc.terminated, want)
		}
	}
}

func TestCancelLiveSessionTerminates(t *testing.T) {
	pc := &fakeController{caffeinate: true}
	m, path := newManagerWithSession(t, pc, 24*time.Hour)

	pid, err := m.Cancel()
	if err != nil {
		t.Fatalf("Cancel() error = %v", err)
	}
	if pid != fakePID {
		t.Errorf("Cancel() pid = %d, want %d", pid, fakePID)
	}
	assertTerminated(t, pc, []int{fakePID})
	assertStateFileRemoved(t, path)
}

func TestCancelReusedPIDNeverTerminates(t *testing.T) {
	pc := &fakeController{caffeinate: false}
	m, path := newManagerWithSession(t, pc, 24*time.Hour)

	if _, err := m.Cancel(); !errors.Is(err, ErrStateNotFound) {
		t.Fatalf("Cancel() error = %v, want ErrStateNotFound", err)
	}
	assertTerminated(t, pc, nil)
	assertStateFileRemoved(t, path)
}

func TestCancelExpiredSessionNeverTerminates(t *testing.T) {
	pc := &fakeController{caffeinate: true}
	m, path := newManagerWithSession(t, pc, -time.Hour)

	if _, err := m.Cancel(); !errors.Is(err, ErrStateNotFound) {
		t.Fatalf("Cancel() error = %v, want ErrStateNotFound", err)
	}
	assertTerminated(t, pc, nil)
	assertStateFileRemoved(t, path)
}

func TestCancelExistingSessionLiveTerminates(t *testing.T) {
	pc := &fakeController{caffeinate: true}
	m, path := newManagerWithSession(t, pc, 24*time.Hour)

	if err := m.cancelExistingSession(); err != nil {
		t.Fatalf("cancelExistingSession() error = %v", err)
	}
	assertTerminated(t, pc, []int{fakePID})
	assertStateFileRemoved(t, path)
}

func TestCancelExistingSessionReusedPIDNeverTerminates(t *testing.T) {
	pc := &fakeController{caffeinate: false}
	m, path := newManagerWithSession(t, pc, 24*time.Hour)

	if err := m.cancelExistingSession(); err != nil {
		t.Fatalf("cancelExistingSession() error = %v", err)
	}
	assertTerminated(t, pc, nil)
	assertStateFileRemoved(t, path)
}

func TestCurrentStateReusedPIDIsStale(t *testing.T) {
	pc := &fakeController{caffeinate: false}
	m, path := newManagerWithSession(t, pc, 24*time.Hour)

	if _, err := m.CurrentState(); !errors.Is(err, ErrStateNotFound) {
		t.Fatalf("CurrentState() error = %v, want ErrStateNotFound", err)
	}
	assertTerminated(t, pc, nil)
	assertStateFileRemoved(t, path)
}
