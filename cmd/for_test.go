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
