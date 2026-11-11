# Sen-VN v1.2.0

**Deterministic AI Coding, SEN-STD-SDLC-001 (`SSDS v1.0.0`) Lifecycle & Creative Workspace Referee (`sen`)** — zero paid API keys, zero CGO, zero Docker required, packaged as a Zero-Build GitHub repository and native **Antigravity Plugin**.

---

## Why Sen-VN v1.2.0 + SSDS v1.0.0?

AI agents frequently jump straight from a prompt into writing production code without requirements or architecture, fabricate `"PASS"` results for unexecuted tests, mutate approved architecture baselines silently, contaminate sibling repositories in multi-project workspaces, or spin in endless broken retry loops.

**Sen-VN v1.2.0** compiles the entire **SEN SOFTWARE DEVELOPMENT STANDARD (`SEN-STD-SDLC-001 v1.0.0`, 7 Parts / 28 Sections)** into an unforgeable local state machine backed by an **8-table SQLite WAL ledger (`.sen/state.db`)** and **9 CLI commands**:

1. **Zero-Prompt Automatic SSDS Activation (§3.1–§3.4):** `PreInvocation` detects software creation/modification requests (bilingual Vietnamese & English across 15 software domains), audits existing Brownfield repos (§2.3), selects `WF_MICRO | WF_STANDARD | WF_CRITICAL`, pins `applied_ssds_version = "1.0.0"`, and emits the `[SSDS v1.0.0 — Đã kích hoạt tự động]` Workflow Declaration banner — while pure Q&A queries and non-code media/docs folders stay untouched.
2. **10-Phase State Machine (`P0..P9`) & Pre-Execution Write Firewall (§5–§14, §22):** `PreToolUse` and `sen begin` physically block premature production code edits during `P0` (Charter), `P1` (Requirements), `P2` (Architecture/ADRs), and `P3` (Planning), separate `REVIEW_REQUIRED` from `APPROVED` (`Silence != Approval`, §21), and reject hollow `TBD/TODO` deliverables.
3. **27 Engineering Roles, Honest Review, 6-Link Traceability & CIA Lock (§4.3, §16–§20, §27):** Enforces `author_role != reviewer_role`, downgrades unverified `INDEPENDENT_AI_REVIEW` to `SELF_REVIEW` with explicit disclosure, locks approved `P1/P2` baselines until a 10-dimension `CIA-*` Change Impact Analysis is recorded, cross-checks `Requirement -> Component -> Task -> File -> Evidence -> Release` in `docs/traceability-matrix.md`, blocks fake QA `PASS` claims, and rejects expired `EXC-*` exceptions.
4. **Multi-Project Isolation & Independent SSDS Versioning:** Target-path nearest-project resolution (`ResolveNearestProjectRoot` + `--project <dir>` + `CheckNoSubprojectCrossing`) guarantees zero cross-project contamination when opening an umbrella workspace. Each project pins its own SHA-256-signed `ssds_version` (`sen sdlc version --changelog | --check | --upgrade | --rollback`), so releasing `SSDS v1.1` / `v2.0` never silently alters active `v1.0.0` projects.
5. **Bounded Repair (`<= 3` Rounds), Safe `--express` Brakes & Raw Media Guard:** Preserves `SMALL`/`MEDIUM`/`LARGE` sizing, `--express` autonomous execution (with `WF_CRITICAL`, deletion, and critical-path `force_ask` brakes), `[QUARANTINE 3/3]` loop protection, `ffprobe` media stream verification, and 1-command Companion/Pack management.

---

## Zero-Build Installation (No Go Toolchain Required!)

### Option 1 — Native Antigravity Plugin (Zero-Build)
```bash
git clone https://github.com/developerfs0110/Sen-VN.git .agents/plugins/sen-vn
```

### Option 2 — 1-Command CLI + Global Antigravity Hooks (`~/.gemini/config/hooks.json`)
**Windows PowerShell (No Admin / SmartScreen-Safe):**
```powershell
irm https://raw.githubusercontent.com/developerfs0110/Sen-VN/main/scripts/install-sen.ps1 | iex
# Optional: skip global ~/.gemini/config/hooks.json merge or initialize a workspace with packs:
powershell -ExecutionPolicy Bypass -File .\scripts\install-sen.ps1 -ConfigureHooks:$false
powershell -ExecutionPolicy Bypass -File .\scripts\install-sen.ps1 -Workspace . -Packs media-video,workflow-diagram
```

