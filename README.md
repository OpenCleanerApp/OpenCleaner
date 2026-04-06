# OpenCleaner

Safety-first macOS cleaner for developer + system caches, with a native SwiftUI app and a Go daemon.

## Requirements
- macOS 14+
- Go 1.22+
- Xcode 15+ (for the app)

## Repo layout
- `go/` — daemon (`opencleanerd`) + CLI (`opencleaner`)
- `app/` — SwiftUI macOS app (xcodegen) + `OpenCleanerClient` SPM package
- `scripts/` — build tooling (`create-dmg.sh`)
- `.github/workflows/` — CI and release pipelines
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

## Test coverage

Library packages (excluding `cmd/` and `daemon/`) are at **90.0%** statement coverage.

| Package | Coverage |
|---|---|
| `rules` | 100.0% |
| `stream` | 100.0% |
| `pkg/logger` | 100.0% |
| `analyzer` | 98.8% |
| `scheduler` | 97.8% |
| `engine` | 94.0% |
| `safety` | 94.0% |
| `transport` | 86.4% |
| `audit` | 85.7% |
| `scanner` | 82.1% |
| `cleaner` | 81.6% |
| `daemon` | 54.3% |

`scanner` (includes Docker) and `daemon` have known coverage ceilings due to external dependencies (Docker socket, `launchd`). `cmd/` packages are exercised by E2E tests rather than unit tests.

## Quickstart (SwiftUI app)

The Xcode project is generated via [xcodegen](https://github.com/yonaskolb/XcodeGen):
```bash
brew install xcodegen        # one-time
cd app && xcodegen generate  # creates OpenCleaner.xcodeproj
```
Then open `app/OpenCleaner.xcodeproj` in Xcode and run. On first launch an onboarding flow requests **Full Disk Access** (required for scanning caches).

If the daemon is offline, configure the socket path in **Settings → Daemon → Socket path**.

### Keyboard shortcuts

| Shortcut | Action |
|---|---|
| `⌘S` | Start scan |
| `⌘⌫` | Clean selected items |
| `⌘Z` | Undo last clean |
| `⌘A` | Select all |
| `Space` | Toggle item selection |

After cleaning, a **completion banner** appears with an undo option.

## Safety model (high level)
- Default is **dry-run**.
- “Risky” targets are blocked unless explicitly allowed.
- Server enforces path safety (protected prefixes, symlink handling, TOCTOU mitigations).
- Audit log records every operation (including blocks).

See also:
- `docs/safety-reference-notes.md`
- `docs/ipc-sse-notes.md`

## CI / CD

### CI (`.github/workflows/ci.yml`)
Runs on PRs to `main` and pushes to `develop`/`main`. Two **parallel** jobs:

| Job | Runner | What it does |
|---|---|---|
| `go-test` | `macos-15` | `go test`, `go vet`, build CLI + daemon |
| `swift-build` | `macos-15` | Resolve packages, `xcodebuild build`, `xcodebuild test` |

### Release (`.github/workflows/release.yml`)
Triggered by pushing a `v*` tag. Single job that:

1. Tests Go + Swift
2. Builds **universal** (arm64 + amd64) Go binaries via `lipo`
3. Archives the Swift app (Release, signed)
4. Embeds Go binaries in `.app/Contents/MacOS/` and re-signs everything
5. Notarizes via `notarytool` and staples the ticket
6. Creates a DMG (`scripts/create-dmg.sh`)
7. Uploads to a GitHub Release (with SHA-256 in release notes)
8. Signs the DMG with Sparkle EdDSA and updates `appcast.xml` on `gh-pages`
9. Dispatches a cask update to the Homebrew tap repo

#### Required GitHub secrets

| Secret | Purpose |
|---|---|
| `DEVELOPER_ID_CERT_BASE64` | Base64-encoded `.p12` Developer ID certificate |
| `DEVELOPER_ID_CERT_PASSWORD` | Password for the `.p12` |
| `APPLE_ID` | Apple ID for notarization |
| `APPLE_ID_PASSWORD` | App-specific password for notarization |
| `APPLE_TEAM_ID` | Apple Developer team ID |
| `SPARKLE_EDDSA_PRIVATE_KEY` | EdDSA key for Sparkle update signatures |
| `TAP_REPO_TOKEN` | PAT with repo access to `OpenCleanerApp/homebrew-tap` |

### Post-merge setup

Before the release pipeline can run end-to-end:

1. **Sparkle EdDSA keys** — Generate with `sparkle/bin/generate_keys`. Store the private key as the `SPARKLE_EDDSA_PRIVATE_KEY` secret.
2. **`gh-pages` branch** — Create with an initial `appcast.xml`:
   ```bash
   git checkout --orphan gh-pages
   cat > appcast.xml << 'EOF'
   <?xml version="1.0" encoding="utf-8"?>
   <rss version="2.0" xmlns:sparkle="http://www.andymatuschak.org/xml-namespaces/sparkle">
     <channel>
       <title>OpenCleaner</title>
     </channel>
   </rss>
   EOF
   git add appcast.xml && git commit -m "chore: initial appcast" && git push origin gh-pages
   git checkout main
   ```
3. **Homebrew tap repo** — Create `OpenCleanerApp/homebrew-tap` with:
   - `Casks/opencleaner.rb` (cask definition)
   - `.github/workflows/update-cask.yml` (listens for `repository_dispatch` event `update-cask`)
4. **GitHub secrets** — Configure all secrets listed above in the repository settings.
