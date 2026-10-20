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
	"syscall"
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

// ErrInvalidState is returned when the state file exists but is corrupt or incomplete.
var ErrInvalidState = errors.New("invalid keepawake state file")

// MinDuration and MaxDuration bound the durations accepted by For.
const (
	MinDuration = time.Minute
	MaxDuration = 24 * time.Hour
)

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
	now := m.now()
	targetTime, err := parseTargetTime(now, target)
	if err != nil {
		return 0, 0, err
	}

	pid, err := m.start(now, targetTime)
	if err != nil {
		return 0, 0, err
	}

	return pid, targetTime.Sub(now), nil
}

// For starts caffeinate for d, replacing any running session.
// It returns the pid that was started and the time the session ends.
func (m *Manager) For(d time.Duration) (int, time.Time, error) {
	if d < MinDuration || d > MaxDuration {
		return 0, time.Time{}, fmt.Errorf("duration %s out of range (%s to %s)", d, MinDuration, MaxDuration)
	}

	now := m.now()
	until := now.Add(d)
	pid, err := m.start(now, until)
	if err != nil {
		return 0, time.Time{}, err
	}

	return pid, until, nil
}

// StartUntil is the typed counterpart of Until: it starts caffeinate until t,
// replacing any running session.
// It returns the pid that was started and the duration until t.
func (m *Manager) StartUntil(t time.Time) (int, time.Duration, error) {
	now := m.now()
	pid, err := m.start(now, t)
	if err != nil {
		return 0, 0, err
	}

	return pid, t.Sub(now), nil
}

// start replaces any running session with a caffeinate process lasting from
// now until target, and saves its state. It is the only place that runs caffeinate.
func (m *Manager) start(now, target time.Time) (int, error) {
	duration := target.Sub(now)
	if duration < time.Second {
		return 0, errors.New("target time is in the past")
	}

	if err := m.cancelExistingSession(); err != nil {
		return 0, err
	}

	cmd := exec.Command(m.caffeinateBinary, "-di", "-t", strconv.FormatInt(int64(duration.Seconds()), 10))
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("failed to start %s: %w", m.caffeinateBinary, err)
	}

	pid := cmd.Process.Pid
	if err := m.saveState(pid, target.Unix()); err != nil {
		return 0, fmt.Errorf("failed to persist pid: %w", err)
	}

	return pid, nil
}

// Cancel terminates the running caffeinate session (if any).
func (m *Manager) Cancel() (int, error) {
	state, err := m.readState()
	if err != nil {
		if errors.Is(err, ErrInvalidState) {
			if rmErr := m.removeStateFile(); rmErr != nil {
				return 0, rmErr
			}
			return 0, ErrStateNotFound
		}
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
		return nil, fmt.Errorf("%w: %s: %v", ErrInvalidState, m.stateFilePath, err)
	}

	if s.PID <= 0 || s.Until <= 0 {
		return nil, fmt.Errorf("%w: missing or non-positive pid or until in %s", ErrInvalidState, m.stateFilePath)
	}

	return &s, nil
}

// CurrentState returns the exported state information if present.
// It automatically cleans up stale state if the time has passed or the process is dead.
func (m *Manager) CurrentState() (*State, error) {
	s, err := m.readState()
	if err != nil {
		if errors.Is(err, ErrInvalidState) {
			if rmErr := m.removeStateFile(); rmErr != nil {
				return nil, rmErr
			}
			return nil, ErrStateNotFound
		}
		return nil, err
	}

	untilTime := time.Unix(s.Until, 0)
	now := m.now()

	// Check if time has passed
	if now.After(untilTime) {
		_ = m.removeStateFile()
		return nil, ErrStateNotFound
	}

	// Check if process is still alive
	if !isProcessAlive(s.PID) {
		_ = m.removeStateFile()
		return nil, ErrStateNotFound
	}

	return &State{
		PID:   s.PID,
		Until: untilTime,
	}, nil
}

func isProcessAlive(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	// Signal 0 doesn't actually send a signal, but checks if the process exists
	err = proc.Signal(syscall.Signal(0))
	return err == nil
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
		// Never kill a PID read from a corrupt file; just discard it.
		if errors.Is(err, ErrInvalidState) {
			return m.removeStateFile()
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
		"3:04pm",
		"3pm",
	}

	value := normalizeTime(target)
	for _, layout := range layoutCandidates {
		t, err := time.ParseInLocation(layout, value, now.Location())
		if err == nil {
			return time.Date(now.Year(), now.Month(), now.Day(), t.Hour(), t.Minute(), t.Second(), 0, now.Location()), nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse time %q (try 17:30, 5pm or 5:30 pm)", target)
}

// normalizeTime lowercases s, collapses whitespace, maps a.m./p.m. to am/pm
// and drops the space before the suffix, so "5:30  P.M." becomes "5:30pm".
func normalizeTime(s string) string {
	s = strings.Join(strings.Fields(strings.ToLower(s)), " ")
	s = strings.ReplaceAll(s, "a.m.", "am")
	s = strings.ReplaceAll(s, "p.m.", "pm")
	s = strings.ReplaceAll(s, " am", "am")
	s = strings.ReplaceAll(s, " pm", "pm")
	return s
}
