# Phase 03 — Scan rules + analyzer

## Goal
Implement scanners that find candidates + analyzer that calculates sizes and groups by category/safety.

## Requirements
- Scan targets must be vetted against reference repo paths; do not invent unsafe targets
- Concurrency bounded worker pool
- Results include explicit safety level + safety note

## Files (Go)
Create:
- `go/pkg/types/types.go` (ScanItem, ScanResult, SafetyLevel)
- `go/internal/rules/builtin.go`
- `go/internal/scanner/*` (system/dev/apps)
- `go/internal/analyzer/analyzer.go`

## Steps
1) Define categories + safety levels aligned with PRD and reference
2) Implement initial Tier-1 scanners (MVP):
   - Xcode DerivedData
   - System caches (safe subset)
   - Homebrew cache
   - Node/npm caches (safe subset)
   - Docker cache candidates (no deletion unless safe)
3) Analyzer computes sizes and creates category summary

## Success criteria
- Scan returns stable JSON schema
- Items include path, size, category, safety
