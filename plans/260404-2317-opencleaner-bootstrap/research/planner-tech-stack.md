# Tech stack + repo layout (from PRD)

Locked decisions:
- Go >=1.22
- SwiftUI macOS 14+
- IPC: HTTP/JSON over Unix domain socket
- Plugins later: WASM (wazero)
- Updates: Sparkle (app), Homebrew (CLI)

Recommended layout:
- `go/` module with:
  - `cmd/opencleanerd` (daemon)
  - `cmd/opencleaner` (CLI)
  - `internal/{safety,scan,clean,transport,stream}`
- `app/` Xcode project + `app/Packages/OpenCleanerClient` SPM lib
- `launchd/`, `scripts/`, `.github/workflows/`, `docs/`

Hard problem:
- Swift client for HTTP-over-unix: implement with Network.framework (preferred) or custom URLProtocol adapter.
