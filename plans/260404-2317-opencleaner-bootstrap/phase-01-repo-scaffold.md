# Phase 01 — Repo scaffold + tooling

## Goal
Create the baseline monorepo structure for Go daemon/CLI + SwiftUI app; enable local builds + CI.

## Requirements
- Matches PRD architecture decisions (daemon day 1)
- Minimal dependencies

## Files to create/modify
Create:
- `go/` module with `cmd/opencleanerd`, `cmd/opencleaner`
- `scripts/` (build/test helpers)
- `launchd/com.opencleaner.daemon.plist`
- `.github/workflows/ci.yml` (Go test + xcodebuild build)
- `app/` (SwiftUI Xcode project; created via xcodebuild/Xcode)

## Steps
1) Create directory layout per `docs/tech-stack.md`
2) Initialize Go module (module path TBD; use placeholder until org known)
3) Add minimal logging wrapper (zap) and version injection
4) Set up CI jobs:
   - Go: go test/vet + gofmt check
   - Swift: xcodebuild build

## Success criteria
- `go test ./...` runs (even if mostly empty)
- `xcodebuild ... build` succeeds (minimal app skeleton)
