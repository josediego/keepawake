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

Install the binary with `go install ./...` or build locally with `go build ./...`. Runtime metadata lives in `~/.keepawake.state`, which stores the active PID and the scheduled wake timestamp.
