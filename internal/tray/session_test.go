package tray

import (
	"errors"
	"go/parser"
	"go/token"
	"path/filepath"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/jose/keepawake/internal/keepawake"
)

func TestTransition(t *testing.T) {
	const poll = 5 * time.Second
	until := time.Date(2026, 10, 20, 17, 0, 0, 0, time.UTC)
	active := session{Active: true, Until: until}
	inactive := session{}

	tests := []struct {
		name          string
		prev, cur     session
		now           time.Time
		selfCancelled bool
		wantNotify    bool
		wantMsg       string
	}{
		{"natural expiry", active, inactive, until.Add(2 * time.Second), false, true, msgFinished},
		{"expiry seen exactly one poll early", active, inactive, until.Add(-poll), false, true, msgFinished},
		{"just over one poll early", active, inactive, until.Add(-poll - time.Nanosecond), false, true, msgStoppedEarly},
		{"early stop", active, inactive, until.Add(-time.Hour), false, true, msgStoppedEarly},
		{"self-cancel early", active, inactive, until.Add(-time.Hour), true, false, ""},
		{"self-cancel late", active, inactive, until.Add(time.Second), true, false, ""},
		{"inactive to inactive", inactive, inactive, until, false, false, ""},
		{"inactive to active", inactive, active, until.Add(-time.Hour), false, false, ""},
		{"active to active with a new end", active, session{Active: true, Until: until.Add(time.Hour)}, until.Add(-time.Minute), false, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			notify, msg := transition(tt.prev, tt.cur, tt.now, poll, tt.selfCancelled)
			if notify != tt.wantNotify || msg != tt.wantMsg {
				t.Errorf("transition() = (%v, %q), want (%v, %q)", notify, msg, tt.wantNotify, tt.wantMsg)
			}
		})
	}
}

func TestObserveSequence(t *testing.T) {
	a := &App{pollInterval: 5 * time.Second}
	start := time.Date(2026, 10, 20, 16, 0, 0, 0, time.UTC)
	until := start.Add(time.Hour)
	active := &keepawake.State{PID: 1, Until: until}

	steps := []struct {
		name      string
		state     *keepawake.State
		err       error
		now       time.Time
		selfStop  bool // the tray stops the session before this reading
		wantMsg   string
		wantEnded bool
	}{
		{name: "session starts", state: active, now: start},
		{name: "failed read is skipped", err: errors.New("boom"), now: start.Add(time.Minute)},
		{name: "early stop notifies", err: keepawake.ErrStateNotFound, now: start.Add(2 * time.Minute), wantMsg: msgStoppedEarly, wantEnded: true},
		{name: "new session", state: active, now: start.Add(3 * time.Minute)},
		{name: "tray's own stop is silent", err: keepawake.ErrStateNotFound, now: start.Add(4 * time.Minute), selfStop: true, wantEnded: true},
		{name: "another session", state: active, now: start.Add(5 * time.Minute)},
		{name: "flag does not leak into a later stop", err: keepawake.ErrStateNotFound, now: start.Add(6 * time.Minute), wantMsg: msgStoppedEarly, wantEnded: true},
		{name: "last session", state: active, now: start.Add(7 * time.Minute)},
		{name: "expiry notifies", err: keepawake.ErrStateNotFound, now: until.Add(time.Second), wantMsg: msgFinished, wantEnded: true},
		{name: "still inactive", err: keepawake.ErrStateNotFound, now: until.Add(6 * time.Second)},
	}
	for i, s := range steps {
		if s.selfStop {
			a.stoppedFromTray = true
		}
		msg, ended := a.observe(s.state, s.err, s.now)
		if msg != s.wantMsg || ended != s.wantEnded {
			t.Fatalf("step %d (%s): observe() = (%q, %v), want (%q, %v)", i, s.name, msg, ended, s.wantMsg, s.wantEnded)
		}
	}
}

type fakeProcesses struct{}

func (fakeProcesses) IsCaffeinate(int) bool { return false }
func (fakeProcesses) Terminate(int) error   { return nil }

func TestStopFromTraySuppressesNotification(t *testing.T) {
	manager, err := keepawake.NewManager(
		keepawake.WithStateFile(filepath.Join(t.TempDir(), "state")),
		keepawake.WithProcessController(fakeProcesses{}),
	)
	if err != nil {
		t.Fatalf("NewManager() error = %v", err)
	}
	a := NewApp(manager, WithoutNotifications())
	now := time.Date(2026, 10, 20, 16, 0, 0, 0, time.UTC)

	if msg, ended := a.observe(&keepawake.State{PID: 1, Until: now.Add(time.Hour)}, nil, now); msg != "" || ended {
		t.Fatalf("observe(active) = (%q, %v), want (\"\", false)", msg, ended)
	}
	if _, err := a.stopFromTray(); !errors.Is(err, keepawake.ErrStateNotFound) {
		t.Fatalf("stopFromTray() error = %v, want ErrStateNotFound", err)
	}
	if msg, ended := a.observe(nil, keepawake.ErrStateNotFound, now.Add(time.Minute)); msg != "" || !ended {
		t.Fatalf("observe(after stop) = (%q, %v), want (\"\", true)", msg, ended)
	}
}

