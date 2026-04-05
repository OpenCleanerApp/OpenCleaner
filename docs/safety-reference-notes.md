# OpenCleaner — Safety reference notes (MUST FOLLOW)

PRD mandates referencing: https://github.com/0xAstroAlpha/clmm-clean-my-mac-cli

## Port-worthy patterns (do not improvise)
- Protected paths denylist + allowed paths overrides
- Path traversal detection (`../`, odd encodings)
- Expand `~` safely; avoid escaping home unless explicitly allowed
- `ValidatePathSafety` must hard-stop on:
  - `/` (root)
  - `$HOME` (home dir itself)
  - any protected/system prefixes
- Deletion must include TOCTOU mitigation:
  - validate safety
  - `lstat` immediately before delete
  - if symlink: unlink only (never follow)
  - else: delete directory tree
- Dry-run everywhere
- Safety levels per category: `safe | moderate | risky` with explicit notes

## Known pitfalls seen in reference repo (avoid porting blindly)
- Some scanners can target protected paths (ex: `/var/log`) leading to “uncleanable” results.
  - Fix: filter at scan time OR surface as skipped/blocked with reason.
- Avoid shelling out for deletion (`rm -rf`, `find`, `du`): injection + bypass risk.

## MUST-HAVE safety tests
- Refuse deletion of: `/`, `$HOME`, `/System/*`, `/usr/*`, `/bin/*`, `/sbin/*`, `/private/var/db/*`, etc.
- Allow deletion under: `/tmp/*`, `/private/var/folders/*` (explicit allowlist override)
- Symlink safety: deleting link never deletes target
- TOCTOU regression: swap candidate for symlink just before delete; ensure target survives
- Risk gating: risky categories require `--unsafe` (CLI)

## Undo restore safety (Trash → original)
- Undo restores **only** the last trash-based clean (single manifest).
- Restore refuses to run if `$HOME` cannot be resolved.
- Each entry must satisfy:
  - `SrcPath` is within `$HOME` (no restoring outside the user home)
  - `DstPath` is within the computed Trash directory (typically `$HOME/.Trash`)
  - No *existing* ancestor directories in the restore path are symlinks (prevents symlink-escape)
  - `SrcPath` is never overwritten (if it exists, the item is reported as failed)
