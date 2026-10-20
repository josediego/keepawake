package keepawake

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

func newStateWriteManager(t *testing.T, path string) *Manager {
	t.Helper()
	m, err := NewManager(WithStateFile(path), WithProcessController(&fakeController{}))
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	return m
}

func TestSaveStateReplacesFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state")
	m := newStateWriteManager(t, path)

	if err := m.saveState(7, 1); err != nil {
		t.Fatalf("saveState() error = %v", err)
	}
	before, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	reader, err := os.Open(path)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer reader.Close()

	if err := m.saveState(123456789, 9999999999); err != nil {
		t.Fatalf("saveState() error = %v", err)
	}
	after, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}

	// A reader opened before the save keeps seeing the old file in full.
	old, err := io.ReadAll(reader)
	if err != nil {
		t.Fatalf("ReadAll() error = %v", err)
	}
	if got, want := string(old), `{"pid":7,"until":1}`; got != want {
		t.Fatalf("open reader read %q, want %q", got, want)
	}
	s, err := m.readState()
	if err != nil {
		t.Fatalf("readState() error = %v", err)
	}
	if s.PID != 123456789 || s.Until != 9999999999 {
		t.Fatalf("readState() = %+v, want pid 123456789, until 9999999999", *s)
	}
	if os.SameFile(before, after) {
		t.Fatal("saveState rewrote the state file in place, want it replaced")
	}
}

func TestSaveStateConcurrentReader(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state")
	m := newStateWriteManager(t, path)
	if err := m.saveState(7, 1); err != nil {
		t.Fatalf("saveState() error = %v", err)
	}

	var reads, missing, torn int
	stop := make(chan struct{})
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			select {
			case <-stop:
				return
			default:
			}
			_, err := m.readState()
			reads++
			if errors.Is(err, ErrStateNotFound) {
				missing++
			} else if err != nil {
				torn++
			}
		}
	}()

	start := time.Now()
	saves := 0
	for time.Since(start) < 4*time.Second && (saves < 200 || time.Since(start) < 300*time.Millisecond) {
		var err error
		if saves%2 == 0 {
			err = m.saveState(123456789, 9999999999)
		} else {
			err = m.saveState(7, 1)
		}
		if err != nil {
			close(stop)
			<-done
			t.Fatalf("saveState() error = %v", err)
		}
		saves++
	}
	close(stop)
	<-done

	if missing != 0 || torn != 0 {
		t.Fatalf("%d reads during %d saves: %d missing, %d torn; want none", reads, saves, missing, torn)
	}
	if reads < 100 {
		t.Fatalf("only %d reads during %d saves, too few to tell", reads, saves)
	}
}

func TestSaveStateFailedRenameLeavesNothing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state")
	// Nothing can be renamed over a non-empty directory.
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatalf("Mkdir() error = %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, "keep"), nil, 0o600); err != nil {
		t.Fatalf("WriteFile() error = %v", err)
	}
	m := newStateWriteManager(t, path)

	if err := m.saveState(7, 1); err == nil {
		t.Fatal("saveState() error = nil, want rename failure")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir() error = %v", err)
	}
	var names []string
	for _, e := range entries {
		names = append(names, e.Name())
	}
	if len(names) != 1 || names[0] != "state" {
		t.Fatalf("directory holds %v, want only [state]", names)
	}
}

func TestStateIOErrorsReturnedAsIs(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, path string)
		want  error
	}{
		{
			name: "directory",
			setup: func(t *testing.T, path string) {
				// Empty, so removing the state file would delete it.
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatalf("Mkdir() error = %v", err)
				}
			},
			want: syscall.EISDIR,
		},
		{
			name: "unreadable",
			setup: func(t *testing.T, path string) {
				if err := os.WriteFile(path, []byte(`{"pid":4242,"until":9999999999}`), 0o600); err != nil {
					t.Fatalf("WriteFile() error = %v", err)
				}
				if err := os.Chmod(path, 0); err != nil {
					t.Fatalf("Chmod() error = %v", err)
				}
				if _, err := os.ReadFile(path); err == nil {
					t.Skip("mode 000 file is still readable (running as root?)")
				}
			},
			want: fs.ErrPermission,
		},
	}
	calls := []struct {
		name string
		call func(m *Manager) error
	}{
		{"readState", func(m *Manager) error { _, err := m.readState(); return err }},
		{"CurrentState", func(m *Manager) error { _, err := m.CurrentState(); return err }},
		{"Cancel", func(m *Manager) error { _, err := m.Cancel(); return err }},
		{"cancelExistingSession", func(m *Manager) error { return m.cancelExistingSession() }},
	}
	for _, tc := range cases {
		for _, c := range calls {
			t.Run(tc.name+"/"+c.name, func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "state")
				tc.setup(t, path)
				m := newStateWriteManager(t, path)

				err := c.call(m)
				if !errors.Is(err, tc.want) {
					t.Fatalf("%s() error = %v, want %v", c.name, err, tc.want)
				}
				if errors.Is(err, ErrInvalidState) || errors.Is(err, ErrStateNotFound) {
					t.Fatalf("%s() error = %v, want the I/O error as it is", c.name, err)
				}
				if _, err := os.Lstat(path); err != nil {
					t.Fatalf("state path gone after %s(): %v", c.name, err)
				}
			})
		}
	}
}
