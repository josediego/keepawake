package keepawake

import (
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"github.com/jose/keepawake/internal/procutil"
)

// ProcessController inspects and stops the process recorded in the state file.
type ProcessController interface {
	// IsCaffeinate reports whether pid is alive and is a caffeinate process.
	IsCaffeinate(pid int) bool
	// Terminate asks pid to exit; a process that is already gone is not an error.
	Terminate(pid int) error
}

type defaultProcessController struct{}

func (defaultProcessController) IsCaffeinate(pid int) bool {
	if !procutil.Alive(pid) {
		return false
	}
	out, err := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "comm=").Output()
	if err != nil {
		return false
	}
	return isCaffeinateComm(string(out))
}

func (defaultProcessController) Terminate(pid int) error {
	if pid <= 0 {
		return fmt.Errorf("refusing to signal non-positive pid %d", pid)
	}
	if err := syscall.Kill(pid, syscall.SIGTERM); err != nil && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

// isCaffeinateComm reports whether ps "comm=" output names caffeinate.
// ps prints the full executable path, so only the base name is compared.
func isCaffeinateComm(out string) bool {
	return filepath.Base(strings.TrimSpace(out)) == "caffeinate"
}
