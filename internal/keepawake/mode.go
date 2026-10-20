package keepawake

import (
	"fmt"
	"strconv"
	"time"
)

// Mode selects the caffeinate assertions a session holds. The zero Mode
// means DefaultMode.
type Mode struct {
	Display bool // -d: prevent the display from sleeping
	Idle    bool // -i: prevent the system from idle sleeping
	System  bool // -s: prevent the system from sleeping (AC power only)
	Disk    bool // -m: prevent the disk from idle sleeping
}

// DefaultMode keeps both the display and the system awake.
var DefaultMode = Mode{Display: true, Idle: true}

// String returns the mode's caffeinate letters in d, i, s, m order.
func (m Mode) String() string {
	var b []byte
	if m.Display {
		b = append(b, 'd')
	}
	if m.Idle {
		b = append(b, 'i')
	}
	if m.System {
		b = append(b, 's')
	}
	if m.Disk {
		b = append(b, 'm')
	}
	return string(b)
}

// orDefault returns DefaultMode for the zero Mode and m otherwise.
// caffeinate with no assertion flag only prevents idle sleep, so the zero
// Mode never maps to no flags.
func (m Mode) orDefault() Mode {
	if m == (Mode{}) {
		return DefaultMode
	}
	return m
}

// parseMode parses the canonical letters written by String. An empty string,
// as in state files written before modes existed, means DefaultMode.
func parseMode(s string) (Mode, error) {
	if s == "" {
		return DefaultMode, nil
	}
	var mode Mode
	for _, c := range s {
		switch c {
		case 'd':
			mode.Display = true
		case 'i':
			mode.Idle = true
		case 's':
			mode.System = true
		case 'm':
			mode.Disk = true
		default:
			return Mode{}, fmt.Errorf("unknown mode %q", s)
		}
	}
	// Rejects repeated and out-of-order letters.
	if mode.String() != s {
		return Mode{}, fmt.Errorf("unknown mode %q", s)
	}
	return mode, nil
}

// caffeinateArgs returns the caffeinate arguments for a session of mode
// lasting d: one combined flag, then -t and the seconds rounded up, at least 1.
func caffeinateArgs(mode Mode, d time.Duration) []string {
	secs := int64((d + time.Second - 1) / time.Second)
	if secs < 1 {
		secs = 1
	}
	return []string{"-" + mode.orDefault().String(), "-t", strconv.FormatInt(secs, 10)}
}
