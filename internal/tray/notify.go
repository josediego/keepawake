package tray

import (
	"os/exec"
	"strings"
)

const notificationTitle = "Keepawake"

// Notifier shows a desktop notification.
type Notifier interface {
	Notify(title, msg string) error
}

// osascriptNotifier shows notifications through AppleScript's display notification.
type osascriptNotifier struct{}

func (osascriptNotifier) Notify(title, msg string) error {
	return exec.Command("osascript", "-e", notificationScript(title, msg)).Run()
}

// notificationScript builds the AppleScript that shows msg under title.
func notificationScript(title, msg string) string {
	return "display notification " + appleScriptString(msg) + " with title " + appleScriptString(title)
}

// appleScriptString quotes s as an AppleScript string literal.
func appleScriptString(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return `"` + s + `"`
}
