// Package helpers provides test utilities for the taskmaster evaluation suite.
// The implementation is split across focused files:
//   - printer.go  — ANSI terminal output
//   - report.go   — pass/fail counting and result printing
//   - daemon.go   — daemon lifecycle, ctl runner, status parsing
//   - files.go    — file I/O, YAML mutation, RunCmd, Must
package helpers
