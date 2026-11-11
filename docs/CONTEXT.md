# Sen-VN v1.2.0 + SSDS v1.0.0 — Project Context (`docs/CONTEXT.md`)

## Overview
- **Module:** `github.com/developerfs0110/Sen-VN` (`go 1.26.0` minimum, verified on Go `1.27.3` `windows/amd64`, zero CGO).
- **Version:** `1.2.0` (`plugin.json`, `workspace.CurrentVersion`, `packages.lock.json`) bundling **`SEN-STD-SDLC-001` (`SSDS v1.0.0`, `workspace.DefaultSSDSVersion`)**.
- **Distribution:** Native Antigravity Plugin (`plugin.json`, `hooks.json` under unified `"sen-vn"` key, `rules/AGENTS.md`, `skills/sen-setup/`, `skills/sen-dialectical/`, `skills/sen-skill-capture/`, `packages.lock.json`) + Zero-Build Smart Wrappers (`bin/sen.cmd`, `bin/sen`) + Global/Workspace Lifecycle Hook Installers (`scripts/install-sen.ps1 -ConfigureHooks`, `scripts/install-sen.sh --hooks-config`) + Multi-OS GitHub Releases (`checksums.txt` SHA-256 verified).
- **Database & CLI:** `.sen/state.db` (`modernc.org/sqlite`, WAL mode, 8 canonical tables: `evidence`, `requirements`, `rules`, `snapshots`, `ssds_governance`, `ssds_phases`, `ssds_project`, `tasks`) & 9 CLI commands (`init`, `brief`, `intake`, `begin`, `verify`, `undo`, `done`, `status`, `sdlc`).

## Locked Architectural Decisions (`v1.2.0` / `SSDS v1.0.0`)
1. **Zero-Prompt Automatic SDLC Activation:** `PreInvocation` (`internal/bridge/agy_ssds_activation.go`) automatically detects bilingual software engineering prompts, runs Brownfield Repository Audits (§2.3), bootstraps adaptive `WF_MICRO | WF_STANDARD | WF_CRITICAL` workflows into `.sen/state.db` + `PROJECT_STATUS.md`, and emits `[SSDS v1.0.0 — Đã kích hoạt tự động]`.
2. **Umbrella Workspace & Multi-Project Isolation:** `workspace.IsUmbrellaMultiProjectDir`, `workspace.ResolveNearestProjectRoot`, and `workspace.CheckNoSubprojectCrossing` ensure multi-repo parent directories (such as `Workspace/`) are never polluted with a root `.sen/state.db`, while sibling/nested projects maintain isolated requirements, baselines, and phase states (`--project <dir>`).
3. **Physical Pre-Execution Write Firewall & Human Privilege Guard:** `PreToolUse` (`internal/bridge/agy_hooks.go`, `agy_cmd_guard.go`) physically blocks source/manifest file edits during `P0..P3` (`WF_STANDARD`/`WF_CRITICAL`) and returns `force_ask` whenever an agent attempts to self-run `sen sdlc approve`, `sen sdlc exception`, `--approve`, `--express`, or subshell-wrapped variants (`powershell -c`, `cmd /c`, `sh -c`).
4. **Honest Review, Cryptographic CIA & Anti-Fake-PASS:** Enforces `author_role != reviewer_role`, automatic `INDEPENDENT_AI_REVIEW -> SELF_REVIEW` downgrade without external `--ref`, rejection of `SELF_REVIEW` on critical phases, one-shot SHA-256-bound 10-dimension `CIA-*` Change Impact Analysis (§17), 6-link `docs/traceability-matrix.md` validation (§16), and 4-tier task states (`IMPLEMENTED -> EXECUTED -> TESTED -> VERIFIED`, §20).
5. **Independent SSDS Standard Versioning (Part II):** Projects pin `applied_ssds_version` inside SHA-256-signed `.sen/config.json` and `ssds_project`; embedded rulepacks (`internal/ssds/versions/v1.0.0/`) take strict precedence over `SEN_SSDS_EXTRA_VERSIONS_DIR`; upgrades require explicit `sen sdlc version --upgrade <ver> --approve --quote "..."` with instant `--rollback`.

