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

// stopFromTray cancels the session so that its end does not notify.
func (a *App) stopFromTray() (int, error) {
	a.stoppedFromTray = true
	return a.manager.Cancel()
}
