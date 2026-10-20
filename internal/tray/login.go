package tray

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

// LoginLabel is the launchd label of the agent that starts the tray at login.
const LoginLabel = "com.keepawake.tray"

// LoginPlist returns the LaunchAgent plist that runs `<exe> tray --resident`
// at login.
func LoginPlist(exe string) []byte {
	var buf bytes.Buffer
	buf.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	`)
	writePlistString(&buf, LoginLabel)
	buf.WriteString(`
	<key>ProgramArguments</key>
	<array>
`)
	for _, arg := range []string{exe, "tray", "--resident"} {
		buf.WriteString("\t\t")
		writePlistString(&buf, arg)
		buf.WriteString("\n")
	}
	buf.WriteString(`	</array>
	<key>RunAtLoad</key>
	<true/>
</dict>
</plist>
`)
	return buf.Bytes()
}

func writePlistString(buf *bytes.Buffer, s string) {
	buf.WriteString("<string>")
	// Writes to a bytes.Buffer cannot fail.
	_ = xml.EscapeText(buf, []byte(s))
	buf.WriteString("</string>")
}

// LoginAgent installs and removes the LaunchAgent for the user's GUI
// session. Launchctl runs `launchctl` with args; tests replace it.
type LoginAgent struct {
	Home      string
	UID       int
	Launchctl func(args ...string) error
}

// NewLoginAgent returns a LoginAgent for the current user and the real
// launchctl.
func NewLoginAgent() (*LoginAgent, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("tray: resolve home: %w", err)
	}
	return &LoginAgent{Home: home, UID: os.Getuid(), Launchctl: runLaunchctl}, nil
}

func runLaunchctl(args ...string) error {
	out, err := exec.Command("launchctl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(out)))
	}
	return nil
}

// PlistPath is where the agent's plist is installed.
func (a *LoginAgent) PlistPath() string {
	return filepath.Join(a.Home, "Library", "LaunchAgents", LoginLabel+".plist")
}

func (a *LoginAgent) domain() string {
	return "gui/" + strconv.Itoa(a.UID)
}

func (a *LoginAgent) service() string {
	return a.domain() + "/" + LoginLabel
}

// Install writes the plist and (re)loads the agent; it is safe to repeat.
func (a *LoginAgent) Install(exe string) error {
	path := a.PlistPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("tray: create LaunchAgents dir: %w", err)
	}
	if err := os.WriteFile(path, LoginPlist(exe), 0o644); err != nil {
		return fmt.Errorf("tray: write login plist: %w", err)
	}
	// bootout fails when the agent is not loaded.
	_ = a.Launchctl("bootout", a.service())
	if err := a.Launchctl("bootstrap", a.domain(), path); err != nil {
		return fmt.Errorf("tray: bootstrap login agent: %w", err)
	}
	return nil
}

// Uninstall unloads the agent and removes its plist; it is safe to repeat.
func (a *LoginAgent) Uninstall() error {
	// bootout fails when the agent is not loaded.
	_ = a.Launchctl("bootout", a.service())
	if err := os.Remove(a.PlistPath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("tray: remove login plist: %w", err)
	}
	return nil
}

// Installed reports whether the agent's plist exists.
func (a *LoginAgent) Installed() (bool, error) {
	_, err := os.Stat(a.PlistPath())
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
