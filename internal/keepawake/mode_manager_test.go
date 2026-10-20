package keepawake

import (
	"os"
	"strings"
	"testing"
	"time"
)

func assertStateMode(t *testing.T, m *Manager, want string) {
	t.Helper()
	data, err := os.ReadFile(m.stateFilePath)
	if err != nil {
		t.Fatalf("failed to read state file: %v", err)
	}
	if !strings.Contains(string(data), `"mode":"`+want+`"`) {
		t.Errorf("state file = %s, want mode %q", data, want)
	}
}

func TestStartUsesManagerMode(t *testing.T) {
	tests := []struct {
		name  string
		start func(m *Manager, now time.Time) (int, error)
	}{
		{"For", func(m *Manager, now time.Time) (int, error) {
			pid, _, err := m.For(45 * time.Minute)
			return pid, err
		}},
		{"StartUntil", func(m *Manager, now time.Time) (int, error) {
			pid, _, err := m.StartUntil(now.Add(45 * time.Minute))
			return pid, err
		}},
		{"Until", func(m *Manager, now time.Time) (int, error) {
			pid, _, err := m.Until("10:45")
			return pid, err
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, argsFile, now := newExtendManager(t, &fakeController{})
			WithMode(Mode{Idle: true})(m)

			pid, err := tt.start(m, now)
			if err != nil {
				t.Fatalf("%s() error = %v", tt.name, err)
			}
			killOnCleanup(t, pid)

			assertArgs(t, argsFile, "-i -t 2700")
			assertStateMode(t, m, "i")
		})
	}
}

func TestStartRecordsDefaultMode(t *testing.T) {
	m, argsFile, _ := newExtendManager(t, &fakeController{})

	pid, _, err := m.For(45 * time.Minute)
	if err != nil {
		t.Fatalf("For() error = %v", err)
	}
	killOnCleanup(t, pid)

	assertArgs(t, argsFile, "-di -t 2700")
	assertStateMode(t, m, "di")
}

func TestExtendKeepsSessionMode(t *testing.T) {
	pc := &fakeController{caffeinate: true}
	m, argsFile, now := newExtendManager(t, pc)
	WithMode(Mode{Display: true})(m)
	if err := m.writeState(state{PID: fakePID, Until: now.Add(time.Hour).Unix(), Mode: "is"}); err != nil {
		t.Fatalf("writeState() error = %v", err)
	}

	pid, _, err := m.Extend(30 * time.Minute)
	if err != nil {
		t.Fatalf("Extend() error = %v", err)
	}
	killOnCleanup(t, pid)

	assertTerminated(t, pc, []int{fakePID})
	assertArgs(t, argsFile, "-is -t 5400")
	assertStateMode(t, m, "is")
}
