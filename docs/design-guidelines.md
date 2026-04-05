# OpenCleaner — Design Guidelines (macOS 14+ SwiftUI)

Last updated: 2026-04-04  
Scope: macOS menu bar app + optional main window (SwiftUI).  
Goal: Native macOS aesthetics (not web-app UI), safety-first cleaning, transparent previews.

---

## 0. Product UX Principles (from PRD, enforced)
1. **Safety first**: always preview; never auto-delete; trash-based default.
2. **Transparency**: show paths, sizes, and category for every item.
3. **Speed**: partial results fast; stream progress; never “frozen” UI.
4. **Developer-first**: keyboard shortcuts, search, clear audit trail.
5. **Minimal friction**: smart defaults; remember preferences; uncluttered chrome.

---

## 1. App Surfaces & Navigation Model

### 1.1 Menu Bar Popover (primary surface)
- Quick status + primary actions.
- Width: **320–360pt**, height: content-driven (avoid scrolling unless needed).
- Actions: **Scan Now**, **Open Full View**, **Preview & Clean** (only when results exist), **Settings**.
- Always show **daemon connection** and **permission** status.

### 1.2 Main Window (optional “Full View”)
- Default size: **980×640pt**, minimum: **820×560pt**.
- Layout: macOS-standard:
  - **Toolbar** (top) with primary controls.
  - Optional **sidebar** for navigation (Dashboard / Results / Audit Log).
  - Content uses **Lists, Tables, Forms**, and **Sheets** for confirmation.

### 1.3 Sheets (confirmation, preview, destructive actions)
- Use sheets for:
  - “Preview & Clean” confirmation
  - Risk acknowledgements (when risky categories selected)
  - “View details” for failures
- Destructive primary button uses `.destructive` styling and **explicit verb**: “Move to Trash”.

---

## 2. Typography (macOS-native)

### 2.1 Fonts
- Primary: **SF Pro** (system) via SwiftUI defaults.
- Monospace for file paths/logs: **SF Mono** (`.monospaced()` in SwiftUI).

### 2.2 Type Scale (use semantic styles)
Use SwiftUI text styles to inherit Dynamic Type:
- Large titles: `.largeTitle` (sparingly; onboarding only)
- Section titles: `.title2` / `.title3`
- Row titles: `.headline`
- Body: `.body`
- Secondary: `.callout`
- Metadata: `.footnote`, `.caption`

### 2.3 Path & Size Formatting
- Paths: single-line truncation middle where possible (`…/DerivedData/...`).
- Sizes: use consistent binary units (GiB/MiB) and align right in lists.
- Keep numbers tabular where feasible (monospaced digits).

---

## 3. Color System (semantic first; auto light/dark)

### 3.1 Rule
**Never hardcode “light theme” colors** for production UI. Use semantic colors:
- Foreground:
  - `NSColor.labelColor` (primary)
  - `secondaryLabelColor`, `tertiaryLabelColor`
- Background:
  - `windowBackgroundColor`
  - `controlBackgroundColor`
  - `textBackgroundColor`
- Separators/borders:
  - `separatorColor`, `quaternaryLabelColor`

### 3.2 Semantic Tokens (names to implement in `Design/Theme.swift`)
Foreground:
- `fg.primary` = Label
- `fg.secondary` = Secondary label
- `fg.muted` = Tertiary label

Background:
- `bg.window` = Window background
- `bg.group` = Control background (grouped boxes)
- `bg.sidebar` = Under-page sidebar fill (slightly distinct)
- `bg.selection` = Selected content background

Stroke / Divider:
- `stroke.separator` = Separator color

Accents:
- `accent` = System Blue
- `success` = System Green
- `warning` = System Orange/Yellow
- `danger` = System Red

### 3.3 Risk Level Color (badges)
- Safe: neutral/green hint (do not over-signal)
- Moderate: warning
- Risky: danger
Badges must remain legible in Dark Mode (use semantic fills + `Material` where possible).

---

## 4. Spacing, Layout, and Density (macOS rhythm)

### 4.1 Baseline grid
- Use **4pt** base increments.

