# OpenCleaner — Tech stack (current)

## Current implementation (verified)
- Go daemon + CLI (see `go/go.mod` for the required Go toolchain version)
- SwiftUI macOS app (target macOS 14+)
- IPC: HTTP/JSON over a Unix domain socket
  - Default socket path is per-user: `/tmp/opencleaner.<uid>.sock`

## Repo layout (as in this repository)
- `go/` (Go module)
  - `cmd/opencleanerd` (daemon)
  - `cmd/opencleaner` (CLI)
  - `internal/` (engine, safety, cleaner, transport, stream, audit)
  - `pkg/types` (shared API types)
- `app/`
  - `project.yml` — xcodegen spec (run `cd app && xcodegen generate` to create `.xcodeproj`)
  - `OpenCleaner/` — SwiftUI app sources (xcodegen target)
  - `OpenCleanerTests/` — unit tests (xcodegen target)
  - `Packages/OpenCleanerClient/` — SPM client library: daemon + CLI clients
- `scripts/`
  - `create-dmg.sh` — builds a compressed DMG for distribution
- `.github/workflows/`
  - `ci.yml` — CI pipeline (parallel Go + Swift jobs)
  - `release.yml` — release pipeline (sign, notarize, DMG, Sparkle appcast, Homebrew tap)
- `docs/` (documentation)
- `plans/` (planning notes)

## Tooling / dependencies (current)
- Go: standard library (daemon uses stdlib `log` for debug prints; audit log is written by `internal/audit`)
- Swift: SwiftUI + Network.framework (`NWConnection`) for HTTP-over-unix-socket
- [Sparkle](https://github.com/sparkle-project/Sparkle) `2.9+`: in-app auto-update framework (EdDSA-signed appcast)
- [xcodegen](https://github.com/yonaskolb/XcodeGen): generates `.xcodeproj` from `app/project.yml`
- Observability:
  - Go SSE logs gated by `OPENCLEANER_DEBUG` or `OPENCLEANER_DEBUG_SSE`
  - Swift SSE lifecycle logs via OSLog (`subsystem: OpenCleanerClient`, `category: sse`)

## Build & signing
- Hardened runtime enabled (`ENABLE_HARDENED_RUNTIME: true` in `project.yml`)
- Entitlements: `app/OpenCleaner/OpenCleaner.entitlements`
- Release builds produce universal (arm64 + amd64) Go binaries embedded in the `.app` bundle
- Notarization via `xcrun notarytool`; ticket stapled to the app
