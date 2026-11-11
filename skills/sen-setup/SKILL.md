---
name: sen-setup
description: Initialize and verify Sen MVP deterministic requirement ledger and anti-cheat verification referee in the current workspace. Trigger when the user asks to set up, enable, or initialize Sen MVP.
---

# Sen MVP Workspace Setup (`sen-setup`)

Follow these exact steps to initialize **Sen MVP** in the user's current project workspace without requiring manual `.exe` compilation:

1. **Locate CLI Entrypoint:**
   - Check if `sen` is already on `PATH`.
   - Otherwise invoke the zero-build smart wrapper `<sen-mvp-root>/bin/sen` (Windows resolves `bin/sen.cmd`; macOS/Linux resolves `bin/sen`), which automatically fetches and SHA-256-verifies the prebuilt GitHub Release binary (or builds once locally if Go is installed).
2. **Initialize Workspace Ledger:**
   - Run `sen init --with-companions` inside the target project root.
   - Confirm `.sen/state.db`, `.sen/config.json`, and `AGENTS.md` (plus `.agents/hooks.json` when using standalone workspace hooks) are created.
3. **Verify Readiness:**
   - Run `sen status` and `sen brief` to confirm active Companion Skills (`superpowers`, `security-audit`, `vn-law-check`) and clean baseline capture.
   - Optionally offer 1-line PATH installation via `scripts/install-sen.ps1` (Windows) or `scripts/install-sen.sh` (macOS/Linux) so bare `sen` works across all terminals.
