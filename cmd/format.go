package cmd

import (
	"fmt"
	"time"
)

func humanDuration(d time.Duration) string {
	if d < time.Minute {
		return d.Round(time.Second).String()
	}

	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60

	switch {
	case hours > 0 && minutes > 0:
		return fmt.Sprintf("%dh %dm", hours, minutes)
	case hours > 0:
		return fmt.Sprintf("%dh", hours)
	default:
		return fmt.Sprintf("%dm", minutes)
	}
}

// formatUntil formats t as a clock time, adding the date when t is not on now's day.
func formatUntil(now, t time.Time) string {
	ny, nm, nd := now.In(t.Location()).Date()
	ty, tm, td := t.Date()
	if ny == ty && nm == tm && nd == td {
		return t.Format("15:04")
	}
	return t.Format("Mon 2 Jan 15:04")
}
