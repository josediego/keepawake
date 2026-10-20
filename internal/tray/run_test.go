package tray

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
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