type fakeNotifier struct {
	calls chan [2]string
}

func (f fakeNotifier) Notify(title, msg string) error {
	f.calls <- [2]string{title, msg}
	return errors.New("notifier errors are only logged")
}

func TestNotifyUsesNotifier(t *testing.T) {
	f := fakeNotifier{calls: make(chan [2]string, 1)}
	a := &App{notifier: f}

	a.notify(msgFinished)

	select {
	case got := <-f.calls:
		if want := [2]string{notificationTitle, msgFinished}; got != want {
			t.Errorf("Notify(%q, %q), want Notify(%q, %q)", got[0], got[1], want[0], want[1])
		}
	case <-time.After(time.Second):
		t.Fatal("notifier was not called")
	}
}

type recordingNotifier struct {
	events *[]string
}

func (r recordingNotifier) Notify(_, msg string) error {
	*r.events = append(*r.events, "notify: "+msg)
	return nil
}

func TestReactSequence(t *testing.T) {
	start := time.Date(2026, 10, 20, 16, 0, 0, 0, time.UTC)
	until := start.Add(time.Hour)

	tests := []struct {
		name     string
		endAt    time.Time
		selfStop bool
		want     []string
	}{
		{"expiry", until.Add(time.Second), false, []string{"notify: " + msgFinished, "quit"}},
		{"early end", start.Add(10 * time.Minute), false, []string{"notify: " + msgStoppedEarly, "quit"}},
		{"tray's own stop", start.Add(10 * time.Minute), true, []string{"quit"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var events []string
			a := &App{
				pollInterval: 5 * time.Second,
				notifier:     recordingNotifier{events: &events},
				quit:         func() { events = append(events, "quit") },
			}

			before := []struct {
				name  string
				state *keepawake.State
				err   error
				now   time.Time
			}{
				{"idle at start", nil, keepawake.ErrStateNotFound, start.Add(-time.Minute)},
				{"session starts", &keepawake.State{PID: 1, Until: until}, nil, start},
				{"failed read", nil, errors.New("boom"), start.Add(time.Minute)},
			}
			for _, s := range before {
				if a.react(s.state, s.err, s.now) || len(events) != 0 {
					t.Fatalf("%s: react() quit or reported %v before the end", s.name, events)
				}
			}

			if tt.selfStop {
				a.stoppedFromTray = true
			}
			if !a.react(nil, keepawake.ErrStateNotFound, tt.endAt) {
				t.Fatal("react(end) = false, want true")
			}
			if !reflect.DeepEqual(events, tt.want) {
				t.Errorf("events = %q, want %q", events, tt.want)
			}
		})
	}
}

func TestNextRefresh(t *testing.T) {
	const poll = 5 * time.Second
	now := time.Date(2026, 10, 20, 16, 0, 0, 0, time.UTC)

	tests := []struct {
		name string
		cur  session
		want time.Duration
	}{
		{"idle", session{}, poll},
		{"far from the end", session{Active: true, Until: now.Add(time.Hour)}, poll},
		{"end within one poll", session{Active: true, Until: now.Add(2 * time.Second)}, 3 * time.Second},
		{"end just passed", session{Active: true, Until: now.Add(-500 * time.Millisecond)}, 500 * time.Millisecond},
		{"end already past", session{Active: true, Until: now.Add(-10 * time.Second)}, 100 * time.Millisecond},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := nextRefresh(tt.cur, now, poll); got != tt.want {
				t.Errorf("nextRefresh() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestWithoutNotificationsDisablesNotifier(t *testing.T) {
	if a := NewApp(nil); a.notifier == nil {
		t.Error("NewApp() has no notifier, want the default")
	}
	if a := NewApp(nil, WithoutNotifications()); a.notifier != nil {
		t.Errorf("NewApp(WithoutNotifications()) notifier = %v, want nil", a.notifier)
	}
}

func TestSessionFilesDoNotImportSystray(t *testing.T) {
	for _, name := range []string{"session.go", "notify.go", "quit.go", "confirm.go"} {
		f, err := parser.ParseFile(token.NewFileSet(), name, nil, parser.ImportsOnly)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		for _, imp := range f.Imports {
			path, err := strconv.Unquote(imp.Path.Value)
			if err != nil {
				t.Fatalf("unquote import %s: %v", imp.Path.Value, err)
			}
			if path == "github.com/getlantern/systray" {
				t.Fatalf("%s must not import %s", name, path)
			}
		}
	}
}
