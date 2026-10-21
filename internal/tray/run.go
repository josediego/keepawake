package tray

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/jose/keepawake/internal/procutil"
)

const trayPIDFileName = ".keepawake.tray.pid"

// EnsureRunning starts the tray process in the background if it's not active yet.
func EnsureRunning() error {
	pid, err := readTrayPID()
	if err == nil {
		if alive(pid) {
			return nil
		}
		_ = removeTrayPID()
	} else if !errors.Is(err, errPIDNotFound) {
		return err
	}

	exe, err := os.Executable()
	if err != nil {
		return fmt.Errorf("tray: resolve executable: %w", err)
	}

	cmd := exec.Command(exe, "tray")
	nullFile, _ := os.OpenFile(os.DevNull, os.O_RDWR, 0o666)
	if nullFile != nil {
		defer nullFile.Close()
		cmd.Stdout = nullFile
		cmd.Stderr = nullFile
	}

	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("tray: start background process: %w", err)
	}

	_ = cmd.Process.Release()

	return waitForTrayReady(3 * time.Second)
}

var errPIDNotFound = errors.New("tray pid not found")

func readTrayPID() (int, error) {
	path, err := trayPIDPath()
	if err != nil {
		return 0, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, errPIDNotFound
		}
		return 0, err
	}
	pidStr := strings.TrimSpace(string(data))
	if pidStr == "" {
		// The guard creates the file before writing its PID under the lock.
		return 0, errPIDNotFound
	}
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("tray: invalid pid %q: %w", pidStr, err)
	}
	return pid, nil
}

func removeTrayPID() error {
	path, err := trayPIDPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// trayPIDPath resolves the tray PID/lock file; tests override it.
var trayPIDPath = defaultTrayPIDPath

func defaultTrayPIDPath() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	return filepath.Join(usr.HomeDir, trayPIDFileName), nil
}

func alive(pid int) bool {
	return procutil.Alive(pid)
}

// ForegroundPIDGuard registers the caller as the active tray process by
// holding an exclusive advisory lock on the PID file for the tray's lifetime,
// and returns a cleanup function that removes the file and releases the lock.
// If another live tray holds the lock, alreadyRunning=true and existingPID
// reports it. The kernel releases the lock if the holder dies, so a stale
// file is simply acquired again.
func ForegroundPIDGuard(pid int) (cleanup func() error, existingPID int, alreadyRunning bool, err error) {
	path, err := trayPIDPath()
	if err != nil {
		return nil, 0, false, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, 0, false, err
	}
	fd := int(f.Fd())
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		if errors.Is(err, syscall.EWOULDBLOCK) {
			// Best effort: the holder may not have written its PID yet.
			existingPID, _ = readTrayPID()
			return nil, existingPID, true, nil
		}
		return nil, 0, false, fmt.Errorf("tray: lock pid file: %w", err)
	}
	if err := writeLockedPID(f, pid); err != nil {
		_ = syscall.Flock(fd, syscall.LOCK_UN)
		_ = f.Close()
		return nil, 0, false, err
	}
	return func() error {
		// Remove while still holding the lock so no contender locks the old inode.
		removeErr := removeTrayPID()
		_ = syscall.Flock(fd, syscall.LOCK_UN)
		if err := f.Close(); err != nil {
			return err
		}
		return removeErr
	}, pid, false, nil
}

func writeLockedPID(f *os.File, pid int) error {
	if err := f.Truncate(0); err != nil {
		return err
	}
	_, err := f.WriteAt([]byte(strconv.Itoa(pid)), 0)
	return err
}

// signalTray sends sig to pid; tests override it.
var signalTray = syscall.Kill

// EnsureStopped terminates the background tray if one is running. It signals
// only a tray that holds the PID file lock; an unlocked file is stale and is
// removed without signalling its PID, which may belong to another process.
func EnsureStopped() error {
	path, err := trayPIDPath()
	if err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	defer f.Close()

	fd := int(f.Fd())
	if err := syscall.Flock(fd, syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
		// Remove while still holding the lock so no contender locks the old inode.
		removeErr := removeTrayPID()
		_ = syscall.Flock(fd, syscall.LOCK_UN)
		return removeErr
	} else if !errors.Is(err, syscall.EWOULDBLOCK) {
		return fmt.Errorf("tray: lock pid file: %w", err)
	}

	pid, err := readTrayPID()
	if err != nil {
		if errors.Is(err, errPIDNotFound) {
			return nil
		}
		return err
	}
	if pid <= 0 {
		// kill(-N) would signal a whole process group.
		return fmt.Errorf("tray: invalid pid %d", pid)
	}
	if err := signalTray(pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return fmt.Errorf("tray: signal pid %d: %w", pid, err)
	}
	return removeTrayPID()
}

func waitForTrayReady(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		pid, err := readTrayPID()
		if err == nil && alive(pid) {
			return nil
		}
		if err != nil && !errors.Is(err, errPIDNotFound) {
			return err
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("tray: did not register within %s", timeout)
}
