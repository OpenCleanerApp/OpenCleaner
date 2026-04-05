# OpenCleaner — Tech stack (bootstrap)

## Locked by PRD
- Go engine (>=1.22)
- SwiftUI macOS app (target macOS 14+)
- IPC: HTTP/JSON over Unix domain socket
- Plugins later: WASM via wazero
- Updates: Sparkle (app), Homebrew (CLI)

## Recommended repo layout (monorepo)
- `go/` (Go module)
  - `cmd/opencleanerd` (daemon)
  - `cmd/opencleaner` (CLI)
  - `internal/` (safety, scanners, cleaner, transport, stream)
- `app/` (Xcode project) + `app/Packages/OpenCleanerClient` (SPM library)
- `launchd/` (LaunchAgent plist)
- `scripts/` (build/install helpers)
- `docs/` (this folder)

## Tooling (minimal deps)
- Go: stdlib + `go.uber.org/zap` (PRD)
- Swift: SwiftUI + (likely) Network.framework for unix-socket HTTP
- CI: GitHub Actions macOS runner (Go tests + `xcodebuild` build)

## Known hard problem
Swift `URLSession` does not natively support HTTP-over-unix sockets. Plan options:
1) Network.framework (`NWConnection` to `.unix(path)`) + small HTTP client/parser
2) Custom `URLProtocol` / socket adapter (more complex)
3) Fall back to TCP 127.0.0.1 + token auth (simpler, but violates PRD lock)

Default: pursue (1) to honor PRD.
