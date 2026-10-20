package tray

import (
	"errors"
	"time"

	"github.com/jose/keepawake/internal/keepawake"
)

const (
	msgFinished     = "Keepawake finished — your Mac can sleep again"
	msgStoppedEarly = "Keepawake stopped early"
)

// session is what the tray remembers of one state reading.
type session struct {
	Active bool
	Until  time.Time
}

// transition decides whether going from prev to cur deserves a notification.
// A session that ends within one poll of its end time has finished; one that
// ends sooner was stopped early, unless the tray stopped it itself.
func transition(prev, cur session, now time.Time, poll time.Duration, selfCancelled bool) (notify bool, msg string) {
	if !prev.Active || cur.Active || selfCancelled {
		return false, ""
	}
	if !now.Before(prev.Until.Add(-poll)) {
		return true, msgFinished
	}
	return true, msgStoppedEarly
}

// sessionFrom maps a successful result of Manager.CurrentState to a session.
func sessionFrom(state *keepawake.State, err error) session {
	if err != nil || state == nil {
		return session{}
	}
	return session{Active: true, Until: state.Until}
}

// observe records a state reading and reports the notification to show, if
// any, and whether the session just ended. Failed readings are skipped.
func (a *App) observe(state *keepawake.State, err error, now time.Time) (msg string, ended bool) {
	if err != nil && !errors.Is(err, keepawake.ErrStateNotFound) {
		return "", false
	}

	cur := sessionFrom(state, err)
	selfCancelled := a.stoppedFromTray
	a.stoppedFromTray = false

	notify, m := transition(a.last, cur, now, a.pollInterval, selfCancelled)
	ended = a.last.Active && !cur.Active
	a.last = cur
	if !notify {
		return "", ended
	}
	return m, ended
}

// react records a state reading and, once a session the tray saw has ended,
// notifies first and then quits. Failed readings and a tray started with no
// session never quit; the tray's own stop quits without a notification.
func (a *App) react(state *keepawake.State, err error, now time.Time) (quit bool) {
	msg, ended := a.observe(state, err, now)
	if msg != "" {
		a.notify(msg)
	}
	if !ended {
		return false
	}
	a.quit()
	return true
}

// nextRefresh returns the wait before the next state reading: one poll when
// idle, otherwise until just after the end, capped at one poll. CurrentState
// reports the end only once now is after Until in whole seconds, hence the 1s.
func nextRefresh(cur session, now time.Time, poll time.Duration) time.Duration {
	if !cur.Active {
		return poll
	}
	wait := cur.Until.Add(time.Second).Sub(now)
	switch {
	case wait > poll:
		return poll
	case wait < 100*time.Millisecond:
		return 100 * time.Millisecond
	}
	return wait
}

// stopFromTray cancels the session so that its end does not notify.
func (a *App) stopFromTray() (int, error) {
	a.stoppedFromTray = true
	return a.manager.Cancel()
}
