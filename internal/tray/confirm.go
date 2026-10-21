package tray

import (
	"errors"
	"os/exec"
	"strings"
)

const (
	dialogCancel   = "Cancel"
	dialogStopQuit = "Stop & Quit"
)

// Confirmer asks the user a yes/no question.
type Confirmer interface {
	Confirm(msg string) (bool, error)
}

// osascriptConfirmer asks through AppleScript's display dialog.
type osascriptConfirmer struct{}

func (osascriptConfirmer) Confirm(msg string) (bool, error) {
	return dialogAnswer(exec.Command("osascript", "-e", quitDialogScript(msg)).Output())
}

// quitDialogScript builds the AppleScript that asks whether to stop and quit.
func quitDialogScript(msg string) string {
	cancel := appleScriptString(dialogCancel)
	return "display dialog " + appleScriptString(msg) +
		" with title " + appleScriptString(notificationTitle) +
		" buttons {" + cancel + ", " + appleScriptString(dialogStopQuit) + "}" +
		" default button " + cancel +
		" cancel button " + cancel +
		" with icon caution"
}

// dialogAnswer reads the result of quitDialogScript. Exit status 1 is Cancel;
// only the exact Stop & Quit answer is a yes.
func dialogAnswer(out []byte, err error) (bool, error) {
	if err != nil {
		var exit interface{ ExitCode() int }
		if errors.As(err, &exit) && exit.ExitCode() == 1 {
			return false, nil
		}
		return false, err
	}
	return strings.TrimSpace(string(out)) == "button returned:"+dialogStopQuit, nil
}
