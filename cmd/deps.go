package cmd

import (
	"context"
	"time"

	"github.com/jose/keepawake/internal/keepawake"
	"github.com/jose/keepawake/internal/tray"
)

// sessionManager starts and cancels sessions; *keepawake.Manager satisfies it.
type sessionManager interface {
	StartUntil(time.Time) (int, time.Duration, error)
	For(time.Duration) (int, time.Time, error)
	Cancel() (int, error)
}

var _ sessionManager = (*keepawake.Manager)(nil)

// sessionDeps holds what until, for and cancel need from the outside world, so tests can fake them.
type sessionDeps struct {
	manager    func(keepawake.Mode) (sessionManager, error) // cancel passes the zero Mode; only starts use it
	launchTray func() error
}

// defaultSessionDeps builds the manager only when a command runs, so building the commands reads no state.
func defaultSessionDeps() sessionDeps {
	return sessionDeps{
		manager: func(mode keepawake.Mode) (sessionManager, error) {
			return keepawake.NewManager(keepawake.WithMode(mode))
		},
		launchTray: tray.EnsureRunning,
	}
}

// trayRunner runs the tray until it exits; *tray.App satisfies it.
type trayRunner interface {
	Run(ctx context.Context) error
}

var _ trayRunner = (*tray.App)(nil)

// trayDeps holds what the tray command needs from the outside world, so tests can fake them.
type trayDeps struct {
	newApp func(opts ...tray.Option) (trayRunner, error)
	guard  func(pid int) (cleanup func() error, existingPID int, alreadyRunning bool, err error)
}

func defaultTrayDeps() trayDeps {
	return trayDeps{
		newApp: func(opts ...tray.Option) (trayRunner, error) {
			manager, err := keepawake.NewManager()
			if err != nil {
				return nil, err
			}
			return tray.NewApp(manager, opts...), nil
		},
		guard: tray.ForegroundPIDGuard,
	}
}
