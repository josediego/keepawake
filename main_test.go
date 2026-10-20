package main

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestDefaultVersion(t *testing.T) {
	if version != "dev" {
		t.Fatalf("version = %q, want %q", version, "dev")
	}
}

func TestLdflagsSetVersion(t *testing.T) {
	bin := filepath.Join(t.TempDir(), "keepawake")
	build := exec.Command("go", "build", "-o", bin, "-ldflags", "-X main.version=1.2.3-test", ".")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("go build: %v\n%s", err, out)
	}

	want := "keepawake version 1.2.3-test\n"
	for _, arg := range []string{"--version", "-v"} {
		out, err := exec.Command(bin, arg).Output()
		if err != nil {
			t.Fatalf("keepawake %s: %v", arg, err)
		}
		if string(out) != want {
			t.Errorf("keepawake %s = %q, want %q", arg, out, want)
		}
	}
}
