package keepawake

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Manager orchestrates keepawake sessions backed by caffeinate.
type Manager struct {
	stateFilePath    string
	caffeinateBinary string
	now              func() time.Time
}

// Option configures a Manager.
type Option func(*Manager)

// ErrStateNotFound is returned when the state file doesn't exist yet.
var ErrStateNotFound = errors.New("no keepawake active (state file not found)")

// WithStateFile overrides the default state file path.
func WithStateFile(path string) Option {
	return func(m *Manager) {
		m.stateFilePath = path
	}
}

// WithCaffeinateBinary overrides the binary that will be executed.
func WithCaffeinateBinary(path string) Option {
	return func(m *Manager) {
		m.caffeinateBinary = path
	}
}

// WithClock injects a custom clock (helpful for tests).
func WithClock(clock func() time.Time) Option {
	return func(m *Manager) {
		m.now = clock
	}
}

// NewManager builds a Manager with sane defaults.
func NewManager(opts ...Option) (*Manager, error) {
	defaultStateFile, err := defaultStateFilePath()
	if err != nil {
		return nil, err
	}

	manager := &Manager{
		stateFilePath:    defaultStateFile,
		caffeinateBinary: "caffeinate",
		now:              time.Now,
	}

	for _, opt := range opts {
		opt(manager)
	}

	return manager, nil
}

// Until starts caffeinate until the requested target time.
// It returns the pid that was started and the duration until that time.
func (m *Manager) Until(target string) (int, time.Duration, error) {
	targetTime, err := parseTargetTime(m.now(), target)
	if err != nil {
		return 0, 0, err
	}

	if err := m.cancelExistingSession(); err != nil {
		return 0, 0, err
	}

	duration := targetTime.Sub(m.now())
	if duration <= 0 {
		return 0, 0, errors.New("target time is in the past")
	}

	cmd := exec.Command(m.caffeinateBinary, "-di", "-t", strconv.FormatInt(int64(duration.Seconds()), 10))
	if err := cmd.Start(); err != nil {
		return 0, 0, fmt.Errorf("failed to start %s: %w", m.caffeinateBinary, err)
	}

	pid := cmd.Process.Pid
	if err := m.saveState(pid, targetTime.Unix()); err != nil {
		return 0, 0, fmt.Errorf("failed to persist pid: %w", err)
	}

	return pid, duration, nil
}

// Cancel terminates the running caffeinate session (if any).
func (m *Manager) Cancel() (int, error) {
	state, err := m.readState()
	if err != nil {
		return 0, err
	}

	proc, err := os.FindProcess(state.PID)
	if err != nil {
		return 0, fmt.Errorf("failed to find process %d: %w", state.PID, err)
	}

	if err := proc.Kill(); err != nil {
		return 0, fmt.Errorf("failed to kill process %d: %w", state.PID, err)
	}

	if err := m.removeStateFile(); err != nil {
		return state.PID, err
	}

	return state.PID, nil
}

type state struct {
	PID   int   `json:"pid"`
	Until int64 `json:"until"`
}

// State represents the exported runtime information of keepawake.
type State struct {
	PID   int
	Until time.Time
}

func (m *Manager) saveState(pid int, until int64) error {
	payload, err := json.Marshal(state{
		PID:   pid,
		Until: until,
	})
	if err != nil {
		return err
	}

	return os.WriteFile(m.stateFilePath, payload, 0o644)
}

func (m *Manager) readState() (*state, error) {
	data, err := os.ReadFile(m.stateFilePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrStateNotFound
		}
		return nil, err
	}

	var s state
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("invalid state in %s: %w", m.stateFilePath, err)
	}

	if s.PID == 0 || s.Until == 0 {
		return nil, fmt.Errorf("invalid state content in %s", m.stateFilePath)
	}

	return &s, nil
}

// CurrentState returns the exported state information if present.
func (m *Manager) CurrentState() (*State, error) {
	s, err := m.readState()
	if err != nil {
		return nil, err
	}

	return &State{
		PID:   s.PID,
		Until: time.Unix(s.Until, 0),
	}, nil
}

func (m *Manager) removeStateFile() error {
	if err := os.Remove(m.stateFilePath); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func (m *Manager) cancelExistingSession() error {
	state, err := m.readState()
	if err != nil {
		if errors.Is(err, ErrStateNotFound) {
			return nil
		}
		return err
	}

	proc, err := os.FindProcess(state.PID)
	if err == nil {
		if killErr := proc.Kill(); killErr != nil && !errors.Is(killErr, os.ErrProcessDone) {
			return fmt.Errorf("failed to terminate prior keepawake process %d: %w", state.PID, killErr)
		}
	}

	return m.removeStateFile()
}

func defaultStateFilePath() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	return filepath.Join(usr.HomeDir, ".keepawake.state"), nil
}

func parseTargetTime(now time.Time, target string) (time.Time, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return time.Time{}, errors.New("target time is required")
	}

	layoutCandidates := []string{
		"15:04",
		"15:04:05",
		time.Kitchen, // 3:04PM
		"3:04pm",
		"3pm",
	}

	var (
		parsed  time.Time
		lastErr error
	)

	for _, layout := range layoutCandidates {
		value := target
		if strings.Contains(layout, "pm") || layout == "3pm" {
			value = strings.ToLower(target)
		}
		if layout == time.Kitchen {
			value = strings.ToUpper(target)
		}

		t, err := time.ParseInLocation(layout, value, now.Location())
		if err == nil {
			parsed = time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), t.Second(), 0, now.Location())
			lastErr = nil
			break
		}
		lastErr = err
	}

	if lastErr != nil {
		return time.Time{}, fmt.Errorf("unable to parse time %q: %w", target, lastErr)
	}

	return parsed, nil
}
