package tray

import (
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/jose/keepawake/internal/keepawake"
)

// quitDialogMessage warns that quitting stops the session ending at until.
func quitDialogMessage(until, now time.Time) string {
	return fmt.Sprintf("A keepawake session is active until %s. Quitting will stop it and let your Mac sleep.", formatEnd(until, now))
}

// requestQuit quits at once when no session is active; otherwise it asks in
// the background and the answer comes back on quitAnswers. A click while the
// dialog is up asks nothing more.
func (a *App) requestQuit(now time.Time) (quitting bool) {
	if a.confirming {
		return false
	}
	if !a.last.Active {
		a.quit()
		return true
	}

	a.confirming = true
	msg := quitDialogMessage(a.last.Until, now)
	go func() {
		ok, err := a.confirmer.Confirm(msg)
		if err != nil {
			log.Printf("tray: quit dialog failed: %v", err)
			ok = false
		}
		a.quitAnswers <- ok
	}()
	return false
}

// confirmedQuit stops the session and quits once the dialog said yes.
func (a *App) confirmedQuit(ok bool) (quitting bool) {
	a.confirming = false
	if !ok {
		return false
	}
	if _, err := a.stopFromTray(); err != nil && !errors.Is(err, keepawake.ErrStateNotFound) {
		log.Printf("tray: failed to cancel keepawake: %v", err)
	}
	// The tray is already quitting: the next reading must neither notify nor quit.
	a.last = session{}
	a.quit()
	return true
}
