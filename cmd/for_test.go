package cmd

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

func TestParseDurationArg(t *testing.T) {
	tests := []struct {
		in      string
		want    time.Duration
		wantErr bool
	}{
		{in: "45m", want: 45 * time.Minute},
		{in: "1h30m", want: 90 * time.Minute},
		{in: "90", want: 90 * time.Minute},
		{in: "1m", want: time.Minute},
		{in: "24h", want: 24 * time.Hour},
		{in: " 2H ", want: 2 * time.Hour},
		{in: "0", wantErr: true},
		{in: "-5m", wantErr: true},
		{in: "59s", wantErr: true},
		{in: "25h", wantErr: true},
		{in: "abc", wantErr: true},
		{in: "1440", want: 24 * time.Hour},
		{in: "1", want: time.Minute},
		{in: "9007199254741052", wantErr: true},
		{in: "1441", wantErr: true},
		{in: "-1", wantErr: true},
		{in: "99999999999999999999999999", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.in, func(t *testing.T) {
			got, err := parseDurationArg(tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseDurationArg(%q) = %v, want error", tc.in, got)
				}
				if !strings.Contains(err.Error(), tc.in) {
					t.Errorf("error %q does not mention input %q", err, tc.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDurationArg(%q) error: %v", tc.in, err)
			}
			if got != tc.want {
				t.Errorf("parseDurationArg(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestFormatUntil(t *testing.T) {
	now := time.Date(2026, time.October, 17, 10, 0, 0, 0, time.Local)

	if got := formatUntil(now, time.Date(2026, time.October, 17, 15, 30, 0, 0, time.Local)); got != "15:30" {
		t.Errorf("same day = %q, want %q", got, "15:30")
	}
	if got := formatUntil(now, time.Date(2026, time.October, 18, 4, 0, 0, 0, time.Local)); got != "Sun 18 Oct 04:00" {
		t.Errorf("next day = %q, want %q", got, "Sun 18 Oct 04:00")
	}

	// 2026-10-17 22:00 in A is 2026-10-18 01:00 UTC: the same day as t in t's zone.
	zoned := time.Date(2026, time.October, 17, 22, 0, 0, 0, time.FixedZone("A", -3*3600))
	if got := formatUntil(zoned, time.Date(2026, time.October, 18, 3, 0, 0, 0, time.UTC)); got != "03:00" {
		t.Errorf("same day in t's zone = %q, want %q", got, "03:00")
	}
}

func TestDurationFromArgs(t *testing.T) {
	tests := []struct {
		args    []string
		want    time.Duration
		wantErr bool
	}{
		{args: []string{"1h", "30m"}, want: 90 * time.Minute},
		{args: []string{"45m"}, want: 45 * time.Minute},
		{args: []string{"1h", "x"}, wantErr: true},
	}

	for _, tc := range tests {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			got, err := durationFromArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("durationFromArgs(%q) = %v, want error", tc.args, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("durationFromArgs(%q) error: %v", tc.args, err)
			}
			if got != tc.want {
				t.Errorf("durationFromArgs(%q) = %v, want %v", tc.args, got, tc.want)
			}
		})
	}
}

func TestForHelp(t *testing.T) {
	var buf bytes.Buffer
	root := newRootCommand("test")
	root.SetOut(&buf)
	root.SetArgs([]string{"for", "--help"})

	if err := root.Execute(); err != nil {
		t.Fatalf("Execute: %v", err)
	}

	help := buf.String()
	for _, want := range []string{"45m", "1h30m", "90"} {
		if !strings.Contains(help, want) {
			t.Errorf("help does not mention %q:\n%s", want, help)
		}
	}
}
