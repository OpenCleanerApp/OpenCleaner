# Phase 06 — CLI commands

## Goal
Provide `opencleaner` CLI that talks to daemon, supports dry-run + JSON output.

## Requirements
- Commands (MVP): `scan`, `clean`, `status`
- Flags:
  - `--json`
  - `--dry-run`
  - `--unsafe` (enables risky categories)
  - `--force` (hard delete, CLI only; default off)

## Files (Go)
Create:
- `go/cmd/opencleaner/main.go`
- `go/internal/cli/*`

## Success criteria
- CLI can trigger scan and display categorized summary
- Clean respects safety gating + flags
