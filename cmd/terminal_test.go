package cmd

import (
	"os"
	"testing"
)

func TestIsTerminalFalse(t *testing.T) {
	tests := []struct {
		name string
		open func(t *testing.T) *os.File
	}{
		{name: "dev null", open: func(t *testing.T) *os.File {
			f, err := os.Open(os.DevNull)
			if err != nil {
				t.Fatal(err)
			}
			return f
		}},
		{name: "pipe reader", open: func(t *testing.T) *os.File {
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = w.Close() })
			return r
		}},
		{name: "pipe writer", open: func(t *testing.T) *os.File {
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = r.Close() })
			return w
		}},
		{name: "regular file", open: func(t *testing.T) *os.File {
			f, err := os.CreateTemp(t.TempDir(), "file")
			if err != nil {
				t.Fatal(err)
			}
			return f
		}},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			f := tc.open(t)
			t.Cleanup(func() { _ = f.Close() })
			if isTerminal(f) {
				t.Errorf("isTerminal(%s) = true, want false", tc.name)
			}
		})
	}
}
