package keepawake

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Manager orchestrates keepawake sessions backed by caffeinate.
type Manager struct {
	stateFilePath    string
	caffeinateBinary string
	now              func() time.Time
	processes        ProcessController
	mode             Mode
}

// Option configures a Manager.
type Option func(*Manager)

// ErrStateNotFound is returned when the state file doesn't exist yet.
var ErrStateNotFound = errors.New("no keepawake active (state file not found)")

// ErrInvalidState is returned when the state file exists but is corrupt or incomplete.
var ErrInvalidState = errors.New("invalid keepawake state file")

// ErrStateBusy is returned when another keepawake holds the state lock for
// longer than lockWait.
var ErrStateBusy = errors.New("keepawake state is busy (another keepawake holds the lock)")

// lockWait bounds how long an operation waits for the state lock; tests
// shorten it.
var lockWait = 2 * time.Second

// lockPoll is how often a busy state lock is retried.
const lockPoll = 10 * time.Millisecond

// MinDuration and MaxDuration bound the durations accepted by For and Extend.
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

// WithProcessController injects a custom ProcessController (helpful for tests).
func WithProcessController(pc ProcessController) Option {
	return func(m *Manager) {
		m.processes = pc
	}
}

// WithMode sets the mode of the sessions this Manager starts.
// The zero Mode means DefaultMode.
func WithMode(mode Mode) Option {
	return func(m *Manager) {
		m.mode = mode
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
		processes:        defaultProcessController{},
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

	pid, err := m.start(now, targetTime, m.mode)
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
	pid, err := m.start(now, until, m.mode)
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
	pid, err := m.start(now, t, m.mode)
	if err != nil {
		return 0, 0, err
	}

	return pid, t.Sub(now), nil
}

// Extend pushes the end of the running session d further, or starts one for d
// if none is running. The new end may be at most MaxDuration from now.
// It returns the pid that was started and the time the session ends.
func (m *Manager) Extend(d time.Duration) (int, time.Time, error) {
	if d < MinDuration || d > MaxDuration {
		return 0, time.Time{}, fmt.Errorf("duration %s out of range (%s to %s)", d, MinDuration, MaxDuration)
	}

	unlock, err := m.lock()
	if err != nil {
		return 0, time.Time{}, err
	}
	defer unlock()

	now := m.now()
	until := now.Add(d)
	mode := m.mode
	s, err := m.currentState()
	if err == nil && s.Until.After(now) {
		until = s.Until.Add(d)
		mode = s.Mode
	} else if err != nil && !errors.Is(err, ErrStateNotFound) {
		return 0, time.Time{}, err
	}
	if until.Sub(now) > MaxDuration {
		return 0, time.Time{}, fmt.Errorf("extended end %s is more than %s from now", until.Format(time.RFC3339), MaxDuration)
	}

	pid, err := m.replace(now, until, mode)
	if err != nil {
		return 0, time.Time{}, err
	}

	return pid, until, nil
}

// ResolveTarget parses target for today. A time that cannot run for at least
// 1s today means the same wall-clock time tomorrow, with rolledOver set.
func ResolveTarget(now time.Time, target string) (t time.Time, rolledOver bool, err error) {
	t, err = parseTargetTime(now, target)
	if err != nil {
		return time.Time{}, false, err
	}
	if t.Sub(now) >= time.Second {
		return t, false, nil
	}
	// time.Date, not Add(24h), keeps the wall-clock time across DST changes.
	return time.Date(t.Year(), t.Month(), t.Day()+1, t.Hour(), t.Minute(), t.Second(), 0, now.Location()), true, nil
}

// start runs replace while holding the state lock.
func (m *Manager) start(now, target time.Time, mode Mode) (int, error) {
	unlock, err := m.lock()
	if err != nil {
		return 0, err
	}
	defer unlock()
	return m.replace(now, target, mode)
}

// replace replaces any running session with a caffeinate process lasting from
// now until target holding mode's assertions, and saves its state. It is the
// only place that runs caffeinate. The caller must hold the state lock.
func (m *Manager) replace(now, target time.Time, mode Mode) (int, error) {
	duration := target.Sub(now)
	if duration < time.Second {
		return 0, errors.New("target time is in the past")
	}

	if err := m.cancelExistingSession(); err != nil {
		return 0, err
	}

	mode = mode.orDefault()
	cmd := exec.Command(m.caffeinateBinary, caffeinateArgs(mode, duration)...)
	if err := cmd.Start(); err != nil {
		return 0, fmt.Errorf("failed to start %s: %w", m.caffeinateBinary, err)
	}
	// Reap caffeinate when it exits so a long-lived parent does not keep
	// zombies; Process.Release does not reap.
	go func() { _ = cmd.Wait() }()

	pid := cmd.Process.Pid
	if err := m.writeState(state{PID: pid, Until: target.Unix(), Mode: mode.String()}); err != nil {
		return 0, fmt.Errorf("failed to persist pid: %w", err)
	}

	return pid, nil
}

// Cancel terminates the running caffeinate session (if any).
func (m *Manager) Cancel() (int, error) {
	unlock, err := m.lock()
	if err != nil {
		return 0, err
	}
	defer unlock()

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

	// Never signal a PID that may have been reused; just discard the stale state.
	if !m.isLive(state) {
		if err := m.removeStateFile(); err != nil {
			return 0, err
		}
		return 0, ErrStateNotFound
	}

	if err := m.processes.Terminate(state.PID); err != nil {
		return 0, fmt.Errorf("failed to terminate process %d: %w", state.PID, err)
	}

	if err := m.removeStateFile(); err != nil {
		return state.PID, err
	}

	return state.PID, nil
}

type state struct {
	PID   int    `json:"pid"`
	Until int64  `json:"until"`
	Mode  string `json:"mode,omitempty"`
}

// State represents the exported runtime information of keepawake.
type State struct {
	PID   int
	Until time.Time
	// Mode is never the zero Mode: files written before modes existed read
	// back as DefaultMode.
	Mode Mode
}

// saveState writes a state file without a mode, like those written before
// modes existed.
func (m *Manager) saveState(pid int, until int64) error {
	return m.writeState(state{PID: pid, Until: until})
}

func (m *Manager) writeState(s state) error {
	payload, err := json.Marshal(s)
	if err != nil {
		return err
	}

	// Write to a sibling temp file and rename it over the target so readers
	// never observe a partially written state file.
	dir, base := filepath.Dir(m.stateFilePath), filepath.Base(m.stateFilePath)
	tmp, err := os.CreateTemp(dir, base+".tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if err := writeAndSync(tmp, payload); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, m.stateFilePath); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

func writeAndSync(f *os.File, payload []byte) error {
	if err := f.Chmod(0o600); err != nil {
		return err
	}
	if _, err := f.Write(payload); err != nil {
		return err
	}
	return f.Sync()
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

	if _, err := parseMode(s.Mode); err != nil {
		return nil, fmt.Errorf("%w: invalid mode %q in %s", ErrInvalidState, s.Mode, m.stateFilePath)
	}

	return &s, nil
}

// CurrentState returns the exported state information if present.
// It automatically cleans up stale state if the time has passed or the process is dead.
func (m *Manager) CurrentState() (*State, error) {
	unlock, err := m.lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	return m.currentState()
}

// currentState is CurrentState for callers that already hold the state lock.
// flock is per open file, so taking the lock again in the same process would
// deadlock.
func (m *Manager) currentState() (*State, error) {
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

	// Check if process is still alive and still caffeinate (the PID may have been reused)
	if !m.processes.IsCaffeinate(s.PID) {
		_ = m.removeStateFile()
		return nil, ErrStateNotFound
	}

	// readState has already validated the mode.
	mode, _ := parseMode(s.Mode)
	return &State{
		PID:   s.PID,
		Until: untilTime,
		Mode:  mode,
	}, nil
}

// isLive reports whether s describes a running session: its until has not
// passed and its PID still belongs to caffeinate.
func (m *Manager) isLive(s *state) bool {
	return !m.now().After(time.Unix(s.Until, 0)) && m.processes.IsCaffeinate(s.PID)
}

// lock takes an exclusive flock on <state file>.lock, waiting at most
// lockWait, and returns the function that releases it. The lock file is never
// removed. os.OpenFile sets close-on-exec, so caffeinate never inherits the
// lock. Each call opens the file anew, so two Managers conflict like two
// processes do.
func (m *Manager) lock() (func(), error) {
	f, err := os.OpenFile(m.stateFilePath+".lock", os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(lockWait)
	for {
		err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) {
			_ = f.Close()
			return nil, err
		}
		if !time.Now().Before(deadline) {
			_ = f.Close()
			return nil, ErrStateBusy
		}
		time.Sleep(lockPoll)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
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
		// Never kill a PID read from a corrupt file; just discard it.
		if errors.Is(err, ErrInvalidState) {
			return m.removeStateFile()
		}
		return err
	}

	if m.isLive(state) {
		if err := m.processes.Terminate(state.PID); err != nil {
			return fmt.Errorf("failed to terminate prior keepawake process %d: %w", state.PID, err)
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
