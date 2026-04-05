# OpenCleaner

Safety-first macOS cleaner for developer + system caches, with a native SwiftUI app and a Go daemon.

## Requirements
- macOS 14+
- Go 1.22+
- Xcode 15+ (for the app)

## Repo layout
- `go/` — daemon (`opencleanerd`) + CLI (`opencleaner`)
- `app/` — SwiftUI macOS app + `OpenCleanerClient` package
- `docs/` — design + transport + safety notes

## Quickstart (daemon + CLI)

### 1) Build
```bash
cd go
go test ./...
go build -o ../bin/opencleanerd ./cmd/opencleanerd
go build -o ../bin/opencleaner ./cmd/opencleaner
```

### 2) Run the daemon
```bash
SOCK="/tmp/opencleaner.$(id -u).sock"
./bin/opencleanerd -socket="$SOCK"
```

### 3) Use the CLI
```bash
SOCK="/tmp/opencleaner.$(id -u).sock"
./bin/opencleaner --socket="$SOCK" status
./bin/opencleaner --socket="$SOCK" scan --json
```

Clean is **dry-run by default**. To actually move items to Trash you must confirm:
```bash
./bin/opencleaner --socket="$SOCK" clean <id1,id2> --execute --yes
```

Exclude paths (never touch):
```bash
./bin/opencleaner --socket="$SOCK" clean <id1,id2> --dry-run --exclude=~/Library/Caches/Homebrew
```

## Quickstart (SwiftUI app)
- Open `app/OpenCleanerApp` in Xcode.
- Run the app.
- If the daemon is offline, configure the socket path in **Settings → Daemon → Socket path**.

## Safety model (high level)
- Default is **dry-run**.
- “Risky” targets are blocked unless explicitly allowed.
- Server enforces path safety (protected prefixes, symlink handling, TOCTOU mitigations).
- Audit log records every operation (including blocks).

See also:
- `docs/safety-reference-notes.md`
- `docs/ipc-sse-notes.md`
