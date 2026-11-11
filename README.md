# Sen-VN v1.1.0

**Deterministic AI Coding & Creative Workspace Referee (`sen`)** — zero paid API keys, zero CGO, zero Docker required, packaged as a Zero-Build GitHub repository and native **Antigravity Plugin**.

---

## Why Sen-VN?

AI agents frequently hallucinate completion: running `go test` on packages with `[no test files]`, weakening existing unit tests to turn red builds green, modifying out-of-scope files, overwriting raw video footage with `ffmpeg`, spinning in endless broken retry loops, or leaking environment secrets into logs.

**Sen-VN v1.1.0** acts as an unforgeable local referee backed by a frozen 5-table SQLite WAL ledger (`.sen/state.db`) and exact 8 CLI commands:
1. **Detection-First (Any IDE) + Pre-Execution Prevention (Antigravity):** Works everywhere via a ~36-line `AGENTS.md` Bookend Protocol, and upgrades to pre-tool prevention on Antigravity via `hooks.json` (`SessionStart`, `PreInvocation`, `PreToolUse`, `Stop`).
2. **3-Tier Deterministic Sizing (`SMALL` / `MEDIUM` / `LARGE`) & Safe `--express` Mode:** Automatically sizes tasks (`<=2` files `SMALL`, `3-5` files `MEDIUM`, `>5` files / `bugfix` / critical / `--allow-delete` `LARGE`). Saying *"làm luôn đi"* / *"just do it"* or passing `--express` runs `SMALL`/`MEDIUM` and non-destructive `LARGE` tasks autonomously while still forcing human confirmation (`force_ask`) on destructive (`--allow-delete`) or critical (`.env`, `auth/`, `payment/`, DB migration) operations. `.sen/config.json` is SHA-256 signed (`.sen/config.sha256`) so agents cannot self-enable `express_mode`.
3. **Bounded Repair Loop (`<= 3` Rounds) & Dialectical Audit:** Blocks the 4th consecutive failed `sen verify` (`[QUARANTINE 3/3]`), lets the `Stop` hook exit cleanly without infinite loops, and guides objective root-cause analysis (`sen-dialectical`) before human unlock (`sen status --approve`) or rollback (`sen undo`).
4. **Multi-Domain & Video Production Integrity (`ffprobe` + Raw Media Lock):** Auto-detects `code`, `web`, `video`, `media`, and `docs` profiles (`--profile`). Protects `raw/`, `master/`, `footage/`, and `originals/` assets from modification or in-place `ffmpeg` overwrites (`>10MB` tracked via zero-copy `size + mtime_ns`), and validates rendered media streams, codecs, dimensions, and duration via `ffprobe` (`MEDIA` strength).
5. **1-Command Companion Bundle + Fork Fallback + Optional Packs:** `sen init --install-companions` installs missing core companions (`superpowers`, `security-audit`, `vn-law-check`, `sen-dialectical`, `sen-skill-capture`) pinned by commit with automatic `developerfs0110/...` fork fallback, skipping already-installed skills (`[exists]`). Optional packs (`--packs list|all|workflow-diagram,media-video,media-image,voice-tts,media-3d`) and atomic `.bak`-backed updates (`sen init --update`) are built in.

---

## Zero-Build Installation (No Go Toolchain Required!)

### Option 1 — Native Antigravity Plugin (Zero-Build)
Clone directly into your workspace `.agents/plugins/` folder:
```bash
git clone https://github.com/developerfs0110/Sen-VN.git .agents/plugins/sen-vn
```
Antigravity discovers `plugin.json`, `hooks.json`, `rules/AGENTS.md`, `skills/sen-setup/SKILL.md`, `skills/sen-dialectical/SKILL.md`, and `skills/sen-skill-capture/SKILL.md`. On first run, `./bin/sen` (`bin/sen.cmd` on Windows, `bin/sen` on macOS/Linux) downloads and SHA-256-verifies the prebuilt GitHub Release binary into `$PLUGIN_DATA/bin` (falling back to a one-time `go build` if offline with Go installed), then executes in `<2ms` on every subsequent call.

### Option 2 — 1-Command CLI + Workspace Companion Install (No Go Required)
**Windows PowerShell (No Admin / SmartScreen-Safe):**
```powershell
irm https://raw.githubusercontent.com/developerfs0110/Sen-VN/main/scripts/install-sen.ps1 | iex
# Initialize a workspace with default companions + optional packs, or update safely:
powershell -ExecutionPolicy Bypass -File .\scripts\install-sen.ps1 -Workspace . -Packs media-video,workflow-diagram
powershell -ExecutionPolicy Bypass -File .\scripts\install-sen.ps1 -Workspace . -Update
```

