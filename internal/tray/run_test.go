package tray

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"sync"
	"syscall"
	"testing"
)

func withTempTrayPIDPath(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), trayPIDFileName)
	orig := trayPIDPath
	trayPIDPath = func() (string, error) { return path, nil }
	t.Cleanup(func() { trayPIDPath = orig })
	return path
}

func TestForegroundPIDGuardSingleInstance(t *testing.T) {
	withTempTrayPIDPath(t)

	const n = 20
	type result struct {
		cleanup        func() error
		alreadyRunning bool
		err            error
	}
	var (
		mu      sync.Mutex
		results []result
		wg      sync.WaitGroup
		start   = make(chan struct{})
	)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			cleanup, _, alreadyRunning, err := ForegroundPIDGuard(os.Getpid())
			mu.Lock()
			results = append(results, result{cleanup, alreadyRunning, err})
			mu.Unlock()
		}()
	}
	close(start)
	wg.Wait()

	acquired := 0
	for _, r := range results {
		if r.err != nil {
			t.Errorf("ForegroundPIDGuard() error = %v", r.err)
		}
		if !r.alreadyRunning && r.cleanup != nil {
			acquired++
		}
	}
	for _, r := range results {
		if r.cleanup != nil {
			if err := r.cleanup(); err != nil {
				t.Errorf("cleanup() error = %v", err)
			}
		}
	}
	if acquired != 1 {
		t.Fatalf("acquired = %d, want exactly 1", acquired)
	}
}

func TestForegroundPIDGuardStalePIDFile(t *testing.T) {
	path := withTempTrayPIDPath(t)

	dead := exec.Command("true")
	if err := dead.Run(); err != nil {
		t.Fatalf("failed to run child process: %v", err)
	}
	deadPID := strconv.Itoa(dead.Process.Pid)
	if err := os.WriteFile(path, []byte(deadPID), 0o644); err != nil {
		t.Fatalf("failed to write stale pid file: %v", err)
	}

	cleanup, existingPID, alreadyRunning, err := ForegroundPIDGuard(os.Getpid())
	if err != nil {
		t.Fatalf("ForegroundPIDGuard() error = %v", err)
	}
	if alreadyRunning || cleanup == nil {
		t.Fatalf("ForegroundPIDGuard() alreadyRunning = %v, cleanup nil = %v; want acquired", alreadyRunning, cleanup == nil)
	}
	if existingPID != os.Getpid() {
		t.Fatalf("existingPID = %d, want %d", existingPID, os.Getpid())
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read pid file: %v", err)
	}
	if got, want := string(data), strconv.Itoa(os.Getpid()); got != want {
		t.Fatalf("pid file contents = %q, want %q", got, want)
	}

	if err := cleanup(); err != nil {
		t.Fatalf("cleanup() error = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected pid file to be removed, stat err = %v", err)
	}
}

func TestForegroundPIDGuardReportsHolder(t *testing.T) {
	path := withTempTrayPIDPath(t)

	cleanup, _, alreadyRunning, err := ForegroundPIDGuard(4242)
	if err != nil {
		t.Fatalf("ForegroundPIDGuard(4242) error = %v", err)
	}
	if alreadyRunning || cleanup == nil {
		t.Fatalf("ForegroundPIDGuard(4242) alreadyRunning = %v, cleanup nil = %v; want acquired", alreadyRunning, cleanup == nil)
	}
	defer cleanup()

	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat() error = %v", err)
	}
	if perm := info.Mode().Perm(); perm != 0o600 {
		t.Fatalf("pid file mode = %o, want 600", perm)
	}

	second, existingPID, alreadyRunning, err := ForegroundPIDGuard(4343)
	if err != nil {
		t.Fatalf("ForegroundPIDGuard(4343) error = %v", err)
	}
	if !alreadyRunning || second != nil {
		t.Fatalf("ForegroundPIDGuard(4343) alreadyRunning = %v, cleanup nil = %v; want already running", alreadyRunning, second == nil)
	}
	if existingPID != 4242 {
		t.Fatalf("existingPID = %d, want 4242", existingPID)
	}
}

