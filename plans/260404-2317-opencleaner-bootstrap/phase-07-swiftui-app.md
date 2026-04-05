# Phase 07 — SwiftUI app (menu bar + main window)

## Goal
Implement SwiftUI menu bar app that drives scan/clean via daemon and matches wireframes.

## Requirements
- App type: Menu bar (with optional main window)
- FDA onboarding screen and re-check on app activation
- Scan progress and results list
- Preview & Clean confirmation sheet
- Settings window (dry-run default? risky gating?)

## Key technical risk
- HTTP-over-unix-socket client:
  - Implement using Network.framework (`NWConnection` to `.unix(path)`) and minimal HTTP request/response handling
  - If this becomes too complex, reevaluate PRD constraint (document decision)

## Files (Swift)
Create per PRD structure under `app/`:
- Core services (PermissionService, Preferences)
- Daemon client + SSE parsing
- Views/ViewModels for onboarding/dashboard/results/clean progress/settings

## Success criteria
- App shows permission state, connects to daemon, can scan + show results
- Clean flow uses Trash by default and shows completion + log link