**macOS / Linux:**
```bash
curl -fsSL https://raw.githubusercontent.com/developerfs0110/Sen-VN/main/scripts/install-sen.sh | sh
./scripts/install-sen.sh --workspace . --packs media-video,workflow-diagram
./scripts/install-sen.sh --workspace . --update
```

### Option 3 — Build from Source (Go Developers, Go 1.26+)
```bash
go install github.com/developerfs0110/Sen-VN/cmd/sen@latest
# or from repo root:
go build -trimpath -ldflags="-s -w" -o sen.exe ./cmd/sen
```

---

## Frozen 8-Command CLI Surface

| # | Command | Purpose |
|---|---|---|
| 1 | `sen init [--install-companions] [--packs <list\|all\|csv>] [--profile <auto\|code\|web\|video\|media\|docs>] [--express] [--update]` | Creates `.sen/state.db` (5 WAL tables), signed `.sen/config.json` (`.sen/config.sha256`), `AGENTS.md`, `.agents/hooks.json`, installs companions/optional packs (skipping existing, with fork fallback), or runs safe `.bak`-backed updates (`--update`). |
| 2 | `sen brief [--path <file>] [--hook [event]]` | Prints a `<250 token` startup brief (`profile`, `express`, Companion Skills, `QUARANTINE` / interrupted `OPEN` task warning, active rules, pending `R_i`) or runs the Antigravity JSON hook adapter. |
| 3 | `sen intake` | Records user intent (`--quote "<raw>" --req "<summary>" [--bugfix] [--security] [--vn-law] [--express]`) or manages project rules (`--rule "<text>" [--glob "*"] [--replace <id>] [--delete <id>]`, capped at `<= 20`). |
| 4 | `sen begin <task_id>` | Opens an immutable scoped task (`--reqs R1,R2 --files f1,f2 [--kind feature\|bugfix\|ui] [--allow-delete] [--express]`), sizes risk (`SMALL` / `MEDIUM` / `LARGE`), locks pre-existing tests & raw media (`is_locked_test = 1`), and snapshots scope files `<= 10MB`. |
| 5 | `sen verify <task_id> -- <bin> [args...]` | Enforces `<= 3` consecutive failed attempts (`[QUARANTINE 3/3]`), checks scope & locked-asset integrity, executes `<bin>` directly (`go`, `pytest`, `vitest`, `ffprobe`, etc.) with scrubbed env, rejects empty tests / corrupt 0-stream media, advances `__BASELINE__` on pass, and appends hash-chained `evidence`. |
| 6 | `sen undo [task_id]` | Restores an `OPEN` task's modified/deleted scope files byte-for-byte from `.sen/snapshots/<task_id>/`, removes newly created scope files, clears quarantine state, and marks the task `UNDONE`. |
| 7 | `sen done [-- <suite_bin> <args...>]` | Verifies the SHA-256 evidence chain from `GENESIS`, workspace/raw-media integrity, domain-appropriate live suite pass, and 100% SQL coverage (`R_i -> VERIFIED Task -> TEST/MEDIA/BUILD/RUN`). Writes enhanced `.sen/delivery-report.md`. |
| 8 | `sen status [--approve] [--express]` | Displays domain profile, express mode, rules (`N/20`), Companion Skills, requirements, tasks (including `[QUARANTINE 3/3]`), and evidence chain status; `--approve` approves pending requirements and unlocks quarantined `OPEN` tasks. |

---

## Frozen 5-Table SQLite Schema (`.sen/state.db`, WAL Mode)

1. `rules` — Persistent user constraints (`COUNT(*) <= 20` hard cap).
2. `requirements` — Verbatim user quotes mapped to `R1..Rn`, `R_sec`, and `R_law`.
3. `tasks` — Immutable scoped work units with `req_ids_json`, `scope_files_json`, 3-tier `risk_auto` (`SMALL` / `MEDIUM` / `LARGE`), and `status` (`OPEN` / `VERIFIED` / `UNDONE`).
4. `snapshots` — Git-independent `__BASELINE__` manifest (`>10MB` raw media tracks `size + mtime_ns` without copying) + per-task backups and locked test/raw-media markers (`is_locked_test = 1`).
5. `evidence` — Tamper-evident canonical-JSON hash chain (`SHA256([seq, prev_hash, task_id, command, strength, tests_ran, exit_code, redacted_output])`) powering bounded repair quarantine (`<= 3` failed attempts) and 100% `sen done` coverage.
