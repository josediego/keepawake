package cmd

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

type fakeStarter struct {
	now   time.Time
	calls []time.Time
}

func (f *fakeStarter) StartUntil(t time.Time) (int, time.Duration, error) {
	f.calls = append(f.calls, t)
	return 123, t.Sub(f.now), nil
}

func TestRunUntil(t *testing.T) {
	now := time.Date(2026, time.October, 17, 10, 0, 0, 0, time.Local)
	tomorrow := time.Date(2026, time.October, 18, 8, 0, 0, 0, time.Local)
	today := time.Date(2026, time.October, 17, 17, 0, 0, 0, time.Local)
	const prompt = "08:00 already passed today. Keep awake until tomorrow 08:00 (Sun 18 Oct)? [y/N] "
	const started = "Keepawake active (PID 123) for 22h (until Sun 18 Oct 08:00)\n"
	const aborted = "aborted: nothing was changed"

	tests := []struct {
		name      string
		target    string
		input     string
		yes       bool
		notTTY    bool
		wantStart time.Time
		wantErr   string
		wantOut   string
	}{
		{name: "y", target: "08:00", input: "y\n", wantStart: tomorrow, wantOut: prompt + started},
		{name: "yes", target: "08:00", input: "yes\n", wantStart: tomorrow, wantOut: prompt + started},
		{name: "Y", target: "08:00", input: "Y\n", wantStart: tomorrow, wantOut: prompt + started},
		{name: "n", target: "08:00", input: "n\n", wantErr: aborted, wantOut: prompt},
		{name: "empty line", target: "08:00", input: "\n", wantErr: aborted, wantOut: prompt},
		{name: "foo", target: "08:00", input: "foo\n", wantErr: aborted, wantOut: prompt},
		{name: "end of input", target: "08:00", input: "", wantErr: aborted, wantOut: prompt},
		{name: "--yes", target: "08:00", yes: true, wantStart: tomorrow, wantOut: started},
		{
			name:    "not a terminal",
			target:  "08:00",
			input:   "y\n",
			notTTY:  true,
			wantErr: "08:00 already passed today; re-run with --yes to keep awake until tomorrow",
		},
		{name: "not a terminal with --yes", target: "08:00", yes: true, notTTY: true, wantStart: tomorrow, wantOut: started},
		{name: "later today", target: "17:00", wantStart: today, wantOut: "Keepawake active (PID 123) for 7h (until 17:00)\n"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			starter := &fakeStarter{now: now}
			var out bytes.Buffer
			deps := untilDeps{
				now:        func() time.Time { return now },
				starter:    starter,
				in:         strings.NewReader(tc.input),
				out:        &out,
				isTerminal: func() bool { return !tc.notTTY },
			}

			err := runUntil(deps, tc.target, tc.yes)
			if tc.wantErr != "" {
				if err == nil || err.Error() != tc.wantErr {
					t.Errorf("runUntil error = %v, want %q", err, tc.wantErr)
				}
				if len(starter.calls) != 0 {
					t.Errorf("StartUntil called %v, want no call", starter.calls)
				}
			} else {
				if err != nil {
					t.Fatalf("runUntil error: %v", err)
				}
				if len(starter.calls) != 1 || !starter.calls[0].Equal(tc.wantStart) {
					t.Errorf("StartUntil calls = %v, want [%v]", starter.calls, tc.wantStart)
				}
			}
			if got := out.String(); got != tc.wantOut {
				t.Errorf("output = %q, want %q", got, tc.wantOut)
			}
		})
	}
}

func TestHumanDuration(t *testing.T) {
	tests := []struct {
		name string
		in   time.Duration
		want string
	}{
		{name: "zero", in: 0, want: "0s"},
		{name: "seconds", in: 45 * time.Second, want: "45s"},
		{name: "rounds down", in: 59400 * time.Millisecond, want: "59s"},
		{name: "rounds up to a minute", in: 59600 * time.Millisecond, want: "1m0s"},
		{name: "one minute", in: time.Minute, want: "1m"},
		{name: "minutes", in: 59 * time.Minute, want: "59m"},
		{name: "one hour", in: time.Hour, want: "1h"},
		{name: "hours and minutes", in: time.Hour + 30*time.Minute, want: "1h 30m"},
		{name: "truncates seconds", in: time.Hour + 59*time.Minute + 59*time.Second, want: "1h 59m"},
		{name: "over a day", in: 25*time.Hour + 5*time.Minute, want: "25h 5m"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := humanDuration(tc.in); got != tc.want {
				t.Errorf("humanDuration(%v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}
