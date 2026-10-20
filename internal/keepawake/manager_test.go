package keepawake

import (
	"testing"
	"time"
)

func TestParseTargetTime(t *testing.T) {
	localNow := time.Date(2026, 3, 10, 9, 0, 0, 0, time.Local)
	ist := time.FixedZone("IST", 5*3600+30*60)
	istNow := time.Date(2026, 3, 10, 9, 0, 0, 0, ist)

	tests := []struct {
		name, in            string
		now                 time.Time
		wantH, wantM, wantS int
		wantErr             bool
	}{
		{name: "24h", in: "17:00", wantH: 17},
		{name: "24h leading zero", in: "09:05", wantH: 9, wantM: 5},
		{name: "24h single digit hour", in: "9:05", wantH: 9, wantM: 5},
		{name: "24h with seconds", in: "17:30:15", wantH: 17, wantM: 30, wantS: 15},
		{name: "hour pm lowercase", in: "5pm", wantH: 17},
		{name: "hour pm uppercase", in: "5PM", wantH: 17},
		{name: "kitchen lowercase", in: "5:30pm", wantH: 17, wantM: 30},
		{name: "kitchen uppercase", in: "5:30PM", wantH: 17, wantM: 30},
		{name: "surrounding spaces", in: "  17:00  ", wantH: 17},
		{name: "surrounding tab and newline", in: "\t5pm\n", wantH: 17},
		{name: "fixed zone", in: "17:00", now: istNow, wantH: 17},
		{name: "empty", in: "", wantErr: true},
		{name: "whitespace only", in: "   ", wantErr: true},
		{name: "word", in: "noon", wantErr: true},
		{name: "hour out of range", in: "25:00", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			now := tc.now
			if now.IsZero() {
				now = localNow
			}

			got, err := parseTargetTime(now, tc.in)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("parseTargetTime(%q) = %v, want error", tc.in, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseTargetTime(%q) unexpected error: %v", tc.in, err)
			}

			want := time.Date(now.Year(), now.Month(), now.Day(), tc.wantH, tc.wantM, tc.wantS, 0, now.Location())
			if !got.Equal(want) {
				t.Errorf("parseTargetTime(%q) = %v, want %v", tc.in, got, want)
			}
			if got.Location() != now.Location() {
				t.Errorf("parseTargetTime(%q) location = %v, want %v", tc.in, got.Location(), now.Location())
			}
			if got.Year() != now.Year() || got.Month() != now.Month() || got.Day() != now.Day() {
				t.Errorf("parseTargetTime(%q) date = %s, want %s", tc.in, got.Format("2006-01-02"), now.Format("2006-01-02"))
			}
		})
	}
}
