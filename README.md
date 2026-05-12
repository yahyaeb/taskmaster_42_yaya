# Taskmaster 42

## Running the Daemon

```bash
go run main.go
# or
go build ./cmd/daemon && ./daemon
```

## Running Tests

```bash
go test ./e2e/... # Run all end-to-end tests
go test ./internal/... # Run all internal tests
```