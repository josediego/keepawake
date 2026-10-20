// Package procutil holds process helpers shared by keepawake and tray.
package procutil

import (
	"errors"
	"syscall"
)

// Alive reports whether a process with pid exists, using signal 0.
// EPERM means the process exists but belongs to another user, so it counts as alive.
func Alive(pid int) bool {
	if pid <= 0 {
		return false
	}
	err := syscall.Kill(pid, 0)
	return err == nil || errors.Is(err, syscall.EPERM)
}
