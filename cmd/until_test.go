package cmd

import (
	"testing"
	"time"
)

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
