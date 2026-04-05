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
  - `OpenCleanerApp/` (SwiftUI app)
  - `Packages/OpenCleanerClient/` (SPM client library: daemon + CLI clients)
- `docs/` (documentation)
- `plans/` (planning notes)

## Tooling / dependencies (current)
- Go: standard library (daemon uses stdlib `log` for debug prints; audit log is written by `internal/audit`)
- Swift: SwiftUI + Network.framework (`NWConnection`) for HTTP-over-unix-socket
- Observability:
  - Go SSE logs gated by `OPENCLEANER_DEBUG` or `OPENCLEANER_DEBUG_SSE`
  - Swift SSE lifecycle logs via OSLog (`subsystem: OpenCleanerClient`, `category: sse`)
