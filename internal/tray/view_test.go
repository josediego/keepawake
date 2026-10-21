package tray

import (
	"bytes"
	"errors"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
	"time"

	"github.com/jose/keepawake/internal/keepawake"
)

func TestBuildView(t *testing.T) {
	loc := time.FixedZone("T", -3*3600)
	// Tuesday 16:05 in loc.
	now := time.Date(2026, 10, 20, 16, 5, 0, 0, loc)

	active := func(until time.Time) *keepawake.State {
		return &keepawake.State{PID: 1, Until: until}
	}
	activeView := func(end, remaining string) view {
		return view{
			Active:        true,
			Title:         remaining,
			StatusText:    "Active until " + end + " (" + remaining + " left)",
			Tooltip:       "Keepawake active until " + end + " (" + remaining + " left)",
			CancelEnabled: true,
			QuitEnabled:   true,
		}
	}
	inactive := view{StatusText: "Inactive", Tooltip: "Keepawake inactive", QuitEnabled: true}
	unavailable := view{StatusText: "Status unavailable", Tooltip: "Keepawake status unavailable", QuitEnabled: true}

	tests := []struct {
		name  string
		state *keepawake.State
		err   error
		want  view
	}{
		{
			name:  "active same day",
			state: active(time.Date(2026, 10, 20, 17, 30, 0, 0, loc)),
			want:  activeView("17:30", "1h 25m"),
		},
		{
			name:  "active ending tomorrow",
			state: active(time.Date(2026, 10, 21, 9, 0, 0, 0, loc)),
			want:  activeView("Wed 09:00", "16h 55m"),
		},
		{
			name:  "under a minute left",
			state: active(now.Add(30 * time.Second)),
			want:  activeView("16:05", "<1m"),
		},
		{
			name:  "until already passed",
			state: active(now.Add(-time.Minute)),
			want:  activeView("16:04", "<1m"),
		},
		{
			name:  "exactly one hour left",
			state: active(now.Add(time.Hour)),
			want:  activeView("17:05", "1h"),
		},
		{
			name:  "one hour minus one second truncated",
			state: active(now.Add(time.Hour - time.Second)),
			want:  activeView("17:04", "59m"),
		},
		{
			name:  "until stored in another zone",
			state: active(time.Date(2026, 10, 20, 17, 30, 0, 0, loc).In(time.FixedZone("E", 5*3600))),
			want:  activeView("17:30", "1h 25m"),
		},
		{
			name:  "tomorrow in UTC but today in now's zone",
			state: active(time.Date(2026, 10, 20, 22, 30, 0, 0, loc).UTC()),
			want:  activeView("22:30", "6h 25m"),
		},
		{
			name: "inactive",
			err:  keepawake.ErrStateNotFound,
			want: inactive,
		},
		{
			name: "inactive with wrapped not found",
			err:  errors.Join(errors.New("cleanup"), keepawake.ErrStateNotFound),
			want: inactive,
		},
		{
			name: "generic error",
			err:  errors.New("permission denied"),
			want: unavailable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildView(tt.state, tt.err, now)
			if got != tt.want {
				t.Fatalf("buildView() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestIconChange(t *testing.T) {
	a := &App{}
	steps := []struct {
		active  bool
		icon    []byte
		changed bool
	}{
		{active: false},
		{active: true, icon: iconActive, changed: true},
		{active: true},
		{active: false, icon: iconInactive, changed: true},
		{active: false},
	}

	for i, s := range steps {
		icon, changed := a.iconChange(s.active)
		if changed != s.changed || !bytes.Equal(icon, s.icon) || (s.icon == nil && icon != nil) {
			t.Fatalf("step %d: iconChange(%v) = (%d bytes, %v), want (%d bytes, %v)",
				i, s.active, len(icon), changed, len(s.icon), s.changed)
		}
	}
}

func TestViewDoesNotImportSystray(t *testing.T) {
	f, err := parser.ParseFile(token.NewFileSet(), "view.go", nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse view.go: %v", err)
	}
	for _, imp := range f.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			t.Fatalf("unquote import %s: %v", imp.Path.Value, err)
		}
		if path == "github.com/getlantern/systray" {
			t.Fatalf("view.go must not import %s", path)
		}
	}
}
