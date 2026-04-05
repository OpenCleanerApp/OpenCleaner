# Phase 02 — Safety guard + safety tests

## Goal
Implement safety guard + deletion primitives that match `clmm-clean-my-mac-cli` patterns.

## Requirements
- Centralize all deletions in one package (no scanner deletes)
- Absolute-path enforcement at delete boundary
- Traversal detection
- TOCTOU mitigation via lstat immediately before removal
- Symlink safety (unlink only)
- Dry-run support

## Files to create/modify (Go)
Create:
- `go/internal/safety/guard.go`
- `go/internal/safety/guard_test.go`
- `go/internal/safety/paths.go` (protected/allowed lists)

## Steps
1) Port protected/allowed sets from reference repo (and PRD never-touch list). Prefer “deny by default”.
2) Implement:
   - `HasTraversalPattern(string) bool`
   - `ExpandPath(string) (string, error)` (only if needed at boundary)
   - `ValidatePathSafety(absPath string) error`
   - `SafeRemove(absPath string, dryRun bool) (removedBytes int64, err error)`
3) Write safety tests:
   - refusal cases: `/`, `$HOME`, protected prefixes
   - allow override: `/tmp`, `/private/var/folders`
   - symlink unlink-only
   - TOCTOU regression
4) Add lint rule: no `os/exec` usage for deletion

## Success criteria
- Safety tests comprehensive + passing
- Any attempt to delete protected paths fails deterministically
