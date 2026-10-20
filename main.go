package main

import (
	"errors"
	"fmt"
	"os"

	"github.com/jose/keepawake/cmd"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := cmd.Execute(version); err != nil {
		if !errors.Is(err, cmd.ErrInactive) {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
		os.Exit(1)
	}
}
