# Phase 08 — Integration/perf tests + docs polish

## Goal
Hardening: safety tests, integration tests, performance checks, and minimal documentation.

## Requirements (from PRD)
- Unit: safety guard, traversal, TOCTOU, symlink behavior
- Integration: daemon endpoints + dry-run
- Safety tests: critical path tests must exist
- Performance targets: 100K files < 10s, 1M < 30s (bench-style tests)

## Steps
1) Add integration tests that spin up daemon on temp unix socket
2) Add golden JSON fixtures for API compatibility
3) Add perf harness (opt-in) with clear limits + doesn’t delete
4) Docs:
   - README (build/run)
   - Security model overview
   - API quick reference

## Success criteria
- `go test ./...` includes safety + integration coverage
- Basic docs exist and match behavior
