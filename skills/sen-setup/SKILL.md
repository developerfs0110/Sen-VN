---
name: sen-setup
description: Initialize, configure companion packs, and update Sen-VN v1.1.0 deterministic requirement ledger and anti-cheat verification referee in the current workspace. Trigger when the user asks to set up, enable, install companions/packs, or update Sen-VN.
---

# Sen-VN v1.1.0 Workspace Setup & Companion Installer (`sen-setup`)

Follow these exact steps to initialize **Sen-VN v1.1.0** in the user's current project workspace without requiring manual `.exe` compilation:

1. **Locate CLI Entrypoint:**
   - Check if `sen` is already on `PATH`.
   - Otherwise invoke the zero-build smart wrapper `<sen-vn-root>/bin/sen` (Windows resolves `bin/sen.cmd`; macOS/Linux resolves `bin/sen`), which automatically fetches and SHA-256-verifies the prebuilt GitHub Release binary (or builds once locally if Go is installed).
2. **Initialize Workspace & Auto-Install Default Companion Bundle:**
   - Run `sen init --install-companions [--profile auto|code|web|video|media|docs]` inside the target project root.
   - This creates `.sen/state.db`, signed `.sen/config.json` (`.sen/config.sha256`), `AGENTS.md`, `.agents/hooks.json`, and installs missing default skills (`superpowers`, `security-audit`, `vn-law-check`, `sen-dialectical`, `sen-skill-capture`) with automatic `developerfs0110/...` fork fallback while skipping any already-present skills.
3. **Optional Recommended Packs & Safe Updates:**
   - List optional domain packs via `sen init --packs list` (`workflow-diagram`, `media-video`, `media-image`, `voice-tts`, `media-3d`), and install selected packs via `sen init --packs <pack1,pack2|all>`.
   - Check for updates and atomically sync `.agents/sen.lock.json` packages (with `.bak` rollback) via `sen init --update` (or `scripts/install-sen.ps1 -Update` / `scripts/install-sen.sh --update`).
4. **Verify Readiness:**
   - Run `sen status` and `sen brief` to confirm domain profile, companion status, and clean `__BASELINE__` snapshot capture.