## Package & Directory Map
- `plugin.json`, `hooks.json`, `packages.lock.json`, `rules/AGENTS.md`, `skills/{sen-setup,sen-dialectical,sen-skill-capture}/SKILL.md` — Native Antigravity plugin & companion catalog root layout.
- `bin/sen.cmd`, `bin/sen`, `scripts/bootstrap-sen.ps1` — Zero-Build wrappers (`<2ms` cached fast-path -> GitHub Releases HTTPS SHA-256 download -> local `go build` fallback).
- `scripts/install-sen.ps1`, `scripts/install-sen.sh` — Cross-platform user `PATH`, global/workspace `"sen-vn"` hook mergers, and workspace installers.
- `cmd/sen/` — Entrypoint (`main.go`), `--project <dir>` isolator, `.sen/config.sha256` tamper guard, and 9-command handlers (`commands.go`, `commands_begin.go`, `commands_exec.go`, `commands_sdlc.go`, `commands_sdlc_gov.go`).
- `internal/ssds/` — Embedded `SSDS_CHANGELOG.md` + `versions/v1.0.0/`, version registry & upgrader (`registry.go`, `registry_upgrade.go`), 27 engineering roles (`roles.go`), bilingual software/brownfield detector (`detector.go`, `detector_brownfield.go`), adaptive workflows & phase state machine (`workflows.go`, `state_machine.go`), reviews/approvals/CIA/exceptions (`governance.go`), 6-link traceability (`traceability.go`), deliverable validators (`validator.go`, `validator_phases.go`), and §25 report renderer (`renderer.go`).
- `internal/storage/` — 8-table SQLite WAL store (`db.go`, `rules_reqs.go`, `tasks_snapshots.go`, `evidence.go`, `ssds_store.go`, `ssds_governance.go`) with idempotent `v1.1 -> v1.2` migrations.
- `internal/workspace/` — 3-tier sizing & media locks (`risk.go`), multi-project root resolution & umbrella guard (`isolation.go`), domain/version constants (`domain.go`), baseline/snapshot management (`snapshot.go`), and `CheckIntegrity` + `UndoTask` (`integrity.go`).
- `internal/verify/` — Binary allowlist & test parsers (`parsers.go`), `ffprobe` referee (`ffprobe.go`), §20 execution state & bounded repair runner (`runner.go`), and SSDS-enforced `EvaluateDoneGateWithOpts` (`done_gate.go`).
- `internal/bridge/` — Companion discovery & signed config reader (`companions.go`), `AGENTS.md` v1.2 template (`templates.go`), zero-prompt SSDS hook activator (`agy_ssds_activation.go`), privilege/env command guard (`agy_cmd_guard.go`), raw media guard (`agy_media_guard.go`), 4-event Antigravity JSON hooks (`agy_hooks.go`), embedded packs & fork fallback (`packages.go`, `packages_git.go`), and `.bak` updater (`updater.go`).
- `e2e_*.go`, `ssds_compliance_test.go`, `ssds_isolation_version_test.go`, `ssds_adversarial_hardening_test.go` — Full E2E, Suites 1–7 SSDS compliance, multi-project isolation, version upgrade/rollback, and R1–R3 adversarial security regression suites.

## Completed Work & Verified Gates
- `GOOS=windows/linux/darwin go vet ./...`: `PASS` (`0` issues).
- `go test -count=1 ./...`: `PASS` across all `8/8` Go packages.
- `Measure-SenCodeHealth`: `Healthy = True`, `Score = 100/100` across `66` files (`0` oversized files, `0` long functions, `0` deep nesting, `0` TODOs).
- `owner-judge-review` (3-Round Independent Security & Spec Audit): `PASS` (`14/14` findings resolved, `0 P0/P1/P2/P3` open).

## Next Immediate Steps
- Future `SSDS v1.1.0+` rulepacks can be added alongside `internal/ssds/versions/v1.0.0/` without affecting existing projects pinned to `1.0.0`.
