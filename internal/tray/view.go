package tray

import (
	"errors"
	"fmt"
	"time"

	"github.com/jose/keepawake/internal/keepawake"
)

// view is everything the tray displays for one state reading.
type view struct {
	Active        bool
	Title         string
	Tooltip       string
	StatusText    string
	CancelEnabled bool
	QuitEnabled   bool
}

// buildView maps the result of Manager.CurrentState to what the tray shows.
func buildView(state *keepawake.State, err error, now time.Time) view {
	switch {
	case err != nil && !errors.Is(err, keepawake.ErrStateNotFound):
		return view{
			StatusText:  "Status unavailable",
			Tooltip:     "Keepawake status unavailable",
			QuitEnabled: true,
		}
	case err != nil || state == nil:
		return view{
			StatusText:  "Inactive",
			Tooltip:     "Keepawake inactive",
			QuitEnabled: true,
		}
	}

	end := formatEnd(state.Until, now)
	remaining := compactDuration(state.Until.Sub(now))
	return view{
		Active:        true,
		Title:         remaining,
		StatusText:    fmt.Sprintf("Active until %s (%s left)", end, remaining),
		Tooltip:       fmt.Sprintf("Keepawake active until %s (%s left)", end, remaining),
		CancelEnabled: true,
	}
}

// formatEnd shows until in now's location, with the weekday if it is not today there.
func formatEnd(until, now time.Time) string {
	u := until.In(now.Location())
	uy, um, ud := u.Date()
	ny, nm, nd := now.Date()
	if uy == ny && um == nm && ud == nd {
		return u.Format("15:04")
	}
	return u.Format("Mon 15:04")
}

// compactDuration formats d truncated to the minute. Unlike cmd's humanDuration,
// anything below one minute (including negative values) is "<1m".
func compactDuration(d time.Duration) string {
	d = d.Truncate(time.Minute)
	if d < time.Minute {
		return "<1m"
	}
	h := d / time.Hour
	m := (d % time.Hour) / time.Minute
	switch {
	case h > 0 && m > 0:
		return fmt.Sprintf("%dh %dm", h, m)
	case h > 0:
		return fmt.Sprintf("%dh", h)
	default:
		return fmt.Sprintf("%dm", m)
	}
}
