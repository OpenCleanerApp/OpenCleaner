---
title: "OpenCleaner bootstrap"
description: "Bootstrap the PRD into a runnable Go daemon+CLI and SwiftUI menu bar app with safety-first scan/clean."
status: in_progress
priority: P1
effort: 40h
issue: null
branch: main
tags: [feature, backend, frontend, api, security, macos]
created: 2026-04-04
---

# OpenCleaner — Implementation Plan

## Overview
Build OpenCleaner per `prd.md`: Go engine/daemon/CLI + SwiftUI macOS menu bar app. Safety-first deletion. MUST reference and port guard patterns from `clmm-clean-my-mac-cli`.

## Inputs
- PRD: `./prd.md`
- Existing docs: `./docs/*` (tech stack, safety notes, IPC/SSE notes, wireframes)
- Research: `./plans/260404-2317-opencleaner-bootstrap/research/*`

## Phases

| # | Phase | Status | Effort | Link |
|---|------|--------|--------|------|
| 1 | Repo scaffold + tooling | Done | 6h | [phase-01](./phase-01-repo-scaffold.md) |
| 2 | Safety guard + safety tests | Done | 8h | [phase-02](./phase-02-safety-core.md) |
| 3 | Scan rules + analyzer | Done (MVP) | 6h | [phase-03](./phase-03-scan-analyze.md) |
| 4 | Cleaner (trash) + audit + undo | Done (MVP) | 5h | [phase-04](./phase-04-cleaner.md) |
| 5 | Daemon transport (unix socket HTTP) + SSE | Done (MVP) | 6h | [phase-05](./phase-05-daemon-transport.md) |
| 6 | CLI commands | Done (MVP) | 3h | [phase-06](./phase-06-cli.md) |
| 7 | SwiftUI app (menu bar + main window) | In progress | 6h | [phase-07](./phase-07-swiftui-app.md) |
| 8 | Integration/perf tests + docs polish | Pending | 4h | [phase-08](./phase-08-testing-docs.md) |

## Key decisions (locked by PRD)
- Go >=1.22, SwiftUI macOS 14+
- IPC: HTTP/JSON over Unix domain socket
- Default deletion: move to Trash; hard delete only with explicit CLI `--force`

## Highest risk items
- Correct safety guard + TOCTOU protection (no data loss)
- Swift client for unix-socket HTTP
- Scan targets: ensure scanned candidates are actually cleanable OR clearly labeled as blocked

## Acceptance criteria (MVP)
- Go daemon runs locally; exposes `/api/v1/scan`, `/api/v1/clean`, `/api/v1/status`, SSE progress
- CLI can scan/clean via daemon; supports `--json` and `--dry-run`
- SwiftUI menu bar app:
  - FDA onboarding screen and limited-mode banner
  - scan progress + results + preview & clean + settings
- Safety tests pass; no shell-based deletion

## Unresolved questions (don’t block start)
- Where should the unix socket live (prefer Application Support, 0600 perms)?
- How strict should never-touch be regarding `/Applications` (recommended: never touch app bundles)?
