# Changelog

All notable changes to **Sen MVP** are documented in this file.

## [1.0.0] - 2026-11-11

### Added
- **Frozen 8-Command Go CLI (`cmd/sen`):** Implemented `init`, `brief`, `intake`, `begin`, `verify`, `undo`, `done`, and `status` using pure-Go `modernc.org/sqlite` (zero CGO, zero external LLM dependencies).
- **5-Table SQLite WAL Ledger (`internal/storage`):** Created `rules` (`<= 20` hard cap with `--replace`/`--delete`), `requirements` (`R1..Rn`, `R_sec`, `R_law`), immutable `tasks`, `snapshots`, and canonical-JSON SHA-256 hash-chained `evidence` (`GENESIS` -> `entry_hash`).
- **Git-Independent Workspace Integrity & Undo (`internal/workspace`):** Added `__BASELINE__` manifest tracking (advancing cleanly on `VERIFIED` tasks), `>10MB` media copy-skip (`size + mtime_ns` tracking), NTFS junction/symlink escape rejection, pre-existing test SHA-256 locking (`is_locked_test = 1`), deterministic `ComputeRisk`, and byte-for-byte `sen undo`.
- **Direct Subprocess Verification & `done` Gate (`internal/verify`):** Enforced safe `PATH` binary resolution (blocking shell interpreters, `%TEMP%`/workspace spoofing, and non-Node `.cmd`/`.bat` wrappers), child environment scrubbing, secret redaction (`[REDACTED]`), empty-test rejection across `go test`, `pytest`, and `jest`/`vitest`, `TEST`-strength enforcement on `BUGFIX` tasks, and 100% SQL requirement coverage in `sen done`.
- **Native Antigravity Plugin & Zero-Build Packaging:** Added `plugin.json`, root `hooks.json` (with plugin `cwd` -> user workspace isolation), `rules/AGENTS.md`, `skills/sen-setup/SKILL.md`, cross-platform zero-build smart wrappers (`bin/sen.cmd`, `bin/sen`, `scripts/bootstrap-sen.ps1`), HTTPS SHA-256 installers (`scripts/install-sen.ps1`, `scripts/install-sen.sh`), and GitHub Actions CI + 6-target Multi-OS Release workflows (`.github/workflows/ci.yml`, `.github/workflows/release.yml`).
