# Research: clmm-clean-my-mac-cli reference implementation

Repo: https://github.com/0xAstroAlpha/clmm-clean-my-mac-cli

## Must-port patterns
- Central safety + deletion module:
  - `PROTECTED_PATHS` denylist + `ALLOWED_PATHS` allow-overrides
  - `ValidatePathSafety()` hard-stops: `/`, `$HOME`, protected prefixes
  - TOCTOU mitigation: `lstat` immediately before delete; if symlink → unlink only
  - Dry-run supported everywhere
- Path utilities:
  - `ExpandPath(~...)` with traversal detection
  - `HasTraversalPattern()` precheck
- Scanner framework:
  - Base scanner returns candidates; only central remover deletes
  - Registry + bounded concurrency scan runner
- Risk gating:
  - Category `safetyLevel: safe|moderate|risky`
  - Risky excluded unless explicit flag (ex: `--unsafe`)

## Pitfalls to avoid
- Scanning targets that the deletion guard will refuse (produce “uncleanable” results). Fix: filter/annotate at scan time.
- Do NOT port shell-based purge logic (`rm -rf`, `find`, `du` via exec): injection + bypass risk.
- Enforce absolute paths at delete boundary; reject empty string; expand `~` before validate.

## Safety tests to mirror (Go)
- Protected path refusal (`/`, `$HOME`, `/System/*`, `/usr/*`, `/bin/*`, `/private/var/db/*`, etc)
- Allowed overrides (`/tmp/*`, `/private/var/folders/*`)
- Symlink safety (unlink only)
- TOCTOU regression (swap candidate for symlink; target survives)
- Risk gating behavior