**macOS / Linux:**
```bash
curl -fsSL https://raw.githubusercontent.com/developerfs0110/Sen-VN/main/scripts/install-sen.sh | sh
./scripts/install-sen.sh --no-hooks --workspace . --packs media-video,workflow-diagram
```

### Option 3 — Build from Source (Go 1.26+)
```bash
go build -trimpath -ldflags="-s -w" -o sen.exe ./cmd/sen
```

---

## 9-Command CLI Surface (`sen [--project <dir>] <cmd>`)

| # | Command | Purpose |
|---|---|---|
| 1 | `sen init [--install-companions] [--packs <list\|all\|csv>] [--profile <...>] [--express] [--update]` | Initializes `.sen/state.db` (8 WAL tables), signed `.sen/config.json` (preserving pinned `ssds_version` & `project_id`), `AGENTS.md`, `.agents/hooks.json`, and companions/packs. |
| 2 | `sen brief [--path <file>] [--hook [event]]` | Emits `<250 token` startup brief (including `[SSDS v1.0.0 \| cat=... \| wf=... \| phase=... \| role=...]` & `ssds_notice`) or runs the 4-event Antigravity JSON hook adapter. |
| 3 | `sen intake` | Captures verbatim user quotes (`R1..Rn`, `R_sec`, `R_law`), toggles `--express`, or manages `<= 20` persistent project rules. |
| 4 | `sen begin <task_id>` | Opens a phase-checked scoped task (`--reqs ... --files ... [--phase P5] [--components C1,C2]`), enforces SSDS write firewall & subproject boundaries, locks tests/raw media, and snapshots files. |
| 5 | `sen verify <task_id> -- <bin> [args...]` | Enforces `<= 3` repair attempts, runs direct binaries (`go`, `pytest`, `vitest`, `ffprobe`), updates SSDS §20 `exec_state` (`EXECUTED`/`TESTED`/`VERIFIED`) + `verdict`, and appends hash-chained `evidence`. |
| 6 | `sen undo [task_id]` | Byte-for-byte rollback of an `OPEN` task from `.sen/snapshots/<task_id>/` and quarantine reset. |
| 7 | `sen done [-- <suite_bin> <args...>]` | Verifies `evidence` & `ssds_governance` hash chains, active `EXC-*` expirations, `P1/P2` baseline hashes, `P1..P6` gate/approval completion, and 100% requirement coverage; writes `.sen/delivery-report.md`. |
| 8 | `sen status [--approve] [--express]` | Displays profile, express mode, rules, requirements, tasks (`[QUARANTINE 3/3]`), and evidence chain; `--approve` unlocks quarantined tasks. |
| 9 | `sen sdlc <subcommand>` | Full `SEN-STD-SDLC-001` engine: `classify`, `status [--report]`, `gate [P]`, `review`, `approve`, `impact`, `exception`, `sync`, and `version [--changelog \| --check \| --upgrade \| --rollback]`. |

---

## 8-Table SQLite Schema (`.sen/state.db`, WAL Mode)

1. `rules` — Persistent user constraints (`COUNT(*) <= 20` hard cap).
2. `requirements` — Verbatim user quotes & typed SSDS requirements (`REQ-F/NF/BR/CON`, `R1..Rn`, `R_sec`, `R_law`).
3. `tasks` — Scoped tasks with `phase_id`, `arch_components_json`, `risk_auto`, SSDS §20 `exec_state` (`IMPLEMENTED..VERIFIED`), and `verdict`.
4. `snapshots` — Git-independent `__BASELINE__` manifest + per-task backups and locked test/raw-media markers.
5. `evidence` — Tamper-evident SHA-256 hash chain of real subprocess verification outputs.
6. `ssds_project` — Per-project UUIDv4, pinned `applied_ssds_version`, 7-category classification, workflow tier, and 8-dimension risk profile.
7. `ssds_phases` — Lifecycle state (`P0..P9`), engineering roles, verified deliverable SHA-256 hashes, gate flags, and user approval quotes.
8. `ssds_governance` — Tamper-evident SHA-256 hash chain of `REVIEW`, `DECISION`, `CHANGE_IMPACT` (`CIA-*`), and time-bounded `EXCEPTION` (`EXC-*`) records.