### 4.2 Spacing scale (tokens)
- `space.1 = 4`
- `space.2 = 8`
- `space.3 = 12`
- `space.4 = 16`
- `space.5 = 20`
- `space.6 = 24`
- `space.7 = 32`

### 4.3 Standard paddings
- Window content padding: 16–20
- Sidebar padding: 12–16
- Group box padding: 12–16
- Row height: 28–32 (comfortable); compact 24–28 (advanced tables)

### 4.4 Corner radius
- Grouped boxes: 10–12
- Badges: 999 (pill)
- Buttons: system default (don’t custom-round)

---

## 5. Components (native patterns)

### 5.1 Category Row (Scan Results)
Row structure:
- Checkbox (selection)
- Category name
- Risk badge (Safe/Moderate/Risky)
- Size (right-aligned)
- Disclosure chevron (opens category detail)
States:
- Hover highlight (subtle)
- Selected checkbox updates total selected size immediately
- Disabled (when scanning / cleaning)

### 5.2 Progress (Scanning / Cleaning)
- Use `ProgressView` with:
  - determinate bar when percentage known
  - indeterminate when streaming discovery
- Always show:
  - current phase (“Scanning”, “Cleaning”)
  - current target path (monospace, truncating)
  - running total found/cleaned

### 5.3 Confirmation (Preview & Clean)
- Summary block:
  - total selected size
  - item count
  - strategy (Trash default)
  - audit log path mention
- List preview: top N items + “and X more…”
- Primary action label:
  - “Move to Trash (23.6 GB)”
- Secondary:
  - “Cancel”
- Optional tertiary:
  - “Show Audit Log” after completion

### 5.4 Empty / First-run States
- Calm, instructional, minimal illustration (SF Symbol ok).
- Never use “marketing hero” layouts.

---

## 6. Status & Error States

### 6.1 Permission missing (Full Disk Access)
- Show clear reason + steps.
- Provide deep-link:
  - `x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles`
- Show live status: “Waiting for permission…” with auto-detect.
- Disable scanning actions until granted.

### 6.2 Daemon offline (Phase 2+)
- Non-blocking banner + dedicated error view with actions:
  - “Retry”
  - “Open Full View” (to show diagnostics)
  - “Copy Diagnostics”
- Always preserve last known scan results (if safe), but disable cleaning if engine not available.

### 6.3 Partial failures during clean
- Complete the operation; show summary:
  - “X items moved to Trash”
  - “Y items failed”
- Provide “View details…” (sheet) and write to audit log.

---

## 7. Interaction, Motion, and Feedback
- Use subtle motion only:
  - 120–200ms ease-out for state transitions
  - progress updates are continuous but not jittery (throttle UI updates)
- Respect `Reduce Motion`:
  - no parallax
  - no fancy transitions
- Use haptics? (not typical on Mac). Prefer soundless, visual feedback.

---

## 8. Accessibility (WCAG + macOS conventions)
- Minimum contrast: aim for **WCAG 2.1 AA** equivalent; rely on semantic colors to pass.
- Keyboard:
  - full navigation of Lists
  - visible focus ring on controls
- Touch target guidance (mouse/trackpad):
  - aim **≥ 24×24pt**, prefer 28–32 for rows.
- Text:
  - avoid truncating critical warnings
  - paths truncate but always copyable via context menu (“Copy Path”)

---

## 9. Keyboard Shortcuts (PRD + additions)

Core (from PRD):
- `⌘ S` — Start scan
- `⌘ ⌫` — Clean selected items
- `⌘ Z` — Undo last clean
- `⌘ ,` — Preferences
- `Space` — Toggle item selection (in results list)
- `⌘ A` — Select all

Recommended additions (macOS-standard):
- `⌘ F` — Search within current list
- `⌘ O` — Open Full View (from menu bar)
- `⌘ W` — Close window
- `⌘ L` — Open Audit Log (if implemented)
- `⌘ .` — Cancel scan/clean (where safe)

---

## 10. Copy & Tone
- Professional, calm, factual.
- Avoid scare tactics; be explicit about what happens:
  - “Moved to Trash” vs “Deleted”
- Use “Reclaimable” for scan totals; “Reclaimed” after cleaning.

---
