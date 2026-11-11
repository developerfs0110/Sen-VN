# Sen-VN v1.1.0 — Project Context (`docs/CONTEXT.md`)

## Overview
- **Module:** `github.com/developerfs0110/Sen-VN` (`go 1.26.0` minimum, verified on Go `1.27.3` `windows/amd64`, zero CGO).
- **Version:** `1.1.0` (`plugin.json`, `workspace.CurrentVersion`, `packages.lock.json`).
- **Distribution:** Native Antigravity Plugin (`plugin.json`, `hooks.json`, `rules/AGENTS.md`, `skills/sen-setup/`, `skills/sen-dialectical/`, `skills/sen-skill-capture/`, `packages.lock.json`) + Zero-Build Smart Wrappers (`bin/sen.cmd`, `bin/sen`) + Multi-OS GitHub Releases (`checksums.txt` SHA-256 verified).
- **Database:** `.sen/state.db` (`modernc.org/sqlite`, WAL mode, frozen 5 tables: `rules`, `requirements`, `tasks`, `snapshots`, `evidence`) & frozen 8 CLI commands (`init`, `brief`, `intake`, `begin`, `verify`, `undo`, `done`, `status`).

## Package & Directory Map
- `plugin.json`, `hooks.json`, `packages.lock.json`, `rules/AGENTS.md`, `skills/{sen-setup,sen-dialectical,sen-skill-capture}/SKILL.md` — Native Antigravity plugin & companion catalog root layout.
- `bin/sen.cmd`, `bin/sen`, `scripts/bootstrap-sen.ps1` — Zero-Build wrappers (`<2ms` cached fast-path -> GitHub Releases HTTPS SHA-256 download -> local `go build` fallback).
- `scripts/install-sen.ps1`, `scripts/install-sen.sh` — Cross-platform user `PATH` & workspace installers (`-Workspace` / `--workspace`, `-Packs` / `--packs`, `-SkipCompanions` / `--skip-companions`, `-Update` / `--update`).
- `cmd/sen/` — Entrypoint (`main.go`), `.sen/config.sha256` tamper guard, and 8-command handlers (`commands.go`, `commands_exec.go`).
- `internal/redact/` — Child environment scrubbing (`FilterEnv`) and secret redaction (`ScrubText`).
- `internal/storage/` — Frozen 5-table SQLite WAL store (`db.go`, `rules_reqs.go`, `tasks_snapshots.go`, `evidence.go` with consecutive failed-verify queries).
- `internal/workspace/` — 3-tier `SMALL`/`MEDIUM`/`LARGE` sizing & raw media locks (`risk.go`), multi-domain `DomainProfile` & `Quarantine` reset state (`domain.go`), baseline/snapshot management (`snapshot.go`), and `CheckIntegrity` + `UndoTask` (`integrity.go`).
- `internal/verify/` — Binary allowlist & test parsers (`parsers.go`), `ffprobe` stream/codec/duration referee (`ffprobe.go`), `<= 3` bounded repair runner (`runner.go`), and multi-domain `EvaluateDoneGateWithOpts` (`done_gate.go`).
- `internal/bridge/` — Companion discovery & signed config reader (`companions.go`), `AGENTS.md` v1.1 template (`templates.go`), Antigravity 4-event JSON hooks + `--express` + `ffmpeg` raw media guard (`agy_hooks.go`, `agy_media_guard.go`), embedded packs & `developerfs0110` fork fallback installer (`packages.go`, `packages_git.go`, `embedded_packs/`), and `.bak`-rollback updater (`updater.go`).
- `e2e_anticheat_test.go`, `e2e_round1_test.go`, `e2e_plugin_wrapper_test.go`, `e2e_v11_test.go` — Full compiled-binary & zero-build wrapper E2E test suites.
