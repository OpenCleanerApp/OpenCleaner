---
title: "Phase 2: Dev Mode Scanners, Smart Suggestions & Scheduler"
description: "Concrete scanner implementations (6 tools), smart suggestion engine, and daemon-side scheduler"
status: pending
priority: P1
effort: 40h
branch: phase1-parallel
tags: [scanners, suggestions, scheduler, dev-mode, phase2]
created: 2026-04-06
---

# Phase 2 Implementation Plan

## Table of Contents
1. [Architecture Overview](#1-architecture-overview)
2. [Stage 0: Foundations](#2-stage-0-foundations--2h)
3. [Stage 1: Scanner Implementations](#3-stage-1-scanner-implementations--20h)
4. [Stage 2: Smart Suggestions](#4-stage-2-smart-suggestions--8h)
5. [Stage 3: Scheduler](#5-stage-3-scheduler--8h)
6. [Stage 4: Integration & CLI](#6-stage-4-integration--cli--2h)
7. [Test Strategy](#7-test-strategy)
8. [Dependency Graph](#8-dependency-graph)
9. [Risk Register](#9-risk-register)

---

## 1. Architecture Overview

### New Packages

```
go/internal/
├── scanner/
│   ├── scanner.go         # (exists) — add shared walker + base helpers
│   ├── walker.go          # NEW: bounded recursive directory walker
│   ├── node.go            # NEW: Node.js scanner
│   ├── docker.go          # NEW: Docker scanner
│   ├── xcode.go           # NEW: Xcode deep scanner
│   ├── homebrew.go        # NEW: Homebrew scanner
│   ├── python.go          # NEW: Python scanner
│   ├── rust.go            # NEW: Rust scanner
│   └── scanner_test.go    # NEW: shared test helpers + per-scanner tests
├── analyzer/
│   └── suggestions.go     # NEW: smart suggestion engine
├── scheduler/
│   └── scheduler.go       # NEW: cron-like auto-scan scheduler
```

### Design Principles
- Every scanner implements `rules.Scanner` — zero changes to engine merge logic
- Recursive walkers are bounded (max depth), context-cancelable, skip `.git`/`vendor`/`node_modules` (when not the target)
- CLI-invoking scanners (Docker, Homebrew) fail gracefully if tool not installed → return empty `[]Rule`, no error
- All produced `Rule` structs pass through existing `safety.ValidatePathSafety` unchanged

---

## 2. Stage 0: Foundations — ~2h

**No dependencies. Must land first.**

### 2a. Shared Bounded Walker (`scanner/walker.go`)

All recursive scanners (node_modules, `__pycache__`, `target/`, `.venv`) need a common bounded directory walker.

```go
// WalkConfig controls bounded recursive discovery.
type WalkConfig struct {
    RootDirs    []string          // starting directories to walk (e.g., ~, ~/Projects)
    TargetName  string            // directory name to match (e.g., "node_modules")
    MaxDepth    int               // max recursion depth (default 10)
    SkipNames   map[string]bool   // dir names to skip entirely (e.g., .git, vendor)
    SkipHidden  bool              // skip dot-prefixed dirs (default true)
    OnMatch     func(path string) // callback per match
}

func Walk(ctx context.Context, cfg WalkConfig) error
```

Implementation details:
- Uses `filepath.WalkDir` with depth tracking via `strings.Count(rel, string(os.PathSeparator))`
- When `TargetName` match found, **do not descend** into it (e.g., don't recurse into `node_modules/*/node_modules`)
- Default `SkipNames`: `.git`, `.hg`, `vendor`, `__MACOSX`
- Context-cancelable: check `ctx.Done()` every N directories (batch of 100)
- Return early on context cancellation

### 2b. Type Additions (`pkg/types/types.go`)

Add `Suggestion` type for E.6:

```go
type Suggestion struct {
    ItemID      string      `json:"item_id"`
    Message     string      `json:"message"`
    Priority    float64     `json:"priority"`     // 0..1 (1 = highest)
    Rationale   string      `json:"rationale"`
    SafetyLevel SafetyLevel `json:"safety_level"`
}
```

Extend `ScanResult`:
```go
type ScanResult struct {
    // ... existing fields ...
    Suggestions []Suggestion `json:"suggestions,omitempty"`
}
```

### 2c. CLI Executor Helper (`scanner/exec.go`)

Thin wrapper for Docker/Homebrew CLI calls:

```go
// RunCommand executes a command with context timeout, returns stdout.
// Returns ("", nil) if binary not found (tool not installed).
func RunCommand(ctx context.Context, name string, args ...string) (string, error)
```

- Uses `exec.CommandContext`
- `exec.ErrNotFound` → return `("", nil)` (graceful skip)
- Timeout via context (caller passes `context.WithTimeout`)
- Captures stdout only; stderr logged at debug level

### 2d. Scanner Registration in Daemon

In `cmd/opencleanerd/main.go`, after `eng := engine.New(...)`:

```go
home, _ := os.UserHomeDir()
scanRoots := scanner.DefaultScanRoots(home) // [home, home+"/Projects", home+"/Developer", home+"/src", home+"/go/src"]

eng.AddScanner(scanner.NewNodeScanner(home, scanRoots))
eng.AddScanner(scanner.NewDockerScanner())
eng.AddScanner(scanner.NewXcodeScanner(home))
eng.AddScanner(scanner.NewHomebrewScanner(home))
eng.AddScanner(scanner.NewPythonScanner(home, scanRoots))
eng.AddScanner(scanner.NewRustScanner(home, scanRoots))
```

`DefaultScanRoots` returns only directories that actually exist.

---

## 3. Stage 1: Scanner Implementations — ~20h

### Priority Order (by disk space impact)

| Priority | Scanner | Typical Space | Effort |
|----------|---------|---------------|--------|
| 1 | Node.js | 5–50 GB | 3h |
| 2 | Docker | 10–100 GB | 4h |
| 3 | Xcode (deep) | 5–40 GB | 3h |
| 4 | Python | 1–10 GB | 2h |
| 5 | Rust | 2–20 GB | 3h |
| 6 | Homebrew | 1–5 GB | 3h |

**Parallelism**: Scanners 1, 3, 4, 5 can be implemented in parallel (no shared deps beyond walker). Scanner 2 (Docker) and 6 (Homebrew) share exec helper but are independent of each other.

---

### 3.1 Node.js Scanner (`scanner/node.go`) — 3h

**ID**: `nodejs`

**Discovers**:

| Rule ID | Target | Discovery Method | Safety |
|---------|--------|------------------|--------|
| `nodejs-node-modules-{hash}` | `node_modules/` dirs | Recursive walk from scan roots, match dir name | Safe |
| `nodejs-npm-cache` | `~/.npm/_cacache` | Known path (already builtin — **dedupe by ID**) | Safe |
| `nodejs-pnpm-store` | `~/Library/pnpm/store` | Known path | Safe |
| `nodejs-yarn-cache-v1` | `~/Library/Caches/Yarn` | Known path (already builtin — dedupe) | Safe |
| `nodejs-yarn-berry-cache` | `~/.yarn/berry/cache` | Known path | Safe |

**Key decisions**:
- `node_modules` discovery uses bounded walker; `TargetName: "node_modules"`, once found → `SkipDir` (don't recurse into nested node_modules)
- Each discovered `node_modules` gets a unique rule ID: `nodejs-node-modules-` + hash of absolute path (use first 8 chars of sha256)
- For npm/pnpm/yarn caches: overlap with builtin `npm-cache` and `yarn-cache` is handled by engine's existing dedupe (scanner IDs differ from builtin IDs, so we use the **same ID strings** as builtin for caches, allowing engine dedupe to skip duplicates)
- Actually: builtins use `npm-cache` and `yarn-cache`. Scanner should produce rules with same IDs for the cache paths → engine dedupe drops them. Or use different IDs and let both appear. **Decision**: use same IDs as builtin → clean dedupe, scanner acts as a superset.

**Safety**: Safe. `node_modules` is always regenerable via `npm install`.

**Test strategy**:
- Unit: create temp dir tree with multiple `node_modules` dirs at various depths. Verify walk finds all, respects max depth, skips `.git`.
- Unit: verify scanner returns correct Rule structs with abs paths, correct IDs.
- Integration (E2E): real `node_modules` in a test project dir.

---

### 3.2 Docker Scanner (`scanner/docker.go`) — 4h

**ID**: `docker`

**Discovers**:

| Rule ID | Target | Discovery Method | Safety |
|---------|--------|------------------|--------|
| `docker-unused-images` | Unused Docker images | `docker image ls --format json --filter dangling=false` + cross-ref with running containers | Moderate |
| `docker-dangling-images` | `<none>:<none>` images | `docker image ls --format json --filter dangling=true` | Safe |
| `docker-build-cache` | Build cache | `docker builder du --verbose` (size only) | Safe |
| `docker-dangling-volumes` | Unused volumes | `docker volume ls --format json --filter dangling=true` | Risky |

**Key decisions**:
- All discovery via `docker` CLI (not Docker API) — per constraint #3
- If `docker` binary not found → return `([]Rule{}, nil)` — graceful skip
- Each target gets a synthetic "path" for the Rule struct: `docker://images/unused`, `docker://images/dangling`, `docker://cache/build`, `docker://volumes/dangling`
  - **Problem**: `Rule.Path` must be absolute file path per `validateRule()` (which checks `filepath.IsAbs`).
  - **Solution**: Store a sentinel path under `~/.opencleaner/docker-targets/` as a marker. The clean engine will need special handling for Docker — **BUT** current clean engine just calls `os.Remove/MoveToTrash` on the path.
  - **Better approach**: Docker clean operations invoke `docker system prune`, `docker volume prune`, `docker builder prune` etc. This means Docker rules need a **custom clean callback** or the engine needs awareness.
  - **Simplest approach (KISS)**: Docker scanner reports only **size information** for suggestion purposes. For cleaning, Docker items use a special clean strategy that invokes Docker CLI commands.

**Architecture decision — Docker cleaning**:
Since `engine.Clean()` currently only does `MoveToTrash` or `SafeRemove` on filesystem paths, and Docker artifacts aren't filesystem paths the user should touch directly, we need:

Option A: Add `CleanFunc` callback to `rules.Rule` (invasive, complicates undo)
Option B: Docker scanner targets the **Docker data root** (usually `/var/lib/docker` or `~/Library/Containers/com.docker.docker/Data`) — too coarse, unsafe
**Option C (chosen)**: Docker scanner only reports Docker Desktop's disk image and data dir as scannable. Clean action invokes `docker system prune` via a registered clean hook in engine. Add optional `CleanCmd` field to `Rule`:

```go
type Rule struct {
    // ... existing fields ...
    CleanCmd []string // optional: if set, engine runs this command instead of fs delete
}
```

Engine changes: if `rule.CleanCmd` is set, execute command instead of trash/delete. No undo for command-based cleans. This is minimal, opt-in, and backward-compatible.

**Concrete Docker rules with CleanCmd approach**:

| Rule ID | CleanCmd | Notes |
|---------|----------|-------|
| `docker-dangling-images` | `["docker", "image", "prune", "-f"]` | Safe, removes `<none>` images |
| `docker-build-cache` | `["docker", "builder", "prune", "-f"]` | Safe, removes build cache |
| `docker-dangling-volumes` | `["docker", "volume", "prune", "-f"]` | Risky, data loss if volume has state |
| `docker-system` | `["docker", "system", "prune", "-f"]` | Moderate, all-in-one cleanup |

For `Path`: use `~/.opencleaner/markers/docker-{id}` — a zero-byte marker file created by scanner. Engine Lstat check passes, but actual cleanup is via `CleanCmd`. Alternative: add `Virtual bool` to Rule and skip Lstat in engine. **Better**: just use Docker Desktop's actual data path for `Path` field:
- `~/Library/Containers/com.docker.docker/Data` (Docker Desktop for Mac)

This gives real size data. Each sub-rule can point to the same parent dir for existence check but clean via `CleanCmd`.

**Actually, simplest viable approach**: Use `Path` as the real Docker data directory for existence/size purposes. Size is obtained from `docker system df` CLI output. `CleanCmd` handles the actual cleanup.

**Final design**:
- Scanner calls `docker system df --format json` to get sizes
- For Path field: use actual Docker Desktop data dir `~/Library/Containers/com.docker.docker/Data` for existence check, but size comes from `docker system df`
- Override `getPathSize` concern: engine sizes rules by walking the fs path. Docker scanner should pre-set size in the Rule. **Problem**: `Rule` has no Size field; engine calculates size independently.
- **Resolution**: Add optional `Size *int64` to Rule. If set, engine skips `getPathSize` for that rule and uses the provided value. This is useful for Docker (CLI-reported size) and avoids walking massive Docker data dirs.

**Implementation summary for Docker**:

```go
type Rule struct {
    // ... existing ...
    CleanCmd []string  // if set, execute command instead of fs delete
    Size     *int64    // if set, skip getPathSize and use this value
}
```

Engine changes (small, backward-compatible):
1. In worker loop: if `rule.Size != nil`, use `*rule.Size` instead of `getPathSize()`
2. In Clean: if `rule.CleanCmd != nil`, exec command instead of trash/delete; skip undo for these items
3. Store CleanCmd in ScanItem (new optional field) for Clean to access

**Safety**: Moderate for images/cache, Risky for volumes.

**Test strategy**:
- Unit: mock `RunCommand` output, verify parsing of `docker system df` and `docker image ls` JSON
- Unit: verify graceful skip when Docker not installed
- Integration: only if Docker available (build-tag or env-gated)

---

### 3.3 Xcode Deep Scanner (`scanner/xcode.go`) — 3h

**ID**: `xcode`

Builtins already cover DerivedData, Archives, iOS DeviceSupport, Simulator Caches. This scanner adds:

| Rule ID | Target | Discovery Method | Safety |
|---------|--------|------------------|--------|
| `xcode-simulator-runtimes` | `~/Library/Developer/CoreSimulator/Profiles/Runtimes` | Known path | Moderate |
| `xcode-watchos-device-support` | `~/Library/Developer/Xcode/watchOS DeviceSupport` | Known path | Moderate |
| `xcode-tvos-device-support` | `~/Library/Developer/Xcode/tvOS DeviceSupport` | Known path | Moderate |
| `xcode-old-device-support` | Individual version dirs in `iOS DeviceSupport/` | List dirs, filter by age | Moderate |
| `xcode-simulator-devices` | `~/Library/Developer/CoreSimulator/Devices` | Known path | Moderate |
| `xcode-previews` | `~/Library/Developer/Xcode/UserData/Previews` | Known path | Safe |

**Key decisions**:
- Simulator runtimes can be 5-10GB each — high value target
- `xcode-old-device-support`: list subdirs of `~/Library/Developer/Xcode/iOS DeviceSupport/`, report each as a separate rule with version in name. Users can selectively keep current device versions.
- All known paths, no recursive walking needed
- Dedupe with builtins: use different IDs (builtins target parent dirs, this scanner targets specific subdirs or individual items)

**Safety**: Moderate. Xcode will re-download runtimes/symbols as needed; first debug session may be slower.

**Test strategy**:
- Unit: create mock directory structure, verify rule generation
- Unit: verify empty results when Xcode dirs don't exist

---

### 3.4 Python Scanner (`scanner/python.go`) — 2h

**ID**: `python`

| Rule ID | Target | Discovery Method | Safety |
|---------|--------|------------------|--------|
| `python-pycache-{hash}` | `__pycache__/` dirs | Recursive walk | Safe |
| `python-venv-{hash}` | `.venv/` dirs | Recursive walk | Moderate |
| `python-virtualenvs-{hash}` | `venv/` dirs | Recursive walk (match `venv` containing `pyvenv.cfg`) | Moderate |
| `python-pip-cache` | `~/Library/Caches/pip` | Known path | Safe |
| `python-pipx-cache` | `~/.local/pipx/venvs` | Known path | Moderate |

**Key decisions**:
- `__pycache__` walk: `TargetName: "__pycache__"` with bounded walker
- `.venv` walk: `TargetName: ".venv"`, validate it's actually a virtualenv by checking for `pyvenv.cfg` inside
- `venv/` walk: trickier — common name. Only match if dir contains `pyvenv.cfg` (confirmation heuristic)
- `pip cache` on macOS: `~/Library/Caches/pip`
- Hash-based IDs for discovered dirs (same pattern as node_modules)

**Safety**: `__pycache__` is Safe (auto-regenerated). `.venv` is Moderate (may contain installed packages not in requirements.txt).

**Test strategy**:
- Unit: temp dir with `__pycache__` and `.venv` containing `pyvenv.cfg`. Verify discovery.
- Unit: verify `venv/` without `pyvenv.cfg` is skipped.

---

### 3.5 Rust Scanner (`scanner/rust.go`) — 3h

**ID**: `rust`

| Rule ID | Target | Discovery Method | Safety |
|---------|--------|------------------|--------|
| `rust-target-{hash}` | `target/` dirs in Rust projects | Recursive walk, validate `Cargo.toml` in parent | Safe |
| `rust-cargo-registry` | `~/.cargo/registry` | Known path (already builtin — dedupe) | Moderate |
| `rust-cargo-git` | `~/.cargo/git` | Known path | Moderate |
| `rust-cargo-bin-cache` | `~/.cargo/.package-cache` | Known path | Safe |

**Key decisions**:
- `target/` is a common dir name. Must validate: walk matches `target/` only if parent dir contains `Cargo.toml` (confirmation heuristic)
- Walker uses `TargetName: "target"` but `OnMatch` callback checks `Cargo.toml` existence before reporting
- Reuse builtin `cargo-registry` ID → engine dedupe handles overlap
- `~/.cargo/git` holds git checkouts of crate deps — safe to clean, Cargo will re-clone

**Safety**: `target/` is Safe (`cargo build` regenerates). Registry is Moderate (re-download).

**Test strategy**:
- Unit: temp dir with `target/` + `Cargo.toml` siblings. Also `target/` without `Cargo.toml` — verify skip.
- Unit: verify known path rules generated correctly.

---

### 3.6 Homebrew Scanner (`scanner/homebrew.go`) — 3h

**ID**: `homebrew`

| Rule ID | Target | Discovery Method | Safety |
|---------|--------|------------------|--------|
| `homebrew-cache` | `~/Library/Caches/Homebrew` | Known path (already builtin — dedupe) | Safe |
| `homebrew-old-versions` | Old formula versions | `brew list --versions` + `brew info --json=v2 --installed` to find old | Moderate |
| `homebrew-logs` | `~/Library/Logs/Homebrew` | Known path | Safe |

**Key decisions**:
- "Unused deps" (`brew autoremove --dry-run`) is complex and potentially destructive → **defer to suggestion only**, not a scannable rule. The suggestion engine can mention it.
- "Old formula versions" via `brew cleanup --dry-run` which lists files that would be removed. Parse output, sum sizes, create single rule with `CleanCmd: ["brew", "cleanup"]`.
- Graceful skip if `brew` not found.
- `homebrew-cache` reuses builtin ID → dedupe.

**Concrete approach for old versions**:
```
$ brew cleanup --dry-run
# outputs list of files/dirs that would be removed
```
- Parse this output, create single rule `homebrew-old-versions` with `CleanCmd: ["brew", "cleanup"]`
- Size: sum of files listed in `--dry-run` output, or run `brew cleanup --dry-run` and parse size info

**Safety**: Safe (brew cleanup only removes outdated downloads). Moderate for old versions.

**Test strategy**:
- Unit: mock `brew cleanup --dry-run` output, verify parsing
- Unit: verify graceful skip when brew not found

---

## 4. Stage 2: Smart Suggestions — ~8h

**Depends on**: Stage 0 (Suggestion type), Stage 1 scanners delivering results with `LastAccess` data.

### 4.1 Suggestion Engine (`internal/analyzer/suggestions.go`)

```go
package analyzer

type SuggestionEngine struct {
    rules []SuggestionRule
}

type SuggestionRule struct {
    Match     func(item types.ScanItem) bool
    Generate  func(item types.ScanItem) *types.Suggestion
}

func NewSuggestionEngine() *SuggestionEngine
func (e *SuggestionEngine) Analyze(items []types.ScanItem) []types.Suggestion
```

### 4.2 Built-in Suggestion Rules

| Rule | Trigger | Message Template | Priority Formula |
|------|---------|-----------------|-----------------|
| Large & Old | Size > 1GB AND age > 30 days | "{name}: {size} unused for {days} days → safe to remove" | `size_gb * 0.3 + age_days/30 * 0.3 + safety_score * 0.4` |
| Docker Heavy | Docker items > 10GB | "Docker using {size} — run cleanup to reclaim space" | `size_gb * 0.5 + 0.5` |
| DerivedData Stale | DerivedData > 14 days old | "DerivedData {days} days old → rebuild takes ~2min" | `0.7` |
| Simulator Runtimes | Any simulator runtimes found | "Simulator runtimes: {size} each — keep only versions you test against" | `size_gb * 0.4 + 0.3` |
| Quick Wins | `__pycache__`, `node_modules` | "{count} {name} dirs totaling {size} — always safe to clean" | `0.6` |
| Homebrew Cleanup | Old Homebrew versions found | "Homebrew has {size} of outdated downloads — safe to remove" | `0.5` |

### 4.3 Priority Calculation

Normalized 0..1 score. Factors:
- **Size weight** (40%): `min(size_bytes / 10GB, 1.0) * 0.4`
- **Safety weight** (35%): Safe=1.0, Moderate=0.6, Risky=0.2 → `* 0.35`
- **Age weight** (25%): `min(days_since_access / 90, 1.0) * 0.25`

Suggestions sorted by priority descending. Top 10 returned.

### 4.4 Integration Point

In `engine.Scan()`, after collecting all items:
```go
suggestions := analyzer.NewSuggestionEngine().Analyze(items)
res.Suggestions = suggestions
```

### 4.5 LastAccess Population

`ScanItem.LastAccess` is already in the type (`*time.Time`). Engine's worker loop should populate it:
- For regular files/dirs: use `os.Stat().ModTime()` as proxy (macOS atime is unreliable with noatime)
- Store on the ScanItem during the sizing phase

---

## 5. Stage 3: Scheduler — ~8h

**Depends on**: Stage 0 (type additions), Stage 1 (scanners working).

### 5.1 Scheduler Package (`internal/scheduler/scheduler.go`)

```go
package scheduler

type Schedule struct {
    Interval  Interval  `json:"interval"`  // daily, weekly, monthly
    TimeOfDay string    `json:"time"`      // "03:00" (24h format)
    DayOfWeek int       `json:"day,omitempty"` // 0=Sun..6=Sat (for weekly)
    AutoClean *AutoCleanConfig `json:"auto_clean,omitempty"`
}

type Interval string
const (
    IntervalDaily   Interval = "daily"
    IntervalWeekly  Interval = "weekly"
    IntervalMonthly Interval = "monthly"
)

type AutoCleanConfig struct {
    Enabled      bool     `json:"enabled"`
    SafetyMax    string   `json:"safety_max"`     // "safe" or "moderate" — max safety level to auto-clean
    MinAge       int      `json:"min_age_days"`   // only auto-clean items older than N days
    CategoryIDs  []string `json:"category_ids"`   // limit to specific categories; empty = all
    DryRun       bool     `json:"dry_run"`        // default true for safety
}

type Scheduler struct {
    engine   *engine.Engine
    broker   *stream.Broker
    config   *Schedule
    configMu sync.RWMutex
    stopCh   chan struct{}
    logger   *slog.Logger
}

func New(eng *engine.Engine, broker *stream.Broker, logger *slog.Logger) *Scheduler
func (s *Scheduler) Start(cfg Schedule) error
func (s *Scheduler) Stop()
func (s *Scheduler) UpdateConfig(cfg Schedule) error
func (s *Scheduler) Config() Schedule
func (s *Scheduler) NextRun() time.Time
```

### 5.2 Implementation Details

- **Timer-based**, not cron: calculates next run time from config, sets `time.Timer`
- On tick: calls `engine.Scan(ctx)` → optionally `engine.Clean(ctx, autoCleanReq)` if auto-clean enabled
- Publishes `ProgressEvent{Type: "scheduled_scan", ...}` through existing SSE broker
- **macOS notification**: shell out to `osascript -e 'display notification ...'` on scan completion (no external deps)
- Config persisted at `~/.opencleaner/scheduler.json`
- Default: disabled. User enables via CLI or API.

### 5.3 Config Persistence

```go
func LoadConfig(home string) (*Schedule, error)    // from ~/.opencleaner/scheduler.json
func SaveConfig(home string, cfg Schedule) error
```

### 5.4 API Additions

New endpoints in `transport/server.go`:

| Method | Path | Body | Response |
|--------|------|------|----------|
| GET | `/api/v1/scheduler` | — | `Schedule` + `next_run` |
| PUT | `/api/v1/scheduler` | `Schedule` JSON | `Schedule` + `next_run` |
| DELETE | `/api/v1/scheduler` | — | `{"ok": true}` |

### 5.5 CLI Additions

```
opencleaner schedule status              # show current schedule
opencleaner schedule set --interval=weekly --time=03:00 --day=0
opencleaner schedule set --auto-clean --safety-max=safe --min-age=7
opencleaner schedule disable
```

### 5.6 Daemon Integration

In `cmd/opencleanerd/main.go`:
```go
sched := scheduler.New(eng, broker, log)
if cfg, err := scheduler.LoadConfig(home); err == nil && cfg != nil {
    _ = sched.Start(*cfg)
}
defer sched.Stop()
// pass sched to transport.Server for API handlers
```

---

## 6. Stage 4: Integration & CLI — ~2h

### 6.1 Wire All Scanners in Daemon

Update `cmd/opencleanerd/main.go` to register all 6 scanners (see Stage 0d).

### 6.2 CLI `scan` Output Enhancement

Update `print()` in CLI to show suggestions:
```
items=24 total=47382917120 bytes (44.1 GB)
- nodejs-node-modules-a3f2e91c (~/ Projects/foo/node_modules): 412 MB [safe]
...

Suggestions:
  ⚡ Docker using 31.2 GB — run cleanup to reclaim space [priority: 0.92]
  💡 DerivedData 21 days old → rebuild takes ~2min [priority: 0.70]
  🧹 12 node_modules dirs totaling 8.3 GB — always safe to clean [priority: 0.65]
```

### 6.3 Engine Changes Summary

Minimal, backward-compatible additions to `rules.Rule`:

```go
type Rule struct {
    ID         string
    Name       string
    Path       string
    Category   types.Category
    Safety     types.SafetyLevel
    SafetyNote string
    Desc       string
    CleanCmd   []string  // NEW: if set, exec instead of fs delete
    PresetSize *int64    // NEW: if set, skip getPathSize
}
```

Engine changes:
1. `getPathSize` bypass when `PresetSize != nil` (~2 lines)
2. Clean path: if `CleanCmd` set, exec command; set undo=unavailable for those items (~15 lines)
3. Store `CleanCmd` on `ScanItem` for clean handler access (new field on ScanItem, omitempty)
4. Call suggestion engine after scan, attach to result (~3 lines)

---

## 7. Test Strategy

### Unit Tests (per scanner)

Each scanner gets a `_test.go` in `scanner/` with:
- **Filesystem mock**: `t.TempDir()` with synthetic directory structures
- **CLI mock**: `RunCommand` accepts an interface/func for testing; inject mock output
- **Edge cases**: missing tool binary, empty directories, permission denied, context cancellation
- **Walker tests**: max depth enforcement, skip patterns, symlink handling

### Table-Driven Tests

```go
func TestNodeScanner(t *testing.T) {
    tests := []struct{
        name    string
        tree    map[string]string  // path -> content ("DIR" for dirs)
        wantIDs []string
    }{
        {"finds nested node_modules", ...},
        {"respects max depth", ...},
        {"skips .git", ...},
        {"empty project dir", ...},
    }
}
```

### Suggestion Engine Tests

- Table-driven: given N ScanItems with known sizes/ages → verify suggestion messages and priority ordering
- Edge: zero items, all items below thresholds

### Scheduler Tests

- Timer calculation: given "weekly, Sunday 03:00" and current time, verify next run time
- Config persistence: save/load round-trip
- Auto-clean config validation: reject unsafe auto-clean without explicit opt-in

### E2E Tests

Extend `e2e/` with:
- Full scan with scanners registered → verify new scanner IDs appear in results
- Schedule set/get via API
- Docker scanner skip when Docker not installed (verify no error)

---

## 8. Dependency Graph

```
Stage 0: Foundations (walker, types, exec helper)
    │
    ├──→ Stage 1a: Node.js scanner ──┐
    ├──→ Stage 1b: Python scanner  ──┤
    ├──→ Stage 1c: Rust scanner    ──┤  (parallel — all use walker)
    ├──→ Stage 1d: Xcode scanner   ──┤  (no walker, known paths)
    ├──→ Stage 1e: Docker scanner  ──┤  (uses exec helper)
    ├──→ Stage 1f: Homebrew scanner──┤  (uses exec helper)
    │                                │
    │    ┌───────────────────────────┘
    │    │
    ├──→ Stage 2: Smart Suggestions (needs ScanItems from Stage 1)
    │    │
    ├──→ Stage 3: Scheduler (needs engine working, independent of Stage 2)
    │    │
    └──→ Stage 4: Integration & CLI (wire everything, final)
```

**Critical path**: Stage 0 → Stage 1 (any scanner) → Stage 4
**Parallel tracks**: Stage 2 and Stage 3 can proceed in parallel after Stage 0

---

## 9. Risk Register

| Risk | Impact | Mitigation |
|------|--------|------------|
| `node_modules` walk on large home dirs is slow | Scan >10s | Bound scan roots to common project dirs; configurable roots; walker respects maxDepth=10 |
| Docker CLI calls slow or hanging | Scan blocks | Context timeout (10s per command); fail gracefully |
| `target/` false positives (non-Rust dirs named "target") | Wrong items shown | Require `Cargo.toml` in parent dir as confirmation |
| `venv/` false positives | Wrong items shown | Require `pyvenv.cfg` inside dir |
| `CleanCmd` addition to Rule/Engine | Complexity | Minimal: <20 lines engine change, opt-in field, no undo for cmd-based cleans |
| Scheduler auto-clean deletes wanted files | Data loss | Default dry-run=true; safety_max=safe; require explicit opt-in |
| `validateRule` rejects Docker virtual paths | Scan fails | Docker scanner uses real Docker data dir path for existence check |
| Scanner errors block entire scan | Partial failure | Engine already wraps scanner errors. Consider: log warning + continue instead of hard fail (currently hard fails). **Recommend changing to soft fail** — log error, skip scanner, continue with others. |

### Recommended Engine Change: Soft-Fail Scanners

Current behavior (line 79): `return types.ScanResult{}, fmt.Errorf(...)` — one failed scanner kills the whole scan.

**Proposed**: Log warning, skip failed scanner, continue:
```go
for _, sc := range scanners {
    rs, err := sc.Scan(ctx)
    if err != nil {
        slog.Warn("scanner failed, skipping", "scanner", sc.ID(), "err", err)
        continue  // instead of return error
    }
    // ... merge rules
}
```

This is critical for real-world use where Docker might not be running, brew might be broken, etc.

---

## Appendix: File Change Summary

| File | Change Type | Lines (est) |
|------|-------------|-------------|
| `internal/scanner/walker.go` | New | ~80 |
| `internal/scanner/exec.go` | New | ~40 |
| `internal/scanner/node.go` | New | ~100 |
| `internal/scanner/docker.go` | New | ~150 |
| `internal/scanner/xcode.go` | New | ~90 |
| `internal/scanner/homebrew.go` | New | ~100 |
| `internal/scanner/python.go` | New | ~90 |
| `internal/scanner/rust.go` | New | ~90 |
| `internal/scanner/scanner_test.go` | New | ~300 |
| `internal/analyzer/suggestions.go` | New | ~150 |
| `internal/analyzer/suggestions_test.go` | New | ~120 |
| `internal/scheduler/scheduler.go` | New | ~200 |
| `internal/scheduler/scheduler_test.go` | New | ~120 |
| `internal/rules/rules.go` | Edit | +2 (CleanCmd, PresetSize fields) |
| `internal/engine/engine.go` | Edit | +25 (soft-fail, preset size, cmd clean, suggestions) |
| `pkg/types/types.go` | Edit | +15 (Suggestion type, ScanItem.CleanCmd) |
| `internal/transport/server.go` | Edit | +40 (scheduler endpoints) |
| `cmd/opencleanerd/main.go` | Edit | +15 (register scanners, scheduler) |
| `cmd/opencleaner/main.go` | Edit | +30 (schedule subcommand, suggestion display) |
| **Total new code** | | **~1800 lines** |
