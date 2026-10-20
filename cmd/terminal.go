package cmd

import (
	"os"
	"syscall"
	"unsafe"
)

// isTerminal reports whether f is a terminal. Unlike os.ModeCharDevice, it is
// false for /dev/null.
func isTerminal(f *os.File) bool {
	var t syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCGETA), uintptr(unsafe.Pointer(&t)))
	return errno == 0
}
