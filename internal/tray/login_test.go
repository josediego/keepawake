package tray

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// plistItems parses p strictly and flattens it to element names, with the
// text of <key> and <string> elements as "name=text".
func plistItems(t *testing.T, p []byte) []string {
	t.Helper()
	d := xml.NewDecoder(bytes.NewReader(p))
	d.Strict = true
	var items []string
	var text *bytes.Buffer
	for {
		tok, err := d.Token()
		if err == io.EOF {
			return items
		}
		if err != nil {
			t.Fatalf("invalid XML: %v\n%s", err, p)
		}
		switch tok := tok.(type) {
		case xml.StartElement:
			if tok.Name.Local == "key" || tok.Name.Local == "string" {
				text = &bytes.Buffer{}
			} else {
				items = append(items, tok.Name.Local)
			}
		case xml.CharData:
			if text != nil {
				text.Write(tok)
			}
		case xml.EndElement:
			if text != nil {
				items = append(items, tok.Name.Local+"="+text.String())
				text = nil
			}
		}
	}
}

func TestLoginPlist(t *testing.T) {
	for _, exe := range []string{"/usr/local/bin/keepawake", `/tmp/a&b<c"d'/keepawake`} {
		p := LoginPlist(exe)
		if !bytes.HasPrefix(p, []byte("<?xml ")) {
			t.Errorf("missing XML declaration:\n%s", p)
		}
		if !bytes.Contains(p, []byte(`<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN"`)) {
			t.Errorf("missing plist doctype:\n%s", p)
		}
		want := []string{
			"plist", "dict",
			"key=Label", "string=" + LoginLabel,
			"key=ProgramArguments", "array", "string=" + exe, "string=tray", "string=--resident",
			"key=RunAtLoad", "true",
		}
		if got := plistItems(t, p); !reflect.DeepEqual(got, want) {
			t.Errorf("plist for %q:\n got %q\nwant %q", exe, got, want)
		}
	}
}

type fakeLaunchctl struct {
	calls        [][]string
	bootoutErr   error
	bootstrapErr error
}

func (f *fakeLaunchctl) run(args ...string) error {
	f.calls = append(f.calls, append([]string(nil), args...))
	switch args[0] {
	case "bootout":
		return f.bootoutErr
	case "bootstrap":
		return f.bootstrapErr
	}
	return nil
}

func newTestLoginAgent(t *testing.T) (*LoginAgent, *fakeLaunchctl) {
	fake := &fakeLaunchctl{}
	return &LoginAgent{Home: t.TempDir(), UID: 501, Launchctl: fake.run}, fake
}

func assertInstalled(t *testing.T, a *LoginAgent, want bool) {
	t.Helper()
	got, err := a.Installed()
	if err != nil {
		t.Fatalf("Installed: %v", err)
	}
	if got != want {
		t.Fatalf("Installed = %v, want %v", got, want)
	}
}

func TestLoginAgentInstall(t *testing.T) {
	a, fake := newTestLoginAgent(t)
	path := filepath.Join(a.Home, "Library", "LaunchAgents", "com.keepawake.tray.plist")
	if a.PlistPath() != path {
		t.Fatalf("PlistPath = %q, want %q", a.PlistPath(), path)
	}
	assertInstalled(t, a, false)

	loadCalls := [][]string{
		{"bootout", "gui/501/com.keepawake.tray"},
		{"bootstrap", "gui/501", path},
	}
	for i, exe := range []string{"/usr/local/bin/keepawake", "/opt/bin/keepawake"} {
		if err := a.Install(exe); err != nil {
			t.Fatalf("Install #%d: %v", i+1, err)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(data, LoginPlist(exe)) {
			t.Errorf("Install #%d wrote:\n%s", i+1, data)
		}
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o644 {
			t.Errorf("plist mode = %v, want 0644", info.Mode().Perm())
		}
		if !reflect.DeepEqual(fake.calls, loadCalls) {
			t.Errorf("Install #%d calls = %q, want %q", i+1, fake.calls, loadCalls)
		}
		fake.calls = nil
		assertInstalled(t, a, true)
	}
}

func TestLoginAgentInstallIgnoresBootoutFailure(t *testing.T) {
	a, fake := newTestLoginAgent(t)
	fake.bootoutErr = errors.New("exit status 3")
	if err := a.Install("/usr/local/bin/keepawake"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	assertInstalled(t, a, true)
}

func TestLoginAgentInstallReturnsBootstrapError(t *testing.T) {
	a, fake := newTestLoginAgent(t)
	fake.bootstrapErr = errors.New("exit status 5")
	if err := a.Install("/usr/local/bin/keepawake"); !errors.Is(err, fake.bootstrapErr) {
		t.Fatalf("Install error = %v, want %v", err, fake.bootstrapErr)
	}
}

func TestLoginAgentUninstall(t *testing.T) {
	a, fake := newTestLoginAgent(t)
	if err := a.Install("/usr/local/bin/keepawake"); err != nil {
		t.Fatalf("Install: %v", err)
	}
	fake.calls = nil

	bootout := [][]string{{"bootout", "gui/501/com.keepawake.tray"}}
	if err := a.Uninstall(); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	if !reflect.DeepEqual(fake.calls, bootout) {
		t.Errorf("Uninstall calls = %q, want %q", fake.calls, bootout)
	}
	if _, err := os.Stat(a.PlistPath()); !os.IsNotExist(err) {
		t.Errorf("plist still present: %v", err)
	}
	assertInstalled(t, a, false)

	// Again, with the agent no longer loaded.
	fake.calls = nil
	fake.bootoutErr = errors.New("exit status 3")
	if err := a.Uninstall(); err != nil {
		t.Fatalf("second Uninstall: %v", err)
	}
	if !reflect.DeepEqual(fake.calls, bootout) {
		t.Errorf("second Uninstall calls = %q, want %q", fake.calls, bootout)
	}
	assertInstalled(t, a, false)
}

func TestLoginAgentUninstallNeverInstalled(t *testing.T) {
	a, fake := newTestLoginAgent(t)
	fake.bootoutErr = errors.New("exit status 3")
	if err := a.Uninstall(); err != nil {
		t.Fatalf("Uninstall: %v", err)
	}
	assertInstalled(t, a, false)
}