func TestForegroundPIDGuardUnlockedLivePIDIsStale(t *testing.T) {
	path := withTempTrayPIDPath(t)

	// A live PID that nobody holds the lock for.
	if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		t.Fatalf("failed to write pid file: %v", err)
	}

	cleanup, _, alreadyRunning, err := ForegroundPIDGuard(4242)
	if err != nil {
		t.Fatalf("ForegroundPIDGuard() error = %v", err)
	}
	if alreadyRunning || cleanup == nil {
		t.Fatalf("ForegroundPIDGuard() alreadyRunning = %v, cleanup nil = %v; want acquired", alreadyRunning, cleanup == nil)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read pid file: %v", err)
	}
	if got := string(data); got != "4242" {
		t.Fatalf("pid file contents = %q, want %q", got, "4242")
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup() error = %v", err)
	}
}

// fakeSignalTray replaces signalTray with a recorder of SIGTERM'd PIDs, so no test signals a real process.
func fakeSignalTray(t *testing.T) *[]int {
	t.Helper()
	var signalled []int
	orig := signalTray
	signalTray = func(pid int, sig syscall.Signal) error {
		if sig != syscall.SIGTERM {
			t.Errorf("signalTray(%d, %v), want SIGTERM", pid, sig)
		}
		signalled = append(signalled, pid)
		return nil
	}
	t.Cleanup(func() { signalTray = orig })
	return &signalled
}

// holdTrayLock writes content to path and holds its lock until the test ends, as a live tray would.
func holdTrayLock(t *testing.T, path, content string) {
	t.Helper()
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		t.Fatalf("Flock() error = %v", err)
	}
	t.Cleanup(func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	})
	if _, err := f.WriteString(content); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureStoppedSignalsLockHolder(t *testing.T) {
	path := withTempTrayPIDPath(t)
	signalled := fakeSignalTray(t)

	cleanup, _, alreadyRunning, err := ForegroundPIDGuard(4242)
	if err != nil || alreadyRunning {
		t.Fatalf("ForegroundPIDGuard() alreadyRunning = %v, error = %v", alreadyRunning, err)
	}

	if err := EnsureStopped(); err != nil {
		t.Fatalf("EnsureStopped() error = %v", err)
	}
	if want := []int{4242}; !slices.Equal(*signalled, want) {
		t.Fatalf("signalled = %v, want %v", *signalled, want)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected pid file to be removed, stat err = %v", err)
	}
	if err := cleanup(); err != nil {
		t.Fatalf("cleanup() error = %v", err)
	}
}

func TestEnsureStoppedUnlockedLivePIDIsStale(t *testing.T) {
	path := withTempTrayPIDPath(t)
	signalled := fakeSignalTray(t)

	if err := os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())), 0o600); err != nil {
		t.Fatalf("failed to write pid file: %v", err)
	}

	if err := EnsureStopped(); err != nil {
		t.Fatalf("EnsureStopped() error = %v", err)
	}
	if len(*signalled) != 0 {
		t.Fatalf("signalled = %v, want none", *signalled)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("expected pid file to be removed, stat err = %v", err)
	}
}

func TestEnsureStoppedInvalidPIDNotSignalled(t *testing.T) {
	for _, content := range []string{"-1", "0"} {
		t.Run(content, func(t *testing.T) {
			path := withTempTrayPIDPath(t)
			signalled := fakeSignalTray(t)
			holdTrayLock(t, path, content)

			if err := EnsureStopped(); err == nil {
				t.Errorf("EnsureStopped() error = nil, want an error for pid %s", content)
			}
			if len(*signalled) != 0 {
				t.Fatalf("signalled = %v, want none", *signalled)
			}
		})
	}
}

func TestEnsureStoppedNoFile(t *testing.T) {
	withTempTrayPIDPath(t)
	signalled := fakeSignalTray(t)

	for i := 0; i < 2; i++ {
		if err := EnsureStopped(); err != nil {
			t.Fatalf("EnsureStopped() call %d error = %v", i+1, err)
		}
	}
	if len(*signalled) != 0 {
		t.Fatalf("signalled = %v, want none", *signalled)
	}
}

func TestEnsureStoppedHolderWithoutPID(t *testing.T) {
	path := withTempTrayPIDPath(t)
	signalled := fakeSignalTray(t)
	holdTrayLock(t, path, "")

	if err := EnsureStopped(); err != nil {
		t.Fatalf("EnsureStopped() error = %v", err)
	}
	if len(*signalled) != 0 {
		t.Fatalf("signalled = %v, want none", *signalled)
	}
}
