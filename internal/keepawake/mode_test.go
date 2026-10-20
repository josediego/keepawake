package keepawake

import (
	"slices"
	"testing"
	"time"
)

func TestCaffeinateArgs(t *testing.T) {
	tests := []struct {
		name string
		mode Mode
		d    time.Duration
		want []string
	}{
		{"zero", Mode{}, time.Hour, []string{"-di", "-t", "3600"}},
		{"default", DefaultMode, time.Hour, []string{"-di", "-t", "3600"}},
		{"display", Mode{Display: true}, time.Hour, []string{"-d", "-t", "3600"}},
		{"idle", Mode{Idle: true}, time.Hour, []string{"-i", "-t", "3600"}},
		{"system", Mode{System: true}, time.Hour, []string{"-s", "-t", "3600"}},
		{"disk", Mode{Disk: true}, time.Hour, []string{"-m", "-t", "3600"}},
		{"all", Mode{Display: true, Idle: true, System: true, Disk: true}, time.Hour, []string{"-dism", "-t", "3600"}},
		{"500ms", DefaultMode, 500 * time.Millisecond, []string{"-di", "-t", "1"}},
		{"1.5s", DefaultMode, 1500 * time.Millisecond, []string{"-di", "-t", "2"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := caffeinateArgs(tt.mode, tt.d); !slices.Equal(got, tt.want) {
				t.Errorf("caffeinateArgs() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestModeString(t *testing.T) {
	if got := DefaultMode.String(); got != "di" {
		t.Errorf("DefaultMode.String() = %q, want %q", got, "di")
	}
	if got := (Mode{}).String(); got != "" {
		t.Errorf("Mode{}.String() = %q, want %q", got, "")
	}
}

func TestParseMode(t *testing.T) {
	for i := 1; i < 16; i++ {
		mode := Mode{Display: i&1 != 0, Idle: i&2 != 0, System: i&4 != 0, Disk: i&8 != 0}
		got, err := parseMode(mode.String())
		if err != nil || got != mode {
			t.Errorf("parseMode(%q) = %+v, %v, want %+v", mode.String(), got, err, mode)
		}
	}
	if got, err := parseMode(""); err != nil || got != DefaultMode {
		t.Errorf("parseMode(\"\") = %+v, %v, want DefaultMode", got, err)
	}
	for _, s := range []string{"x", "dd", "id", "-d", "D"} {
		if _, err := parseMode(s); err == nil {
			t.Errorf("parseMode(%q) error = nil, want error", s)
		}
	}
}

func TestReadStateLegacyFile(t *testing.T) {
	m, _ := newManagerWithState(t, `{"pid":1,"until":2}`)
	s, err := m.readState()
	if err != nil {
		t.Fatalf("readState() error = %v", err)
	}
	if s.PID != 1 || s.Until != 2 || s.Mode != "" {
		t.Errorf("readState() = %+v, want pid 1, until 2, no mode", s)
	}
}

func TestCurrentStateLegacyFileIsDefaultMode(t *testing.T) {
	m, _ := newManagerWithSession(t, &fakeController{caffeinate: true}, time.Hour)
	s, err := m.CurrentState()
	if err != nil {
		t.Fatalf("CurrentState() error = %v", err)
	}
	if s.Mode != DefaultMode {
		t.Errorf("CurrentState() mode = %+v, want DefaultMode", s.Mode)
	}
}
