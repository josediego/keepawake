# Keepawake

A structured CLI wrapper around macOS `caffeinate` that keeps your machine awake until a specified time.

## Usage

```bash
go run . until 17:00
go run . until "5:30pm"
go run . cancel
go run . tray
```

## Project layout

- `cmd/` – Cobra-powered CLI commands (`until`, `cancel`, etc.)
- `internal/keepawake` – core orchestration of `caffeinate`, state management, and time parsing
- `main.go` – thin entrypoint that wires the CLI together
- `internal/tray` – menubar icon logic plus the background launcher used by `keepawake until`
- `assets/` – tray icon PNGs (`icons/`), embedded into the binary by the `assets` Go package

## Build

```bash
make build      # builds bin/keepawake
make test       # go test ./...
make vet        # go vet ./...
make fmt-check  # fails listing any files gofmt would change
make check      # build, vet, fmt-check and test
make install    # go install with the version embedded
make clean      # removes bin/
```

The version comes from `git describe --tags --always --dirty`:

```bash
keepawake --version   # or -v; prints "keepawake version <version>"
```

A plain `go build` or `go install` reports `dev`.

Runtime metadata lives in `~/.keepawake.state`, which stores the active PID and the scheduled wake timestamp.
