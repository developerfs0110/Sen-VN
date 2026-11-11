# Sen MVP v1.0

**Deterministic AI Coding Ledger & Verification Referee (`sen`)** — zero paid API keys, zero CGO, zero Docker required, packaged as a Zero-Build GitHub repository and native **Antigravity Plugin**.

---

## Why Sen MVP?

AI coding agents frequently hallucinate completion: running `go test` on packages with `[no test files]`, weakening existing unit tests to turn red builds green, modifying out-of-scope files, dropping half of a multi-part user prompt, or leaking environment secrets into logs.

**Sen MVP** acts as an unforgeable local referee backed by a 5-table SQLite WAL ledger (`.sen/state.db`):
1. **Detection-First (Any IDE) + Pre-Execution Prevention (Antigravity):** Works everywhere via a ~31-line `AGENTS.md` Bookend Protocol, and automatically upgrades to pre-tool prevention on Antigravity via `hooks.json` (`SessionStart`, `PreInvocation`, `PreToolUse`, `Stop`).
2. **Only `sen` Creates Evidence:** Agents have no flag or SQL path to mark a task `"PASS"`. `sen verify` and `sen done` invoke allowed test/build binaries directly via `exec.CommandContext` (never `sh -c` or `cmd /C`), strip sensitive environment variables, reject empty test suites (`tests_ran == 0`), scrub leaked tokens (`[REDACTED]`), and append a SHA-256 hash-chained record.
3. **Hard Spine + Progressive Companion Skills (`0` Idle Tokens):** Discovers optional skills in `.agents/` (`superpowers`, `security-audit`, `vn-law-check`), anchors `R_sec` / `R_law` requirements in SQLite when triggered, and degrades gracefully when absent.

---

## Zero-Build Installation (No Go Toolchain Required!)

### Option 1 — Native Antigravity Plugin (Zero-Build)
Clone directly into your workspace `.agents/plugins/` folder:
```bash
git clone https://github.com/developerfs0110/Sen-VN.git .agents/plugins/sen-vn
```
Antigravity automatically discovers `plugin.json`, `hooks.json`, `rules/AGENTS.md`, and `skills/sen-setup/SKILL.md`. On first run, `./bin/sen` (`bin/sen.cmd` on Windows, `bin/sen` on macOS/Linux) automatically downloads and SHA-256-verifies the prebuilt GitHub Release binary into `$PLUGIN_DATA/bin` (falling back to a one-time `go build` if offline with Go installed), then executes in `<2ms` on every subsequent call.

### Option 2 — 1-Command CLI Install to User `PATH` (Students Without Go)
**Windows PowerShell (No Admin / SmartScreen-Safe):**
```powershell
irm https://raw.githubusercontent.com/developerfs0110/Sen-VN/main/scripts/install-sen.ps1 | iex
```
*(Or from a local clone: `powershell -ExecutionPolicy Bypass -File .\scripts\install-sen.ps1`)* — Fetches `checksums.txt` + `sen-windows-amd64.exe` over HTTPS, verifies exact SHA-256, runs `Unblock-File`, and adds `%LOCALAPPDATA%\Programs\sen\bin` to User `PATH`.

**macOS / Linux:**
```bash
curl -fsSL https://raw.githubusercontent.com/developerfs0110/Sen-VN/main/scripts/install-sen.sh | sh
```

### Option 3 — Build from Source (Go Developers, Go 1.26+)
```bash
go install github.com/developerfs0110/Sen-VN/cmd/sen@latest
# or from repo root:
go build -o sen.exe ./cmd/sen
```

---

## Frozen 8-Command CLI Surface

| # | Command | Purpose |
|---|---|---|
| 1 | `sen init [--with-companions] [--agy-hooks]` | Creates `.sen/state.db` (5 WAL tables), `.sen/config.json` (+ `.sen/config.sha256`), `AGENTS.md` (~31 lines), `.agents/hooks.json` (when Antigravity is detected or `--agy-hooks` is passed), and captures `__BASELINE__`. |
| 2 | `sen brief [--path <file>] [--hook [event]]` | Prints a `<250 token` startup brief (active rules, pending `R_i`, Companion Skills, interrupted `OPEN` task warning) or runs the Antigravity JSON stdin/stdout hook adapter. |
| 3 | `sen intake` | Records user intent (`--quote "<raw>" --req "<summary>" [--bugfix] [--security] [--vn-law]`) or manages project rules (`--rule "<text>" [--glob "*"] [--replace <id>] [--delete <id>]`, capped at `<= 20`). |
| 4 | `sen begin <task_id>` | Opens an immutable scoped task (`--reqs R1,R2 --files f1,f2 [--kind feature\|bugfix\|ui] [--allow-delete]`), computes deterministic `risk_auto`, locks pre-existing test files (`is_locked_test = 1`), and snapshots scope files `<= 10MB`. |
| 5 | `sen verify <task_id> -- <bin> [args...]` | Checks out-of-scope edits vs `__BASELINE__` & locked-test hashes, executes `<bin>` directly with scrubbed env, rejects `0`-test runs (and non-`TEST` runs on `BUGFIX`), advances `__BASELINE__` on pass, redacts secrets, and appends hash-chained `evidence`. |
| 6 | `sen undo [task_id]` | Restores an `OPEN` task's modified/deleted scope files byte-for-byte from `.sen/snapshots/<task_id>/`, removes newly created scope files safely, and marks the task `UNDONE`. |
| 7 | `sen done [-- <suite_bin> <args...>]` | Verifies the SHA-256 evidence hash chain from `GENESIS`, workspace integrity, live project suite pass, and 100% SQL coverage (`R_i -> VERIFIED Task -> exit_code = 0`). Writes `.sen/delivery-report.md`. |
| 8 | `sen status [--approve]` | Displays rules (`N/20`), Companion Skills, requirements, tasks, and evidence chain status; `--approve` marks pending requirements approved in one step. |

---

## Frozen 5-Table SQLite Schema (`.sen/state.db`, WAL Mode)

1. `rules` — Persistent user constraints (`COUNT(*) <= 20` hard cap).
2. `requirements` — Verbatim user quotes mapped to `R1..Rn`, `R_sec`, and `R_law`.
3. `tasks` — Immutable scoped work units with `req_ids_json`, `scope_files_json`, `risk_auto`, and `status` (`OPEN` / `VERIFIED` / `UNDONE`).
4. `snapshots` — Git-independent `__BASELINE__` manifest (`>10MB` media tracks `size + mtime_ns` without copying) + per-task backups and locked test SHA-256 hashes.
5. `evidence` — Tamper-evident canonical-JSON hash chain (`SHA256([seq, prev_hash, task_id, command, strength, tests_ran, exit_code, redacted_output])`).

---

## Honest Operational Boundaries

- **Process Boundary vs. OS Container:** Binary allowlisting + safe `PATH` resolution + direct `exec.CommandContext` + sensitive environment variable stripping blocks shell injection (`sh -c` / `cmd /C`), `%TEMP%` shim spoofing, and accidental parent API key exfiltration without requiring Docker on student laptops. Untrusted malicious code execution still requires OS/VM sandboxing.
- **Traceability vs. Semantic Proof:** `sen done` deterministically proves every `R_i` has a passing, non-empty verification run and prints `R_i <-> Verbatim Quote <-> Command` side-by-side so human semantic review takes seconds.
- **Defense-in-Depth Redaction:** Regex scrubbing (`sk-...`, `ghp_...`, `AIza...`, `Bearer ...`, `://user:pass@`, PEM private keys, `KEY=...`) complements child environment stripping to keep `.sen/state.db` clean of common credential patterns.
