# Sen MVP v1.0 — Project Context (`docs/CONTEXT.md`)

## Overview
- **Module:** `sen-mvp` (`go 1.26.0` minimum, verified on Go `1.27.3` `windows/amd64`, zero CGO).
- **Distribution:** Native Antigravity Plugin (`plugin.json`, `hooks.json`, `rules/AGENTS.md`, `skills/sen-setup/SKILL.md`) + Zero-Build Smart Wrappers (`bin/sen.cmd`, `bin/sen`) + Multi-OS GitHub Releases (`checksums.txt` SHA-256 verified).
- **Database:** `.sen/state.db` (`modernc.org/sqlite`, WAL mode, exact 5 tables: `rules`, `requirements`, `tasks`, `snapshots`, `evidence`).

## Package & Directory Map
- `plugin.json`, `hooks.json`, `rules/AGENTS.md`, `skills/sen-setup/SKILL.md` — Native Antigravity plugin root layout.
- `bin/sen.cmd`, `bin/sen`, `scripts/bootstrap-sen.ps1` — Zero-Build wrappers (`<2ms` cached fast-path -> GitHub Releases HTTPS SHA-256 download -> local `go build` fallback).
- `scripts/install-sen.ps1`, `scripts/install-sen.sh` — Cross-platform user `PATH` installers enforcing HTTPS + SHA-256 verification.
- `.github/workflows/ci.yml`, `.github/workflows/release.yml` — Matrix CI (`ubuntu-latest`, `windows-latest`, `macos-latest`) and automated 6-binary (`windows`/`darwin`/`linux` x `amd64`/`arm64`) release publisher.
- `cmd/sen/` — Entrypoint (`main.go`), `.sen/config.sha256` integrity verifier, and 8-command handlers (`commands.go`, `commands_exec.go`).
- `internal/redact/` — Child environment scrubbing (`FilterEnv`) and secret redaction (`ScrubText`).
- `internal/storage/` — 5-table SQLite WAL store (`db.go`, `rules_reqs.go`, `tasks_snapshots.go`, `evidence.go`).
- `internal/workspace/` — Task ID & path validation (`risk.go`), baseline/snapshot management with NTFS symlink/junction rejection (`snapshot.go`), and active-scope `CheckIntegrity` + `UndoTask` (`integrity.go`).
- `internal/verify/` — Safe `PATH` binary resolution (`parsers.go`), direct `exec.CommandContext` runner (`runner.go`), and 5-part `EvaluateDoneGate` (`done_gate.go`).
- `internal/bridge/` — `.agents/` Companion Skills discovery (`companions.go`), `AGENTS.md` template (`templates.go`), and Antigravity 4-event JSON hook adapter (`agy_hooks.go` with plugin `cwd` -> user workspace separation).
- `e2e_anticheat_test.go`, `e2e_round1_test.go`, `e2e_plugin_wrapper_test.go` — Full compiled-binary & zero-build wrapper E2E test suites.
