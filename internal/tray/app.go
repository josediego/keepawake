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
}

// NewApp builds a tray application bound to the provided manager.
func NewApp(manager *keepawake.Manager) *App {
	return &App{
		manager:      manager,
		pollInterval: 5 * time.Second,
	}
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
	ticker := time.NewTicker(a.pollInterval)
	defer ticker.Stop()

	// First immediate refresh
	a.refresh()

	for {
		select {
		case <-ticker.C:
			a.refresh()
		case <-a.cancelItem.ClickedCh:
			a.handleCancel()
		case <-a.quitItem.ClickedCh:
			systray.Quit()
			return
		case <-ctx.Done():
			return
		}
	}
}

func (a *App) refresh() {
	state, err := a.manager.CurrentState()
	if err != nil && !errors.Is(err, keepawake.ErrStateNotFound) {
		log.Printf("tray: could not read state: %v", err)
	}

	v := buildView(state, err, time.Now())
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

func (a *App) handleCancel() {
	pid, err := a.manager.Cancel()
	switch {
	case err == nil:
		log.Printf("tray: cancelled keepawake (pid %d)", pid)
	case errors.Is(err, keepawake.ErrStateNotFound):
		log.Printf("tray: cancel requested but no active session")
	default:
		log.Printf("tray: failed to cancel keepawake: %v", err)
		return
	}

	// Force refresh to update UI immediately
	a.refresh()
}
