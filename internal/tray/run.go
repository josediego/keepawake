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
	startedNewTray := false

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

	startedNewTray = true
	_ = cmd.Process.Release()

	if startedNewTray {
		if err := waitForTrayReady(3 * time.Second); err != nil {
			return err
		}
	}

	return nil
}

var errPIDNotFound = errors.New("tray pid not found")

func readTrayPID() (int, error) {
	path, err := trayPIDFilePath()
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
	pid, err := strconv.Atoi(pidStr)
	if err != nil {
		return 0, fmt.Errorf("tray: invalid pid %q: %w", pidStr, err)
	}
	return pid, nil
}

func saveTrayPID(pid int) error {
	path, err := trayPIDFilePath()
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strconv.Itoa(pid)), 0o644)
}

func removeTrayPID() error {
	path, err := trayPIDFilePath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func trayPIDFilePath() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	return filepath.Join(usr.HomeDir, trayPIDFileName), nil
}

func alive(pid int) bool {
	return procutil.Alive(pid)
}

// ForegroundPIDGuard registers the caller as the active tray process and
// returns a cleanup function to clear the PID file when finished. If a live
// tray already exists, alreadyRunning=true and existingPID reports it.
func ForegroundPIDGuard(pid int) (cleanup func() error, existingPID int, alreadyRunning bool, err error) {
	currentPID, alive, err := currentTrayPID()
	if err != nil {
		return nil, 0, false, err
	}
	if alive {
		return nil, currentPID, true, nil
	}
	if err := saveTrayPID(pid); err != nil {
		return nil, 0, false, err
	}
	return func() error {
		return removeTrayPID()
	}, pid, false, nil
}

func currentTrayPID() (pid int, aliveNow bool, err error) {
	pid, err = readTrayPID()
	if err != nil {
		if errors.Is(err, errPIDNotFound) {
			return 0, false, nil
		}
		return 0, false, err
	}
	if alive(pid) {
		return pid, true, nil
	}
	if err := removeTrayPID(); err != nil {
		return 0, false, err
	}
	return 0, false, nil
}

// EnsureStopped terminates background tray process if running.
func EnsureStopped() error {
	pid, err := readTrayPID()
	if err != nil {
		if errors.Is(err, errPIDNotFound) {
			return nil
		}
		return err
	}
	proc, err := os.FindProcess(pid)
	if err == nil {
		_ = proc.Signal(syscall.SIGTERM)
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
