# Research: macOS menu bar utility UX + Full Disk Access onboarding

## FDA onboarding (works well)
- Ask when needed (first scan that touches protected areas), or first launch if PRD insists.
- Explain:
  - what needs FDA (specific locations)
  - what breaks without it (incomplete results)
  - privacy stance (on-device)
- CTA: “Open System Settings” + optional “Continue with Limited Scan”
- Steps users can follow:
  - System Settings → Privacy & Security → Full Disk Access → enable OpenCleaner
  - If missing: click + and choose /Applications/OpenCleaner.app
  - Often requires quit & reopen; say it explicitly
- Re-check permission on app becoming active (`scenePhase == .active`)

## Menu bar layout conventions
- Status row (permission/daemon) → primary actions (Scan/Clean/Open Full View) → Settings… → Quit
- Use window-style popover if you need progress UI + lists; menu-style for commands only
