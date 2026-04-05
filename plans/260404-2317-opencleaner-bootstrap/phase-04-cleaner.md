# Phase 04 — Cleaner (trash) + audit + undo

## Goal
Implement cleaning strategy: move-to-trash by default, optional hard delete only in CLI.

## Requirements
- Default: move to `~/.Trash` (macOS behavior)
- Audit log append-only: timestamp, operation, path, bytes, outcome
- Undo: store last operation manifest and allow “restore from Trash” where possible

## Files (Go)
Create:
- `go/internal/cleaner/trash.go`
- `go/internal/cleaner/cleaner.go`
- `go/internal/audit/audit.go`

## Steps
1) Implement `MoveToTrash` for files/dirs (use `os.Rename` into `~/.Trash` with unique name; handle collisions)
2) Ensure ValidatePathSafety is called before any move/remove
3) Implement audit log writing; rotate per PRD (later)
4) Implement “undo last clean” manifest (limited to trash moves)

## Success criteria
- Clean operation moves items to Trash and produces audit log
- Dry-run produces no filesystem changes
