# Sen MVP v1.0 — Technical Specification (Scope Freeze)

## 1. Purpose & Core Architectural Invariants
**Sen MVP** is a lightweight, zero-API-key, deterministic ledger and post-hoc/hook-enforced verification CLI (`sen`) paired with a compact `AGENTS.md` (~35 lines) and an optional Antigravity `hooks.json` adapter. It works across all AI IDEs and models—including student Windows setups without Docker or paid API keys.

### Three Core Invariants
1. **Detection-First (Universal) + Prevention (Hook-Capable IDEs):** Without hooks, `sen` acts as an unforgeable post-hoc referee (`sen verify` and `sen done` detect unplanned file modifications, locked-test tampering, zero-test runs, and missing requirements). On Antigravity, `.agents/hooks.json` upgrades enforcement to pre-execution prevention (`SessionStart`, `PreInvocation`, `PreToolUse`, `Stop`).
2. **Only `sen` Creates Evidence:** AI agents have no CLI flag or SQL path to declare a task `"PASS"`. `sen verify` and `sen done` execute binaries directly (`exec.CommandContext`, never `sh -c` or `cmd /C`), capture the real exit code and test count, strip sensitive environment variables, redact secrets from output, and append a SHA-256 hash-chained record (`entry_hash = SHA256(prev_hash + canonical_payload)`).
3. **Hard Spine + Progressive Companion Skills (`0` Idle Tokens):** Binary `sen` contains zero LLM calls and zero hardcoded domain bloat. Methodological and compliance power (`superpowers`, `security-audit`, `vn-law-check`) is discovered dynamically in `.agents/` and routed via 4 concise rules in `AGENTS.md`, anchoring security/legal checks as explicit `R_sec` / `R_law` rows in SQLite when triggered while degrading gracefully when absent.

---

## 2. Frozen CLI Surface (Exact 8 Commands)

| # | Command | Role & Deterministic Behavior |
|---|---|---|
| 1 | `sen init [--with-companions]` | Creates `.sen/state.db`, `.sen/snapshots/`, `.sen/config.json`, `AGENTS.md` (~35 lines with 4 Companion Skill routing rules), and `.agents/hooks.json` when Antigravity is detected or `--agy-hooks` is passed. |
| 2 | `sen brief` | Prints compact context (`<250 tokens`): matching project `rules`, pending `R_i`, detected Companion Skills (`superpowers`, `security-audit`, `vn-law-check`), and a warning if an `OPEN` task was interrupted (recommending `sen undo`). Also supports `sen brief --hook <event>` for Antigravity JSON hook stdin/stdout contracts. |
| 3 | `sen intake` | Records user intent (`--quote "<verbatim>" --req "<summary>" [--bugfix] [--security] [--vn-law]`), captures/updates the `__BASELINE__` workspace manifest (Git-independent), or manages project rules (`--rule "<text>" [--glob "*"] [--replace <id>] [--delete <id>]`, capped strictly at `<= 20` rules). |
| 4 | `sen begin <task_id>` | Opens a task (`--reqs R1,R2 --files f1,f2 [--kind feature\|bugfix\|ui]`). Deterministically computes `risk_auto` (`HIGH` on file deletion, config/CI/DB/auth paths, `>5` files, or `bugfix`). Locks existing test files by SHA-256 (`is_locked_test = 1`) and snapshots declared scope files into `.sen/snapshots/<task_id>/` (files `>10MB` record read-only `size + mtime_ns` without copying). |
| 5 | `sen verify <task_id> -- <bin> [args...]` | Validates allowed binary, strips `*KEY*`/`*TOKEN*`/`*SECRET*`/`*PASSWORD*`/`*CREDENTIAL*` env vars, checks for out-of-scope file edits against `__BASELINE__` and locked-test hash drift, executes `<bin>`, enforces `tests_ran > 0` on test runners (`go`, `pytest`, `jest`/`vitest`), tags strength (`TEST` > `BUILD` > `RUN`), redacts secrets, and writes hash-chained `evidence`. |
| 6 | `sen undo [task_id]` | Restores all modified/deleted scope files byte-for-byte from `.sen/snapshots/<task_id>/`, removes newly created scope files, and marks the task `UNDONE`. |
| 7 | `sen done [-- <suite_bin> <args...>]` | Final gate: verifies evidence hash-chain integrity, checks zero unplanned workspace edits, re-runs project verification command live if configured/supplied, and runs deterministic SQL check ensuring **100% of `requirements` map to at least one `VERIFIED` task with `exit_code = 0`**. Prints full `R_i <-> Quote <-> Task <-> Command (Strength, TestsRan)` table; exits `1` on any gap. |
| 8 | `sen status [--approve]` | Displays ASCII progress matrix, `R -> Task -> Evidence` coverage, active rules (`N/20`), and allows 1-step user approval of `requirements` via `--approve`. |

---

## 3. Frozen SQLite Schema (Exact 5 Tables, WAL Mode)
1. `rules(id, rule_text, user_quote, path_glob, created_at)` — Hard limit `COUNT(*) <= 20`.
2. `requirements(id, user_quote, summary, is_bugfix, approved, created_at)` — Traceable `R1..Rn` (including `R_sec`, `R_law` when triggered).
3. `tasks(id, req_ids_json, title, kind, risk_auto, scope_files_json, status, created_at, updated_at)` — Deterministic scope & risk tracking.
4. `snapshots(id, task_id, rel_path, existed_before, orig_sha256, orig_size, orig_mtime_ns, backup_path, is_locked_test)` — Dual-purpose: `__BASELINE__` index + per-task undo & locked-test hashes.
5. `evidence(seq, task_id, command, strength, tests_ran, exit_code, redacted_output, prev_hash, entry_hash, created_at)` — Tamper-evident hash chain.

---

## 4. Honest Operational Boundaries
- **Process Sandboxing:** Binary allowlisting + direct `exec.CommandContext` + env scrubbing prevents shell chaining and parent-key exfiltration, while reducing accidental blast radius. Full OS isolation requires containers (not assumed on student Windows machines).
- **Traceability vs. Semantic Correctness:** SQL coverage guarantees every `R_i` has an executed passing verification command; `sen done` prints `R_i <-> Quote <-> Command` side-by-side so human review takes seconds.
- **Secret Redaction:** Regex scrubbing mitigates common credential leaks (`sk-...`, `ghp_...`, `AIza...`, `Bearer ...`, `KEY=...`) in stored logs as defense-in-depth alongside environment filtering.

