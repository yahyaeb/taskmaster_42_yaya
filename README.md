# taskmaster_42
# Developer Guide

## Quick Start

### Running the Daemon
```bash
go run main.go
# or
go build ./cmd/daemon && ./daemon
```

### Running Tests
```bash
# All internal tests
go test ./internal -v

# Specific package
go test ./internal/engine -v

# Coverage
go test ./internal -cover
```

### Building All
```bash
go build ./...  # Builds main.go, cmd/daemon/main.go, cmd/ctl/main.go
```
---

## Common Commands

```bash
# Run daemon in foreground (kill with Ctrl+C)
go run main.go

# Build daemon binary
go build -o daemon ./cmd/daemon

# Build CLI binary
go build -o ctl ./cmd/ctl

# Watch for changes and rebuild
go run main.go & watch -n 0.5 'ps aux | grep main'

# Run specific test with verbose output
go test -v ./internal/engine -run TestProcessWatcher

# Run tests with race detection
go test -race ./internal/...

# Run tests with coverage report
go test -coverprofile=coverage.out ./internal/...
go tool cover -html=coverage.out
```

