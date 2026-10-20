package keepawake

import (
	"testing"
	"time"
)

func TestResolveTarget(t *testing.T) {
	chicago, err := time.LoadLocation("America/Chicago")
	if err != nil {
		t.Fatalf("LoadLocation: %v", err)
	}
	at := func(h, m, s, ns int) time.Time {
		return time.Date(2026, time.October, 17, h, m, s, ns, time.Local)
	}

	tests := []struct {
		name     string
		now      time.Time
		target   string
		want     time.Time
		rolled   bool
		wantErr  bool
		wantDiff time.Duration
	}{
		{name: "later today", now: at(10, 0, 0, 0), target: "17:00", want: at(17, 0, 0, 0)},
		{name: "passed today", now: at(10, 0, 0, 0), target: "08:00", want: time.Date(2026, time.October, 18, 8, 0, 0, 0, time.Local), rolled: true},
		{name: "exactly now", now: at(10, 0, 0, 0), target: "10:00", want: time.Date(2026, time.October, 18, 10, 0, 0, 0, time.Local), rolled: true},
		{name: "half a second ahead", now: at(9, 59, 59, 5e8), target: "10:00", want: time.Date(2026, time.October, 18, 10, 0, 0, 0, time.Local), rolled: true},
		{name: "1.5s ahead", now: at(9, 59, 58, 5e8), target: "10:00", want: at(10, 0, 0, 0)},
		{
			name:     "across DST",
			now:      time.Date(2026, time.March, 7, 23, 30, 0, 0, chicago),
			target:   "08:00",
			want:     time.Date(2026, time.March, 8, 8, 0, 0, 0, chicago),
			rolled:   true,
			wantDiff: 7*time.Hour + 30*time.Minute,
		},
		{name: "invalid", now: at(10, 0, 0, 0), target: "garbage", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, rolled, err := ResolveTarget(tc.now, tc.target)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("ResolveTarget(%q) = %v, want error", tc.target, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("ResolveTarget(%q) error: %v", tc.target, err)
			}
			if !got.Equal(tc.want) || rolled != tc.rolled {
				t.Errorf("ResolveTarget(%q) = %v, %v, want %v, %v", tc.target, got, rolled, tc.want, tc.rolled)
			}
			if tc.wantDiff != 0 {
				if d := got.Sub(tc.now); d != tc.wantDiff {
					t.Errorf("duration = %v, want %v", d, tc.wantDiff)
				}
				if zone, _ := got.Zone(); zone != "CDT" {
					t.Errorf("zone = %q, want CDT", zone)
				}
			}
		})
	}
}
