# 📄 OpenCleaner — Product Requirements Document (v2.0)

> **Open-source CleanMyMac for Developers**
> Last updated: 2026-04-04

---

## ⚠️ MANDATORY RULE: Reference Implementation

> **ALL developers MUST reference the [`clmm-clean-my-mac-cli`](https://github.com/0xAstroAlpha/clmm-clean-my-mac-cli) codebase before implementing ANY scan, clean, or file deletion logic.**
>
> Deleting files is inherently dangerous. This reference implementation has been tested and validated. Every scanner, safety guard, and deletion operation in OpenCleaner must be verified against the patterns established in this codebase.

### Why This Rule Exists

OpenCleaner's core function is **deleting files from the user's system**. A single bug in path validation, scanner targeting, or deletion logic can cause irreversible data loss. The `clmm-clean-my-mac-cli` repo serves as the **trusted, battle-tested reference** for:

1. **Which files/directories are safe to delete** — the exact paths and categories
2. **How to validate paths** — the safety guard patterns
3. **How to perform deletion** — the security measures during removal

### What to Reference (by file)

| Reference File | What to Learn | Critical Patterns |
|---|---|---|
| `src/utils/fs.ts` | Path safety validation, TOCTOU protection | `isProtectedPath()`, `validatePathSafety()`, `removeItem()` — re-checks file type before deletion to prevent symlink attacks |
| `src/utils/paths.ts` | 90+ validated cleanup target paths | `PATHS` object with exact macOS cache/artifact locations, `expandPath()` with traversal detection, `SYSTEM_PATHS` never-touch list |
| `src/types.ts` | Safety level classification per category | `SafetyLevel: 'safe' \| 'moderate' \| 'risky'` — each category has explicit safety notes |
| `src/scanners/base-scanner.ts` | Scanner interface pattern | `BaseScanner` abstract class with `scan()` → `clean()` lifecycle |
| `src/scanners/dev-cache.ts` | 50+ dev cache targets with categorization | Grouped by: Core, IDE, Electron, API tools, CI/CD, Misc |
| `src/commands/purge.ts` | 45+ build artifact patterns, Docker volume detection | `ARTIFACT_PATTERNS`, `deduplicateArtifacts()`, Docker compose volume scanning |
| `src/commands/check.ts` | System health checks (10 checks) | SMART status, SIP, FileVault, Gatekeeper verification |
| `src/commands/optimize.ts` | 11 optimization tasks | DNS flush, memory purge, font cache, Launch Services rebuild |

### Mandatory Reference Checklist

Before implementing each component, developers MUST:

- [ ] **Scanner rules**: Cross-reference target paths against `src/utils/paths.ts` (90+ paths) and `src/scanners/dev-cache.ts` (50+ targets). Do NOT invent new paths without validation.
- [ ] **Safety guard**: Port the exact `PROTECTED_PATHS` + `ALLOWED_PATHS` + `SYSTEM_PATHS` lists from `src/utils/fs.ts` and `src/utils/paths.ts`.
- [ ] **Path validation**: Implement `isProtectedPath()`, `validatePathSafety()`, `hasTraversalPattern()`, and `expandPath()` with traversal detection — ported from the reference.
- [ ] **Deletion logic**: Implement TOCTOU protection (re-check file type immediately before deletion to prevent symlink replacement attacks) as done in `removeItem()`.
- [ ] **Build artifacts**: Reference `ARTIFACT_PATTERNS` (45+ patterns) in `src/commands/purge.ts` for the purge/scan feature. Include Docker volume detection from compose files.
- [ ] **Safety levels**: Every scan category MUST have a `SafetyLevel` (`safe`, `moderate`, `risky`) with explicit `safetyNote` explaining the consequences — as defined in `src/types.ts`.
- [ ] **Dry-run mode**: All commands must support `--dry-run` flag that previews operations without making changes.

### Key Safety Patterns to Port (Go equivalents)

```go
// Port from clmm: src/utils/fs.ts → internal/safety/guard.go

// 1. Protected paths — NEVER delete these
var PROTECTED_PATHS = []string{
    "/System", "/usr", "/bin", "/sbin", "/etc",
    "/var/log", "/var/db", "/var/root",
    "/private/var/db", "/private/var/root", "/private/var/log",
    "/Library/Apple", "/Applications/Utilities",
}

// 2. Allowed paths — safe even if they match protected patterns
var ALLOWED_PATHS = []string{
    "/tmp", "/private/tmp", "/var/tmp",
    "/private/var/tmp", "/var/folders", "/private/var/folders",
}

// 3. TOCTOU protection — re-stat before deletion
func SafeRemove(path string) error {
    if err := ValidatePathSafety(path); err != nil {
        return err
    }
    // Re-check file type immediately before deletion
    // to prevent symlink replacement attacks
    info, err := os.Lstat(path)
    if err != nil {
        return err
    }
    if info.Mode()&os.ModeSymlink != 0 {
        return os.Remove(path) // Remove symlink only, never follow
    }
    return os.RemoveAll(path)
}
```

---

## Table of Contents

- [⚠️ MANDATORY RULE: Reference Implementation](#️-mandatory-rule-reference-implementation)
- [A. Problem & Opportunity](#a--problem--opportunity)
- [B. Product Vision](#b--product-vision)
- [C. User Personas](#c--user-personas)
- [D. Competitive Analysis](#d--competitive-analysis)
- [E. Core Features](#e--core-features)
- [F. Architecture](#f-️-architecture)
- [G. UX Specification](#g--ux-specification)
- [H. Security & Safety Model](#h--security--safety-model)
- [I. Technical Specification — Go Engine](#i--technical-specification--go-engine)
- [J. Plugin System (WASM)](#j--plugin-system-wasm)
- [K. SwiftUI App Specification](#k-️-swiftui-app-specification)
- [L. Distribution & Updates](#l--distribution--updates)
- [M. Roadmap & Timeline](#m-️-roadmap--timeline)
- [N. Success Metrics](#n--success-metrics)
- [O. Risk Assessment](#o-️-risk-assessment)
- [P. Testing Strategy](#p--testing-strategy)

---

# A. 🔍 Problem & Opportunity

## Pain Points (macOS users — especially developers)

| # | Pain Point | Severity | Details |
|---|---|:---:|---|
| 1 | Disk fills up rapidly | 🔴 Critical | Xcode DerivedData, Docker images, node_modules caches are extremely heavy |
| 2 | No clarity on what's safe to delete | 🔴 Critical | Users fear accidentally deleting system files |
| 3 | Existing tools are "black box" | 🔴 High | CleanMyMac doesn't show exactly what it's deleting |
| 4 | No dev-focused cleaning tool | 🔴 High | No tool optimized for Docker, npm, brew, Xcode workflows |
| 5 | Manual workflow | 🟡 Medium | Requires manual `rm -rf`, `brew cleanup`, `docker prune` |
| 6 | Subscription fatigue | 🟡 Medium | CleanMyMac subscription model frustrates many users |

## Key Insight

> 🔥 "Cleaner" is not a feature — it's a **decision engine**.
>
> Users don't need a "Clean" button.
> They need answers to:
> - "What is safe to delete?"
> - "How much space will I reclaim?"
> - "What will happen if I delete this?"

## Market Opportunity

- macOS has **no production-quality open-source cleaner** for developers
- CleanMyMac ($39.95/yr subscription) is the market leader but is consumer-focused
- Developer tools (Docker, Node, Xcode) are the #1 source of disk bloat but no tool specializes in them
- Rising "open-source alternative" trend: Pearcleaner, Mole (CLI) gaining traction but lack polish

---

# B. 🎯 Product Vision

## Positioning

> "The open-source, transparent, developer-first disk cleaner for macOS."

**Core pillars:**

| Pillar | What it means |
|---|---|
| **Open Source** | Full transparency, community trust, no hidden behavior |
| **Developer-First** | Deep understanding of Docker, Node, Xcode, Homebrew |
| **Safety-First** | Always preview before delete, trash-based removal, undo support |
| **Extensible** | Plugin system for community-contributed scanners |

## Core Value Proposition

1. **Safe cleaning** — always preview before deletion, trash-based by default
2. **Dev-focused** — understands Docker, Node, Xcode, Homebrew artifacts
3. **Transparent** — shows exactly what will be deleted, file paths and sizes
4. **Extensible** — WASM plugin system for custom scan rules
5. **Free & Open Source** — no subscription, no tracking, no black box

---

# C. 👥 User Personas

### Persona 1: "iOS Developer — Minh"

| Attribute | Detail |
|---|---|
| Role | Senior iOS Developer |
| Pain | Xcode DerivedData fills 30-50GB regularly |
| Behavior | Manually deletes DerivedData weekly, forgets Simulator runtimes |
| Need | One-click safe cleanup of Xcode artifacts |
| Technical level | High — comfortable with Terminal |

### Persona 2: "Full-Stack Developer — Alex"

| Attribute | Detail |
|---|---|
| Role | Full-stack developer (Node.js + Docker) |
| Pain | node_modules across 20+ projects, Docker images piling up |
| Behavior | Occasionally runs `docker system prune`, forgets npm cache |
| Need | Scan all projects, show aggregated reclaimable space |
| Technical level | High — prefers CLI but appreciates good GUI |

### Persona 3: "DevOps/Infra — Sarah"

| Attribute | Detail |
|---|---|
| Role | DevOps engineer |
| Pain | Homebrew cache, old formula versions, log files |
| Behavior | Runs `brew cleanup` monthly, manually deletes logs |
| Need | Automated scheduled cleanup, audit trail |
| Technical level | Expert — wants fine-grained control |

---

# D. 🏆 Competitive Analysis

| Feature | CleanMyMac | DaisyDisk | OnyX | Mole (CLI) | Pearcleaner | **OpenCleaner** |
|---|:---:|:---:|:---:|:---:|:---:|:---:|
| **Price** | $39.95/yr | $9.99 | Free | Free | Free | **Free** |
| **Open Source** | ❌ | ❌ | ❌ | ✅ | ✅ | **✅** |
| **Dev tool cleanup** | Basic | ❌ | ❌ | Basic | ❌ | **Deep** |
| **Docker cleanup** | ❌ | ❌ | ❌ | ❌ | ❌ | **✅** |
| **node_modules cleanup** | ❌ | ❌ | ❌ | ❌ | ❌ | **✅** |
| **Xcode cleanup** | Basic | ❌ | ❌ | ❌ | ❌ | **Deep** |
| **Preview before delete** | ✅ | ✅ (visual) | ❌ | ❌ | ❌ | **✅** |
| **Plugin system** | ❌ | ❌ | ❌ | ❌ | ❌ | **✅** |
| **macOS native UI** | ✅ | ✅ | ✅ | ❌ (CLI) | ✅ | **✅** |
| **Transparency** | Low | High | Medium | High | Medium | **High** |

**Competitive advantage:** OpenCleaner is the only tool combining **deep dev-tool knowledge** + **native macOS UI** + **open source** + **plugin extensibility**.

---

# E. 🧩 Core Features

## Tier 1 — MVP (Phase 1)

### E.1 Scan Engine

| Feature | Detail | Priority |
|---|---|:---:|
| System cache scan | `~/Library/Caches`, system logs, temp files | P0 |
| Developer artifact scan | Xcode DerivedData, Homebrew cache | P0 |
| Categorized results | Group by: System / Developer / Applications | P0 |
| Size calculation | Accurate reclaimable space per item | P0 |
| Concurrent scanning | Goroutine worker pool for parallel I/O | P0 |

### E.2 Safe Cleaning

| Feature | Detail | Priority |
|---|---|:---:|
| Dry-run mode (default) | Preview everything before deletion | P0 |
| Trash-based deletion | Move to `~/.Trash` instead of hard delete | P0 |
| Selective cleaning | Check/uncheck individual items or categories | P0 |
| Double confirmation | Confirm dialog before executing clean | P0 |
| Exclude paths (whitelist) | User-defined paths to never touch | P0 |
| Audit log | Log every delete operation with timestamp & path | P0 |

### E.3 macOS App (SwiftUI)

| Screen | Features | Priority |
|---|---|:---:|
| Dashboard | Disk usage overview, reclaimable space, quick scan CTA | P0 |
| Scan Results | Categorized list, sizes, expand to see file paths | P0 |
| Clean Flow | Select items → preview → confirm → execute | P0 |
| Progress | Progress bar, current file, streaming logs | P0 |

### E.4 CLI Interface

| Feature | Detail | Priority |
|---|---|:---:|
| `opencleaner scan` | Run scan, output structured results | P0 |
| `opencleaner clean` | Execute cleaning with confirmation | P0 |
| `opencleaner scan --json` | Machine-readable output | P0 |
| `opencleaner status` | Show daemon status | P1 |

---

## Tier 2 — Enhanced (Phase 2)

### E.5 Dev Mode (USP — Unique Selling Point)

| Category | Clean Targets |
|---|---|
| Node.js | `node_modules`, npm cache, pnpm store, yarn cache |
| Docker | Unused images, dangling volumes, build cache |
| Xcode | DerivedData, Archives, Simulator runtimes, device support |
| Homebrew | Cache, old formula versions, unused deps |
| Python | `__pycache__`, `.venv`, pip cache |
| Rust | `target/` directories, cargo cache |

### E.6 Smart Suggestions

- "30GB Docker images unused for 30+ days → safe to remove"
- "Xcode DerivedData 14 days old → rebuild takes ~2min"
- Size-weighted prioritization: show biggest wins first

### E.7 Scheduler

- Configurable auto-scan (daily/weekly/monthly)
- Background daemon scan with notification
- Auto-clean with user-defined rules (e.g., "auto-clean DerivedData older than 7 days")

---

## Tier 3 — Platform (Phase 3)

### E.8 WASM Plugin System

```go
type CleanerPlugin interface {
    ID() string
    Name() string
    Scan() ([]ScanItem, error)
    Clean(items []ScanItem) error
}
```

- Community-contributed scanners (e.g., JetBrains IDE cache, Unity cache)
- Plugin directory: `~/.opencleaner/plugins/`
- Sandboxed execution via WASM (wazero runtime)

### E.9 AI Suggestions (Experimental)

- Pattern detection for safe-to-delete heuristics
- Learning from user behavior (local-only, no telemetry)

---

# F. 🏗️ Architecture

## Architecture Decisions (Locked)

| Decision | Choice | Rationale |
|---|---|---|
| Engine language | **Go** | Performance, concurrency, single binary distribution |
| UI framework | **SwiftUI** | Native macOS experience, modern API |
| IPC mechanism | **Unix domain socket + HTTP** | Low overhead, localhost-only, no network exposure |
| Plugin runtime | **WASM (wazero)** | Sandbox safety, cross-language, no CGO needed |
| Daemon | **YES — from day 1** | Proper separation of concerns, enables CLI + GUI + scheduler from start |
| Distribution | **Direct download** (no App Store) | Full Disk Access requirement incompatible with sandbox |
| Update mechanism | **Sparkle** (app), **Homebrew** (CLI) | Industry standard for macOS OSS apps |

## High-Level Architecture (Production — Day 1)

```
┌────────────────────────┐     ┌─────────────────────────┐
│   macOS App (SwiftUI)  │     │   CLI (opencleaner)     │
│   thin UI client       │     │   terminal interface    │
└───────────┬────────────┘     └───────────┬─────────────┘
            │ Unix Socket / HTTP                │
            ▼                                   ▼
┌─────────────────────────────────────────────────────────┐
│               opencleanerd (Go Daemon)                  │
│                                                         │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐  ┌────────┐  │
│  │ Scanner  │  │ Analyzer │  │ Cleaner  │  │Plugins │  │
│  │ Engine   │  │ Engine   │  │ Engine   │  │ (WASM) │  │
│  └──────────┘  └──────────┘  └──────────┘  └────────┘  │
│                                                         │
│  ┌──────────┐  ┌──────────┐  ┌──────────┐              │
│  │Scheduler │  │ Progress │  │  Safety  │              │
│  │          │  │ Streamer │  │  Guard   │              │
│  └──────────┘  └──────────┘  └──────────┘              │
└─────────────────────────────────────────────────────────┘
         Managed by: ~/Library/LaunchAgents/
              com.opencleaner.daemon.plist
```

---

# G. 🎨 UX Specification

## UX Principles

| Principle | Implementation |
|---|---|
| **Safety First** | Always preview, never auto-delete, trash-based removal |
| **Transparency** | Show file paths, sizes, and category for every item |
| **Speed** | Partial results in < 3s, full scan < 10s |
| **Developer-first** | Terminal-friendly, keyboard shortcuts, advanced options |
| **Minimal friction** | One-click scan, smart defaults, remember preferences |

## App Type

- **Menu Bar App** with optional main window
- Click menu bar icon → quick summary dropdown
- "Open Full View" → detailed scan results window

## User Flow — First Launch

```
App Launch
    │
    ▼
┌─────────────────────────┐
│   Welcome Screen        │
│   "OpenCleaner needs    │
│    Full Disk Access"    │
│                         │
│   [Why?] [Grant Access] │
└────────────┬────────────┘
             │ Opens System Settings
             ▼
┌─────────────────────────┐
│   Waiting for Permission│
│   (auto-detect when     │
│    granted)             │
└────────────┬────────────┘
             │
             ▼
┌─────────────────────────┐
│   Dashboard             │
│   Auto-starts first     │
│   scan                  │
└─────────────────────────┘
```

## User Flow — Scan & Clean

```
Dashboard
    │ [Scan Now]
    ▼
┌─────────────────────────┐
│   Scanning...           │
│   ▓▓▓▓▓▓▓░░░ 65%       │
│   Scanning: ~/Library/  │
│   Found: 12.4 GB        │
└────────────┬────────────┘
             │
             ▼
┌─────────────────────────────────────────┐
│   Scan Results                          │
│                                         │
│   ☑ System Cache        4.2 GB    ▸    │
│   ☑ Xcode DerivedData  11.3 GB    ▸    │
│   ☑ Docker Images        8.1 GB    ▸    │
│   ☐ Homebrew Cache       1.5 GB    ▸    │
│   ☐ Node Modules         6.8 GB    ▸    │
│                                         │
│   Total Selected: 23.6 GB               │
│                                         │
│   [Preview & Clean]                     │
└────────────┬────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────┐
│   Preview Delete                        │
│                                         │
│   ⚠ 847 items will be moved to Trash   │
│                                         │
│   ~/Library/Caches/com.apple.dt...      │
│   ~/Library/Developer/Xcode/Der...      │
│   ...                                   │
│                                         │
│   [Cancel]  [Move to Trash (23.6 GB)]   │
└────────────┬────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────┐
│   Cleaning...                           │
│   ▓▓▓▓▓▓▓▓░░ 78%                       │
│   Moving: ~/Library/Developer/...       │
│                                         │
│   Cleaned: 18.4 GB / 23.6 GB           │
└────────────┬────────────────────────────┘
             │
             ▼
┌─────────────────────────────────────────┐
│   ✅ Complete!                           │
│                                         │
│   Reclaimed: 23.6 GB                   │
│   Items: 847 moved to Trash            │
│                                         │
│   [View Log]  [Undo]  [Done]           │
└─────────────────────────────────────────┘
```

## Error States

| State | UX Response |
|---|---|
| No Full Disk Access | Show permission guide with deep-link to System Settings |
| Scan interrupted | Save partial results, offer to resume |
| Delete fails (permission) | Show which files failed, skip and continue |
| Disk full during operation | Warn and pause, suggest freeing space first |
| Daemon not running (Phase 2+) | Offer to start daemon, show connection status |

## Keyboard Shortcuts

| Shortcut | Action |
|---|---|
| `⌘ + S` | Start scan |
| `⌘ + ⌫` | Clean selected items |
| `⌘ + Z` | Undo last clean |
| `⌘ + ,` | Open preferences |
| `Space` | Toggle item selection |
| `⌘ + A` | Select all |

---

# H. 🔒 Security & Safety Model

## Core Safety Rules

### Never-Touch List (Hardcoded)

The following paths are **never scanned or deleted**, regardless of any configuration:

```
/System/
/usr/
/bin/
/sbin/
/private/var/db/
~/Documents/
~/Desktop/
~/Pictures/
~/Music/
~/Movies/
~/Downloads/
~/.ssh/
~/.gnupg/
~/.gitconfig
~/.zshrc
~/.bashrc
~/.config/ (selective — only known cache subdirs)
```

### Deletion Strategy

| Strategy | Detail |
|---|---|
| **Trash-based (default)** | Items moved to `~/.Trash`, user can restore via Finder |
| **Hard delete (opt-in)** | Only available in CLI with `--force` flag |
| **Audit log** | Every operation logged to `~/.opencleaner/logs/audit.log` |
| **Undo support** | In-app undo button for last clean operation |

### Permission Model

| Permission | When Needed | How to Request |
|---|---|---|
| Full Disk Access | Required for system cache scanning | First-launch guided setup with deep-link |
| Accessibility | NOT required | — |
| Network | NOT required (local-only daemon) | — |

### Safety Implementation Reference

> ⚠️ The safety guard implementation MUST be ported from `clmm-clean-my-mac-cli`.
> See [Mandatory Rule: Reference Implementation](#️-mandatory-rule-reference-implementation) for details.

Key functions to port to Go (`internal/safety/guard.go`):

| clmm Function | Go Equivalent | Purpose |
|---|---|---|
| `isProtectedPath()` | `IsProtectedPath(path string) bool` | Check against protected system paths |
| `validatePathSafety()` | `ValidatePathSafety(path string) error` | Full validation including root/home checks |
| `hasTraversalPattern()` | `HasTraversalPattern(path string) bool` | Detect `../` attacks |
| `expandPath()` | `ExpandPath(path string) (string, error)` | Expand `~` with traversal detection |
| `removeItem()` with TOCTOU | `SafeRemove(path string) error` | Re-stat before delete, handle symlinks |

### Plugin Sandbox (Phase 2+)

- WASM plugins run in sandboxed wazero runtime
- Memory limit: 256MB per plugin
- Execution timeout: 30s per scan, 60s per clean
- Restricted filesystem access: read-only for scan, write limited to approved paths
- No network access
- **Plugins MUST use the same safety guard** — all plugin deletions pass through `SafeRemove()`

---

# I. 🧠 Technical Specification — Go Engine

## Project Structure

```
opencleaner/
├── cmd/
│   ├── opencleaner/          # CLI binary
│   │   └── main.go
│   └── opencleanerd/         # Daemon binary (Phase 2+)
│       └── main.go
├── internal/
│   ├── engine/               # Orchestrator: Scan → Analyze → Clean
│   │   └── engine.go
│   ├── scanner/              # File system scanner
│   │   ├── scanner.go
│   │   ├── system.go         # System cache rules
│   │   ├── developer.go      # Dev tool rules
│   │   └── apps.go           # Application cache rules
│   ├── analyzer/             # Size calculation, categorization
│   │   └── analyzer.go
│   ├── cleaner/              # Deletion engine (trash-based)
│   │   ├── cleaner.go
│   │   └── trash.go
│   ├── rules/                # Scan rule definitions
│   │   ├── rules.go
│   │   └── builtin.go
│   ├── safety/               # Never-touch guard, validation
│   │   └── guard.go
│   ├── plugins/              # WASM plugin loader (Phase 3)
│   │   └── loader.go
│   ├── transport/            # HTTP/Unix socket server (Phase 2+)
│   │   └── server.go
│   └── stream/               # Progress streaming (SSE)
│       └── progress.go
├── pkg/
│   ├── types/                # Shared types (ScanItem, ScanResult)
│   │   └── types.go
│   └── logger/               # Structured logging (zap)
│       └── logger.go
├── plugins/                  # Built-in plugin examples
├── api/                      # API schema definitions
│   └── v1/
└── go.mod
```

## Core Types

```go
package types

type ScanItem struct {
    ID          string   `json:"id"`
    Name        string   `json:"name"`
    Path        string   `json:"path"`
    Size        int64    `json:"size"`
    Category    Category `json:"category"`
    SafetyLevel Safety   `json:"safety_level"`
    Description string   `json:"description"`
    LastAccess  time.Time `json:"last_access"`
}

type Category string

const (
    CategorySystem    Category = "system"
    CategoryDeveloper Category = "developer"
    CategoryApps      Category = "apps"
)

type Safety string

const (
    SafetySafe    Safety = "safe"       // Always safe to delete
    SafetyCaution Safety = "caution"    // May need rebuild (e.g., DerivedData)
    SafetyRisky   Safety = "risky"      // Could affect running services
)

type ScanResult struct {
    TotalSize      int64      `json:"total_size"`
    Items          []ScanItem `json:"items"`
    ScanDuration   time.Duration `json:"scan_duration"`
    CategorizedSize map[Category]int64 `json:"categorized_size"`
}

type CleanResult struct {
    CleanedSize  int64    `json:"cleaned_size"`
    CleanedCount int      `json:"cleaned_count"`
    FailedItems  []string `json:"failed_items"`
    AuditLogPath string   `json:"audit_log_path"`
}
```

## Engine Pipeline

```
Scan (concurrent goroutines)
    │
    ▼
Analyze (categorize + calculate sizes)
    │
    ▼
Safety Guard (filter against never-touch list)
    │
    ▼
Group & Sort (by category, then by size descending)
    │
    ▼
Output (JSON to stdout / HTTP response)
```

## Concurrency Model

```go
func (e *Engine) Scan(ctx context.Context) (<-chan ScanItem, error) {
    results := make(chan ScanItem, 100)
    jobs := make(chan ScanJob, 50)

    // Worker pool
    var wg sync.WaitGroup
    for i := 0; i < runtime.NumCPU(); i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            for job := range jobs {
                items, err := job.Execute(ctx)
                if err != nil {
                    continue
                }
                for _, item := range items {
                    results <- item
                }
            }
        }()
    }

    // Feed jobs
    go func() {
        for _, rule := range e.rules {
            jobs <- ScanJob{Rule: rule}
        }
        close(jobs)
        wg.Wait()
        close(results)
    }()

    return results, nil
}
```

## API Contract (Daemon — v1)

### Scan

```
POST /api/v1/scan
Content-Type: application/json

Response: ScanResult (JSON)
```

### Clean

```
POST /api/v1/clean
Content-Type: application/json

Request Body:
{
    "item_ids": ["id1", "id2"],
    "strategy": "trash" | "delete"
}

Response: CleanResult (JSON)
```

### Progress Stream (SSE)

```
GET /api/v1/progress/stream
Accept: text/event-stream

Events:
  data: {"type": "scanning", "current": "/path/...", "progress": 0.65}
  data: {"type": "cleaning", "current": "/path/...", "progress": 0.78}
  data: {"type": "complete", "result": {...}}
```

## Logging

- **Library:** `go.uber.org/zap` (structured JSON logging)
- **Log location:** `~/.opencleaner/logs/`
- **Audit log:** Append-only, records every delete with timestamp, path, size
- **Rotation:** 10MB per file, keep 5 files

---

# J. 🧩 Plugin System (WASM)

> Built-in scan rules ship from day 1. WASM runtime enables community plugins from Phase 2.

## Runtime

- **wazero** — pure Go WASM runtime, no CGO, production-ready since 1.0 (2023)
- JIT compilation for ARM64 and AMD64

## Plugin Directory

```
~/.opencleaner/plugins/
├── docker-cleaner/
│   ├── manifest.json
│   └── plugin.wasm
├── node-cleaner/
│   ├── manifest.json
│   └── plugin.wasm
```

## Manifest Schema

```json
{
    "id": "docker-cleaner",
    "name": "Docker Cleaner",
    "version": "1.0.0",
    "author": "community",
    "description": "Scans and cleans Docker artifacts",
    "entry": "plugin.wasm",
    "permissions": {
        "read_paths": ["/var/run/docker.sock", "~/.docker/"],
        "write_paths": []
    }
}
```

## WASM ABI

```
export function plugin_id(): string
export function plugin_scan(): string    // Returns JSON array of ScanItem
export function plugin_clean(input: string): string  // Accepts JSON array of item IDs
```

## Sandbox Constraints

| Constraint | Limit |
|---|---|
| Memory | 256 MB |
| Scan timeout | 30 seconds |
| Clean timeout | 60 seconds |
| Filesystem | Read-only (declared paths only) |
| Network | Blocked |

---

# K. 🖥️ SwiftUI App Specification

## Project Structure

```
OpenCleaner/
├── App/
│   ├── OpenCleanerApp.swift      # @main entry point
│   └── AppDelegate.swift         # Menu bar setup, lifecycle
├── Core/
│   ├── Engine/
│   │   ├── EngineClient.swift    # Communicates with Go binary
│   │   └── EngineProcess.swift   # Process management
│   ├── Models/
│   │   ├── ScanItem.swift
│   │   ├── ScanResult.swift
│   │   └── CleanResult.swift
│   └── Services/
│       ├── PermissionService.swift   # Full Disk Access check
│       ├── PreferenceService.swift   # UserDefaults wrapper
│       └── AuditLogService.swift     # Reading audit logs
├── Features/
│   ├── Onboarding/
│   │   └── OnboardingView.swift      # Permission setup
│   ├── Dashboard/
│   │   ├── DashboardView.swift
│   │   └── DashboardViewModel.swift
│   ├── ScanResults/
│   │   ├── ScanResultsView.swift
│   │   ├── ScanResultsViewModel.swift
│   │   ├── CategoryRowView.swift
│   │   └── FileDetailView.swift
│   ├── CleanProgress/
│   │   ├── CleanProgressView.swift
│   │   └── CleanProgressViewModel.swift
│   ├── Settings/
│   │   ├── SettingsView.swift
│   │   └── WhitelistEditor.swift
│   └── MenuBar/
│       ├── MenuBarView.swift
│       └── QuickStatusView.swift
├── Design/
│   ├── Theme.swift               # Colors, typography, spacing
│   └── Components/               # Reusable UI components
└── Resources/
    └── Assets.xcassets
```

## Daemon Communication

```swift
/// Communicates with opencleanerd via Unix domain socket / HTTP
actor DaemonClient {
    private let socketPath = "/tmp/opencleaner.sock"
    private let baseURL = "http+unix:///tmp/opencleaner.sock"
    private let session: URLSession

    init() {
        let config = URLSessionConfiguration.default
        config.httpAdditionalHeaders = ["Content-Type": "application/json"]
        self.session = URLSession(configuration: config)
    }

    /// Trigger a full system scan
    func scan() async throws -> ScanResult {
        let (data, _) = try await request(.post, path: "/api/v1/scan")
        return try JSONDecoder().decode(ScanResult.self, from: data)
    }

    /// Execute cleaning on selected items
    func clean(itemIDs: [String], strategy: CleanStrategy = .trash) async throws -> CleanResult {
        let body = CleanRequest(itemIDs: itemIDs, strategy: strategy)
        let (data, _) = try await request(.post, path: "/api/v1/clean", body: body)
        return try JSONDecoder().decode(CleanResult.self, from: data)
    }

    /// Subscribe to real-time progress updates via SSE
    func progressStream() -> AsyncStream<ProgressEvent> {
        AsyncStream { continuation in
            let task = Task {
                let (bytes, _) = try await session.bytes(for: urlRequest(.get, path: "/api/v1/progress/stream"))
                for try await line in bytes.lines {
                    guard line.hasPrefix("data: ") else { continue }
                    let json = Data(line.dropFirst(6).utf8)
                    if let event = try? JSONDecoder().decode(ProgressEvent.self, from: json) {
                        continuation.yield(event)
                    }
                }
                continuation.finish()
            }
            continuation.onTermination = { _ in task.cancel() }
        }
    }

    /// Check daemon health
    func status() async throws -> DaemonStatus {
        let (data, _) = try await request(.get, path: "/api/v1/status")
        return try JSONDecoder().decode(DaemonStatus.self, from: data)
    }
}
```

## Key ViewModels

```swift
@Observable
class DashboardViewModel {
    var diskUsage: DiskUsage?
    var lastScanResult: ScanResult?
    var isScanning: Bool = false
    var scanProgress: Double = 0

    func startScan() async { ... }
}

@Observable
class ScanResultsViewModel {
    var categories: [CategoryGroup] = []
    var selectedItems: Set<String> = []
    var totalSelectedSize: Int64 = 0

    func toggleItem(_ id: String) { ... }
    func selectAll() { ... }
    func deselectAll() { ... }
}
```

## Permission Check

```swift
struct PermissionService {
    /// Check if Full Disk Access is granted by attempting to read a protected path
    static func hasFullDiskAccess() -> Bool {
        let testPath = "\(NSHomeDirectory())/Library/Mail"
        return FileManager.default.isReadableFile(atPath: testPath)
    }

    /// Deep-link to System Settings > Privacy > Full Disk Access
    static func openFullDiskAccessSettings() {
        let url = URL(string: "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles")!
        NSWorkspace.shared.open(url)
    }
}
```

---

# L. 📦 Distribution & Updates

## CLI

```bash
# Install via Homebrew
brew tap opencleaner/tap
brew install opencleaner

# Update
brew upgrade opencleaner
```

## macOS App

| Aspect | Detail |
|---|---|
| Format | `.dmg` with drag-to-Applications |
| Download | GitHub Releases + website |
| Code signing | Apple Developer ID certificate |
| Notarization | Required for Gatekeeper |
| Auto-update | Sparkle framework |
| Update feed | `https://opencleaner.dev/appcast.xml` |

### Update Flow

```
GitHub Release (tag) → GitHub Actions builds .dmg
    → Upload to GitHub Releases
    → Update Sparkle appcast.xml
    → App checks for updates on launch (configurable)
    → User prompted to install update
```

---

# M. 🗺️ Roadmap & Timeline

> **Team:** Full development department with parallel workstreams.
> All phases run multiple tracks concurrently to maximize throughput.

---

## Phase 1 — Full Product Launch (4 weeks)

> **Goal:** Production-ready macOS app with daemon, CLI, dev-mode cleaning, and polished UX.
> **All 5 workstreams run in parallel from day 1.**

### Workstream A — Core Engine (Go) · 4 weeks

| Week | Deliverables |
|---|---|
| W1 | Go project scaffolding, core types, scanner interface, worker pool concurrency model |
| W1 | System cache scanner (~/Library/Caches, logs, temp files) |
| W2 | Analyzer engine: categorization, size calculation, safety levels |
| W2 | Safety guard: never-touch list, path validation, audit logger |
| W3 | Cleaner engine: trash-based deletion, hard-delete (opt-in), undo support |
| W3 | Scan rules: Xcode DerivedData, Homebrew cache, system logs |
| W4 | Unit tests (scanner rules, safety guard, cleaner), performance benchmarks |
| W4 | Integration tests: full scan → analyze → clean pipeline |

### Workstream B — Daemon & Transport · 4 weeks

| Week | Deliverables |
|---|---|
| W1 | Daemon scaffolding (`opencleanerd`), Unix domain socket server |
| W1 | HTTP API: `POST /api/v1/scan`, `POST /api/v1/clean` |
| W2 | SSE progress streaming (`GET /api/v1/progress/stream`) |
| W2 | Daemon lifecycle: launchd plist, auto-start, crash recovery |
| W3 | Plugin loading infrastructure (WASM runtime via wazero) |
| W3 | Plugin sandbox: memory limits, timeouts, restricted FS |
| W4 | CLI interface: `opencleaner scan`, `opencleaner clean`, `opencleaner status` |
| W4 | Daemon ↔ CLI integration tests, API contract validation |

### Workstream C — Dev Mode Scanners · 3 weeks (start W2)

| Week | Deliverables |
|---|---|
| W2 | Docker scanner: unused images, dangling volumes, build cache |
| W2 | Node.js scanner: node_modules, npm/pnpm/yarn cache |
| W3 | Xcode deep scanner: Archives, Simulator runtimes, device support files |
| W3 | Homebrew scanner: formula cache, old versions, unused deps |
| W4 | Python scanner: `__pycache__`, `.venv`, pip cache |
| W4 | Rust scanner: `target/` directories, cargo cache |

### Workstream D — macOS App (SwiftUI) · 4 weeks

| Week | Deliverables |
|---|---|
| W1 | SwiftUI project setup, design system (Theme, Colors, Typography) |
| W1 | Onboarding flow: Full Disk Access permission guide with deep-link |
| W2 | Dashboard view: disk usage overview, reclaimable space, scan CTA |
| W2 | Daemon communication layer (Unix socket client, SSE listener) |
| W3 | Scan results view: categorized list, expand/collapse, file detail |
| W3 | Clean flow: item selection → preview → confirm → execute → progress |
| W4 | Menu bar integration: quick status dropdown, "Open Full View" |
| W4 | Settings: whitelist editor, scan preferences, scheduler config |
| W4 | Completion screen with undo, audit log viewer |

### Workstream E — DevOps & Distribution · 2 weeks (start W3)

| Week | Deliverables |
|---|---|
| W3 | GitHub Actions CI/CD pipeline (build, test, lint) |
| W3 | Code signing + notarization workflow |
| W4 | DMG packaging with drag-to-Applications installer |
| W4 | Sparkle auto-update integration + appcast.xml feed |
| W4 | Homebrew tap: `brew install opencleaner` |
| W4 | Beta release candidate |

### Phase 1 — Exit Criteria

- [ ] Daemon runs as background service (launchd managed)
- [ ] macOS app connects to daemon, displays scan results, executes clean
- [ ] CLI fully operational: scan, clean, status
- [ ] All 6 dev-tool scanners functional (Docker, Node, Xcode, Homebrew, Python, Rust)
- [ ] Trash-based deletion with undo support
- [ ] Audit logging for all operations
- [ ] Safety guard: 0 false positives on never-touch list
- [ ] Full Disk Access onboarding flow working
- [ ] DMG installer with code signing + notarization
- [ ] CI/CD pipeline green
- [ ] Beta deployed to internal team

---

## Phase 2 — Intelligence & Polish (3 weeks)

> **Goal:** Smart suggestions, scheduler, enhanced UX, public beta.

### Workstream A — Smart Engine · 3 weeks

- [ ] Smart suggestions engine: size-weighted prioritization
- [ ] Age-based recommendations ("unused for 30+ days")
- [ ] Rebuild-time estimates ("DerivedData rebuild takes ~2min")
- [ ] Scheduler: configurable auto-scan (daily/weekly/monthly)
- [ ] Scheduled auto-clean with user-defined rules
- [ ] launchd scheduled task integration

### Workstream B — Enhanced UI · 3 weeks

- [ ] Advanced filters: by category, size range, age
- [ ] Search across scan results
- [ ] Sorting: by size, name, date, safety level
- [ ] Scan history: view past scans and results
- [ ] Notification center integration (scan complete, space reclaimed)
- [ ] Dark mode polish, animations, micro-interactions
- [ ] Accessibility: VoiceOver, keyboard navigation

### Workstream C — Plugin Ecosystem · 3 weeks

- [ ] WASM plugin manifest schema finalization
- [ ] Plugin discovery: load from `~/.opencleaner/plugins/`
- [ ] Plugin management UI: install, enable/disable, remove
- [ ] Plugin SDK documentation + starter template
- [ ] Example community plugins: JetBrains cache, Unity cache, CocoaPods

### Workstream D — Quality & Docs · 3 weeks

- [ ] End-to-end test suite
- [ ] Performance testing: 100K files < 10s, 1M files < 30s
- [ ] Security audit: permission escalation, path traversal
- [ ] User documentation site
- [ ] Contributing guide + plugin development guide
- [ ] Public beta release

### Phase 2 — Exit Criteria

- [ ] Smart suggestions working with size + age signals
- [ ] Scheduler running automated scans
- [ ] Plugin system loading and executing WASM plugins
- [ ] 3+ example plugins published
- [ ] Documentation site live
- [ ] Public beta released

---

## Phase 3 — Growth & Platform (4 weeks)

> **Goal:** Community growth, AI layer, v1.0 public launch.

### Workstream A — AI & Analytics · 4 weeks

- [ ] AI suggestion layer: pattern detection for safe-to-delete
- [ ] Local-only behavioral learning (no telemetry)
- [ ] Disk usage trends and forecasting
- [ ] "Disk Health Score" dashboard widget

### Workstream B — Community & Growth · 4 weeks

- [ ] Plugin marketplace: GitHub-based discovery
- [ ] Plugin submission and review process
- [ ] Plugin auto-update mechanism
- [ ] Landing page + marketing site
- [ ] Hacker News / Reddit launch campaign
- [ ] Comparison content: OpenCleaner vs CleanMyMac vs alternatives

### Workstream C — Enterprise & Advanced · 4 weeks

- [ ] MDM-compatible deployment (PPPC profile for Full Disk Access)
- [ ] Team configuration profiles (shared scan rules)
- [ ] CI/CD integration: `opencleaner scan --ci` for disk usage reporting
- [ ] Custom rule authoring (YAML-based, no WASM needed)

### Phase 3 — Exit Criteria

- [ ] AI suggestions improving scan accuracy
- [ ] 5+ community plugins in marketplace
- [ ] 1,000+ GitHub stars
- [ ] v1.0 public release
- [ ] Landing page + docs live

---

# N. 📈 Success Metrics

## Phase 1 (MVP)

| Metric | Target |
|---|---|
| Scan speed (full) | < 10 seconds |
| Scan speed (partial results) | < 3 seconds |
| Accuracy (no false positives) | 100% — never delete safe files |
| App crash rate | < 1% of sessions |
| Disk space reclaimed (avg user) | > 5 GB per scan |

## Phase 2 (Growth)

| Metric | Target |
|---|---|
| GitHub stars | 500+ in first 3 months |
| Weekly active users | 200+ |
| Dev tool space reclaimed (avg) | > 15 GB per scan |
| User retention (30-day) | > 40% |

## Phase 3 (Platform)

| Metric | Target |
|---|---|
| Community plugins | 5+ third-party plugins |
| GitHub stars | 2,000+ |
| Weekly active users | 1,000+ |

---

# O. ⚠️ Risk Assessment

| Risk | Severity | Probability | Mitigation |
|---|:---:|:---:|---|
| **Data loss from incorrect deletion** | 🔴 Critical | Low | Trash-based deletion, never-touch list, audit log, thorough testing |
| **macOS permission rejection** | 🔴 High | Medium | Clear onboarding UX, deep-link to System Settings, graceful degradation |
| **Go binary size too large** | 🟡 Medium | Low | Use `go build -ldflags="-s -w"`, UPX compression |
| **SwiftUI ↔ Go communication latency** | 🟡 Medium | Low | Use streaming JSON, buffer results |
| **WASM plugin performance** | 🟡 Medium | Low | wazero JIT compiler, pre-compile modules |
| **macOS version incompatibility** | 🟡 Medium | Medium | Target macOS 14+ (Sonoma), CI testing on multiple versions |
| **CleanMyMac legal concerns** | 🟡 Medium | Low | No trademark infringement, clear "inspired by" positioning |
| **Low adoption** | 🟡 Medium | Medium | Strong dev community outreach, Hacker News launch, comparisons content |
| **Scope creep** | 🟡 Medium | High | Strict phase gating, PRD adherence |

---

# P. 🧪 Testing Strategy

## Unit Tests

| Component | What to Test |
|---|---|
| Scanner rules | Each rule correctly identifies target files |
| Safety guard | Never-touch list enforcement — **critical path** |
| Analyzer | Size calculation accuracy |
| Cleaner | Trash-based move works correctly |
| Audit logger | Log entries are complete and append-only |

## Integration Tests

| Test | Method |
|---|---|
| Full scan → clean pipeline | Create temp directory structure, scan, clean, verify |
| CLI interface | Execute CLI commands, verify JSON output |
| SwiftUI ↔ Go binary | Verify communication protocol |

## Safety Tests (Critical)

```
Given: a directory containing system files + safe-to-delete caches
When: scan and clean are executed
Then: ONLY cache files are deleted, system files are untouched

Given: a file in the never-touch list
When: a scan rule matches it
Then: it is filtered out by the safety guard

Given: clean operation with trash strategy
When: user clicks undo
Then: all items are restored from trash
```

## Performance Tests

| Test | Target |
|---|---|
| Scan 100,000 files | < 10 seconds |
| Scan 1,000,000 files | < 30 seconds |
| Clean 10,000 items | < 5 seconds |

## Manual QA Checklist

- [ ] Fresh install flow (no prior configuration)
- [ ] Full Disk Access grant/deny scenarios
- [ ] Scan with empty caches
- [ ] Scan with very large caches (>50GB)
- [ ] Clean → Undo → Verify restoration
- [ ] Menu bar interaction
- [ ] Dark mode appearance
- [ ] macOS Sonoma + Sequoia compatibility

---

# 🧾 Summary

**OpenCleaner** = Open-source, developer-first macOS disk cleaning platform.

| Layer | Technology |
|---|---|
| Core Engine | Go (scan, analyze, clean) |
| Plugin Runtime | WASM via wazero (Phase 3) |
| macOS App | SwiftUI (native) |
| Distribution | DMG + Sparkle, Homebrew (CLI) |

**Phase 1 focus:** Safe, transparent cleaning of system caches and basic dev artifacts with a polished native macOS experience.
