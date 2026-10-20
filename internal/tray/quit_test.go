package tray

import (
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jose/keepawake/internal/keepawake"
)

// quitEvents records, in order, what the fakes saw.
type quitEvents struct {
	mu   sync.Mutex
	list []string
}

func (e *quitEvents) add(s string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.list = append(e.list, s)
}

func (e *quitEvents) get() []string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return append([]string(nil), e.list...)
}

type fakeSource struct{ events *quitEvents }

func (f fakeSource) CurrentState() (*keepawake.State, error) { return nil, keepawake.ErrStateNotFound }
func (f fakeSource) Cancel() (int, error) {
	f.events.add("stop")
	return 1, nil
}

type fakeConfirmer struct {
	events  *quitEvents
	release chan struct{} // nil answers at once
	ok      bool
	err     error
}

func (f fakeConfirmer) Confirm(string) (bool, error) {
	f.events.add("confirm")
	if f.release != nil {
		<-f.release
	}
	return f.ok, f.err
}

var quitStart = time.Date(2026, 10, 20, 16, 0, 0, 0, time.UTC)

// newQuitApp returns an App that has seen an active session, when active is set.
func newQuitApp(t *testing.T, c Confirmer, events *quitEvents, active bool) *App {
	t.Helper()
	a := &App{
		manager:      fakeSource{events: events},
		pollInterval: 5 * time.Second,
		confirmer:    c,
		quitAnswers:  make(chan bool, 1),
		quit:         func() { events.add("quit") },
		notifier:     notifierFunc(func(msg string) { events.add("notify: " + msg) }),
	}
	if active {
		if a.react(&keepawake.State{PID: 1, Until: quitStart.Add(time.Hour)}, nil, quitStart) {
			t.Fatal("react(active) quit")
		}
	}
	return a
}

type notifierFunc func(msg string)

func (f notifierFunc) Notify(_, msg string) error {
	f(msg)
	return nil
}

func answer(t *testing.T, a *App) bool {
	t.Helper()
	select {
	case ok := <-a.quitAnswers:
		return ok
	case <-time.After(time.Second):
		t.Fatal("no answer from the quit dialog")
		return false
	}
}

func TestRequestQuitWhileActive(t *testing.T) {
	tests := []struct {
		name  string
		ok    bool
		err   error
		quits bool
		want  []string
	}{
		{"confirm", true, nil, true, []string{"confirm", "stop", "quit"}},
		{"cancel", false, nil, false, []string{"confirm"}},
		{"dialog fails", true, errors.New("boom"), false, []string{"confirm"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			events := &quitEvents{}
			a := newQuitApp(t, fakeConfirmer{events: events, ok: tt.ok, err: tt.err}, events, true)

			if a.requestQuit(quitStart.Add(time.Minute)) {
				t.Fatal("requestQuit() = true while active, want false")
			}
			if got := a.confirmedQuit(answer(t, a)); got != tt.quits {
				t.Fatalf("confirmedQuit() = %v, want %v", got, tt.quits)
			}
			if got := events.get(); !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("events = %q, want %q", got, tt.want)
			}
			if !tt.quits {
				return
			}

			// The reading after the tray's own stop neither notifies nor quits.
			if a.react(nil, keepawake.ErrStateNotFound, quitStart.Add(2*time.Minute)) {
				t.Error("react() after the stop = true, want false")
			}
			if got := events.get(); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("events after the next reading = %q, want %q", got, tt.want)
			}
		})
	}
}

type refusingConfirmer struct{ t *testing.T }

func (r refusingConfirmer) Confirm(string) (bool, error) {
	r.t.Error("Confirm() called with no active session")
	return false, nil
}

func TestRequestQuitWhileIdle(t *testing.T) {
	events := &quitEvents{}
	a := newQuitApp(t, refusingConfirmer{t}, events, false)

	if !a.requestQuit(quitStart) {
		t.Fatal("requestQuit() = false while idle, want true")
	}
	if got, want := events.get(), []string{"quit"}; !reflect.DeepEqual(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}
}

func TestRequestQuitDoesNotWaitForTheDialog(t *testing.T) {
	events := &quitEvents{}
	release := make(chan struct{})
	a := newQuitApp(t, fakeConfirmer{events: events, release: release, ok: false}, events, true)

	returned := make(chan bool)
	go func() {
		a.requestQuit(quitStart)
		returned <- true
	}()
	select {
	case <-returned:
	case <-time.After(time.Second):
		close(release)
		t.Fatal("requestQuit() waited for the dialog")
	}

	// A second click while the dialog is up asks nothing more.
	if a.requestQuit(quitStart) {
		t.Fatal("second requestQuit() = true, want false")
	}
	close(release)
	if answer(t, a) {
		t.Fatal("answer = true, want false")
	}
	select {
	case <-a.quitAnswers:
		t.Fatal("second dialog answered")
	case <-time.After(50 * time.Millisecond):
	}
	if got, want := events.get(), []string{"confirm"}; !reflect.DeepEqual(got, want) {
		t.Errorf("events = %q, want %q", got, want)
	}
}

func TestQuitDialogMessage(t *testing.T) {
	loc := time.FixedZone("T", -3*3600)
	now := time.Date(2026, 10, 20, 12, 0, 0, 0, loc)

	today := quitDialogMessage(time.Date(2026, 10, 20, 17, 0, 0, 0, loc), now)
	if want := "A keepawake session is active until 17:00. Quitting will stop it and let your Mac sleep."; today != want {
		t.Errorf("message = %q, want %q", today, want)
	}
	tomorrow := quitDialogMessage(time.Date(2026, 10, 21, 17, 0, 0, 0, loc), now)
	if !strings.Contains(tomorrow, "until Wed 17:00.") {
		t.Errorf("message = %q, want the weekday", tomorrow)
	}
}
