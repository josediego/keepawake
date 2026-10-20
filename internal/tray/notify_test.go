package tray

import "testing"

func TestAppleScriptString(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{"plain", "hello", `"hello"`},
		{"quotes", `say "hi"`, `"say \"hi\""`},
		{"backslashes", `a\b`, `"a\\b"`},
		{"both", `\"`, `"\\\""`},
		{"em dash message", msgFinished, `"Keepawake finished — your Mac can sleep again"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := appleScriptString(tt.in); got != tt.want {
				t.Errorf("appleScriptString(%q) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}
}

func TestNotificationScript(t *testing.T) {
	got := notificationScript("Keepawake", `stopped "early"`)
	want := `display notification "stopped \"early\"" with title "Keepawake"`
	if got != want {
		t.Errorf("notificationScript() = %s, want %s", got, want)
	}
}
