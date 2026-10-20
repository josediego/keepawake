package tray

import (
	"errors"
	"strings"
	"testing"
)

func TestQuitDialogScript(t *testing.T) {
	got := quitDialogScript(`say "hi" \ bye`)
	for _, want := range []string{
		`display dialog "say \"hi\" \\ bye"`,
		`with title "Keepawake"`,
		`buttons {"Cancel", "Stop & Quit"}`,
		`default button "Cancel"`,
		`cancel button "Cancel"`,
		`with icon caution`,
	} {
		if !strings.Contains(got, want) {
			t.Errorf("quitDialogScript() = %q, want it to contain %q", got, want)
		}
	}
}

type exitError int

func (e exitError) Error() string { return "exit status" }
func (e exitError) ExitCode() int { return int(e) }

func TestDialogAnswer(t *testing.T) {
	tests := []struct {
		name    string
		out     string
		err     error
		want    bool
		wantErr bool
	}{
		{"stop and quit", "button returned:Stop & Quit\n", nil, true, false},
		{"cancel button", "button returned:Cancel\n", nil, false, false},
		{"exit status 1 is cancel", "", exitError(1), false, false},
		{"another exit status", "", exitError(2), false, true},
		{"other error", "", errors.New("boom"), false, true},
		{"only the button name", "Stop & Quit\n", nil, false, false},
		{"more than the button", "button returned:Stop & Quit later\n", nil, false, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := dialogAnswer([]byte(tt.out), tt.err)
			if got != tt.want || (err != nil) != tt.wantErr {
				t.Errorf("dialogAnswer() = (%v, %v), want (%v, error %v)", got, err, tt.want, tt.wantErr)
			}
		})
	}
}
