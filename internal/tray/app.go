package tray

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/getlantern/systray"
	"github.com/jose/keepawake/internal/keepawake"
)

// App manages the system tray experience.
type App struct {
	manager      *keepawake.Manager
	pollInterval time.Duration

	statusItem *systray.MenuItem
	cancelItem *systray.MenuItem
	quitItem   *systray.MenuItem

	lastActive bool

	last            session
	stoppedFromTray bool
	notifier        Notifier // nil disables notifications
	quit            func()   // closes the tray; tests replace it
}

// Option customizes an App.
type Option func(*App)

// WithoutNotifications stops the tray from notifying when a session ends.
func WithoutNotifications() Option {
	return func(a *App) {
		a.notifier = nil
	}
}

// NewApp builds a tray application bound to the provided manager.
func NewApp(manager *keepawake.Manager, opts ...Option) *App {
	a := &App{
		manager:      manager,
		pollInterval: 5 * time.Second,
		notifier:     osascriptNotifier{},
	}
	a.quit = a.quitTray
	for _, opt := range opts {
		opt(a)
	}
	return a
}

// Run starts the tray loop and blocks until the app exits or context is cancelled.
func (a *App) Run(ctx context.Context) error {
	if ctx == nil {
		ctx = context.Background()
	}

	exitErr := make(chan error, 1)

	go func() {
		<-ctx.Done()
		systray.Quit()
	}()

	systray.Run(func() {
		a.onReady(ctx)
	}, func() {
		exitErr <- ctx.Err()
	})

	err := <-exitErr
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return nil
	}
	return err
}

func (a *App) onReady(ctx context.Context) {
	systray.SetIcon(iconInactive)
	systray.SetTemplateIcon(iconInactive, iconInactive)
	systray.SetTooltip("Keepawake – status in menubar")

	a.statusItem = systray.AddMenuItem("Status: inactive", "Current status of Keepawake")
	a.statusItem.Disable()
	a.cancelItem = systray.AddMenuItem("Stop", "Stop the current keepawake session")
	a.cancelItem.Disable()
	systray.AddSeparator()
	a.quitItem = systray.AddMenuItem("Quit tray", "Close the Keepawake icon")
	a.quitItem.Enable()

	go a.watchState(ctx)
}

func (a *App) watchState(ctx context.Context) {
	// First immediate refresh
	timer := time.NewTimer(a.refresh())
	defer timer.Stop()

	for {
		select {
		case <-timer.C:
			timer.Reset(a.refresh())
		case <-a.cancelItem.ClickedCh:
			timer.Reset(a.handleCancel())
		case <-a.quitItem.ClickedCh:
			systray.Quit()
			return
		case <-ctx.Done():
			return
		}
	}
}

// refresh reads the state, updates the menu and returns the wait until the next refresh.
func (a *App) refresh() time.Duration {
	state, err := a.manager.CurrentState()
	if err != nil && !errors.Is(err, keepawake.ErrStateNotFound) {
		log.Printf("tray: could not read state: %v", err)
	}

	now := time.Now()
	if a.react(state, err, now) {
		return a.pollInterval
	}

	v := buildView(state, err, now)
	if icon, changed := a.iconChange(v.Active); changed {
		systray.SetIcon(icon)
		systray.SetTemplateIcon(icon, icon)
	}
	systray.SetTitle(v.Title)
	systray.SetTooltip(v.Tooltip)
	if a.statusItem != nil {
		a.statusItem.SetTitle(v.StatusText)
	}
	setEnabled(a.cancelItem, v.CancelEnabled)
	setEnabled(a.quitItem, v.QuitEnabled)
	return nextRefresh(a.last, now, a.pollInterval)
}

// quitTray switches to the closed eye and quits systray.
func (a *App) quitTray() {
	systray.SetIcon(iconInactive)
	systray.SetTemplateIcon(iconInactive, iconInactive)
	systray.Quit()
}

// iconChange reports the icon to set when the active state differs from the last one.
func (a *App) iconChange(active bool) (icon []byte, changed bool) {
	if active == a.lastActive {
		return nil, false
	}
	a.lastActive = active
	if active {
		return iconActive, true
	}
	return iconInactive, true
}

// notify shows msg and waits for it, so the notice is shown before the tray quits.
func (a *App) notify(msg string) {
	if a.notifier == nil {
		return
	}
	if err := a.notifier.Notify(notificationTitle, msg); err != nil {
		log.Printf("tray: could not notify: %v", err)
	}
}

func setEnabled(item *systray.MenuItem, enabled bool) {
	if item == nil {
		return
	}
	if enabled {
		item.Enable()
		return
	}
	item.Disable()
}

func (a *App) handleCancel() time.Duration {
	pid, err := a.stopFromTray()
	switch {
	case err == nil:
		log.Printf("tray: cancelled keepawake (pid %d)", pid)
	case errors.Is(err, keepawake.ErrStateNotFound):
		log.Printf("tray: cancel requested but no active session")
	default:
		log.Printf("tray: failed to cancel keepawake: %v", err)
	}

	// Force refresh to update UI immediately; the end of the session quits the tray
	return a.refresh()
}
