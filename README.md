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
cd ..
```

### 2) Run the daemon
```bash
SOCK="/tmp/opencleaner.$(id -u).sock"
# Optional flags:
# - -log-level=debug|info|warn|error
# - -log-json
./bin/opencleanerd -socket="$SOCK" -log-level=info
```

### 2b) (Optional) Install daemon via launchd (user LaunchAgent)
```bash
SOCK="/tmp/opencleaner.$(id -u).sock"
./bin/opencleaner daemon install --binary-path="$(pwd)/bin/opencleanerd" --socket="$SOCK"
./bin/opencleaner daemon restart
./bin/opencleaner daemon uninstall
```

Artifacts:
- Plist: `~/Library/LaunchAgents/com.opencleaner.daemon.plist`
- Logs: `~/.opencleaner/logs/daemon.log` and `~/.opencleaner/logs/daemon.err` (daemon logs go to stderr by default → check `daemon.err`)

### 3) Use the CLI
```bash
SOCK="/tmp/opencleaner.$(id -u).sock"
# Global flags:
# - --socket=... selects the unix socket to talk to
# - --json prints machine-readable JSON

./bin/opencleaner --socket="$SOCK" status
./bin/opencleaner --socket="$SOCK" scan --json
```

Clean is **dry-run by default**. To actually move items to Trash you must confirm:
```bash
./bin/opencleaner --socket="$SOCK" clean <id1,id2> --execute --yes
```

Hard delete (advanced): `--strategy=delete --force` (cannot be undone; clears any existing undo manifest).

Undo (restores the last clean that moved items to Trash):
```bash
./bin/opencleaner --socket="$SOCK" undo
```
Note: undo only works after an executed trash-based clean, and only restores items originally under `$HOME`. If there’s nothing to undo, it fails (API returns 404).

Exclude paths (never touch):
```bash
./bin/opencleaner --socket="$SOCK" clean <id1,id2> --dry-run --exclude=~/Library/Caches/Homebrew
```

## E2E tests (macOS)
The E2E suite runs the real `opencleanerd` + `opencleaner` binaries against a temporary `$HOME` and a short unix socket path, and stubs `launchctl` for LaunchAgent lifecycle coverage.

```bash
cd go
go test ./... -tags=e2e -count=1
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
