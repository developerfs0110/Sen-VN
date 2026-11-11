# Sen-VN v1.1.0 — Technical Specification (Scope Freeze)

## 1. Purpose & Core Architectural Invariants
**Sen-VN v1.1.0** is a lightweight, zero-API-key, deterministic ledger and post-hoc/hook-enforced verification CLI (`sen`) paired with a compact `AGENTS.md` (~36 lines) and a native Antigravity `hooks.json` adapter. It serves software engineering, web/UI, video production, media, and documentation workflows across Windows, macOS, and Linux.

### Core Invariants
1. **Frozen Surface (8 Commands, 5 SQLite Tables):** All v1.1 capabilities are delivered strictly through the frozen 8 top-level commands (`init`, `brief`, `intake`, `begin`, `verify`, `undo`, `done`, `status`) and 5 SQLite WAL tables (`rules`, `requirements`, `tasks`, `snapshots`, `evidence`).
2. **Only `sen` Creates Evidence:** `sen verify` and `sen done` invoke allowed binaries directly (`exec.CommandContext`, never `sh -c` or `cmd /C`), validate real test counts or `ffprobe` media stream integrity (`TEST`, `MEDIA`, `BUILD`, `RUN`), scrub sensitive env vars and leaked credentials (`[REDACTED]`), and append SHA-256 hash-chained `evidence`.
3. **3-Tier Deterministic Sizing, Bounded Quarantine (`<= 3`), & Tamper-Signed `--express`:** Tasks auto-size into `SMALL` (`<=2` files), `MEDIUM` (`3-5` files), or `LARGE` (`>5` files, `BUGFIX`, `--allow-delete`, or critical surfaces). `--express` accelerates non-destructive execution while enforcing `force_ask` on deletions/critical files and verifying `.sen/config.sha256`. Consecutive failed verifies cap at `3/3` (`[QUARANTINE 3/3]`) until unlocked via `sen status --approve` or rolled back via `sen undo`.
4. **Progressive Companion Skills + Fork Fallback (`0` Idle Tokens):** `sen init --install-companions` and `--packs` install pinned skills (`packages.lock.json`) with automatic `developerfs0110/...` fork fallback, skipping existing folders and supporting atomic `.bak`-rollback updates (`sen init --update`).

---

## 2. Frozen CLI Surface (Exact 8 Commands)

| # | Command | Role & Deterministic Behavior |
|---|---|---|
| 1 | `sen init [--install-companions] [--packs <list\|all\|csv>] [--profile <auto\|code\|web\|video\|media\|docs>] [--express] [--update]` | Initializes `.sen/state.db`, `.sen/snapshots/`, signed `.sen/config.json`, `AGENTS.md`, `.agents/hooks.json`, installs companions/packs (with fork fallback & skip-existing), or atomically updates workspace packages (`--update`). |
| 2 | `sen brief [--path <file>] [--hook [event]]` | Prints compact context (`<250 tokens`: `profile`, `express`, companions, `QUARANTINE` / interrupted `OPEN` task warning, matching `rules`, pending `R_i`) or handles Antigravity JSON hooks (`SessionStart`, `PreInvocation`, `PreToolUse`, `Stop`). |
| 3 | `sen intake` | Records user intent (`--quote "<verbatim>" --req "<summary>" [--bugfix] [--security] [--vn-law] [--express]`) or manages project rules (`<= 20` cap). |
| 4 | `sen begin <task_id>` | Opens an immutable scoped task (`--reqs R1,R2 --files f1,f2 [--kind feature\|bugfix\|ui] [--allow-delete] [--express]`), computes `SMALL`/`MEDIUM`/`LARGE`, locks pre-existing tests and raw media (`is_locked_test = 1`), and snapshots files `<= 10MB`. |
| 5 | `sen verify <task_id> -- <bin> [args...]` | Enforces `<= 3` repair rounds, checks out-of-scope & locked test/raw-media drift, executes `<bin>`, validates `tests_ran > 0` (`go`, `pytest`, `jest`/`vitest`) or valid streams (`ffprobe`), advances `__BASELINE__` on pass, and appends hash-chained `evidence`. |
| 6 | `sen undo [task_id]` | Restores modified/deleted scope files byte-for-byte, removes newly created scope files, clears quarantine reset state, and marks the task `UNDONE`. |
| 7 | `sen done [-- <suite_bin> <args...>]` | Verifies evidence hash-chain integrity, workspace/raw-media integrity, domain-aware live suite pass, and 100% SQL coverage (`R_i -> VERIFIED Task`). Writes `.sen/delivery-report.md` (`Version`, `DomainProfile`, `ExpressMode`, `Companions`, `Risk Tier`, `Strength/Streams`). |
| 8 | `sen status [--approve] [--express]` | Displays domain profile, express mode, rules (`N/20`), companions, requirements, tasks (`[QUARANTINE 3/3]`), and evidence chain status; `--approve` approves pending requirements and unlocks quarantined `OPEN` tasks. |

---

## 3. Frozen SQLite Schema (Exact 5 Tables, WAL Mode)
1. `rules(id, rule_text, user_quote, path_glob, created_at)` — Hard limit `COUNT(*) <= 20`.
2. `requirements(id, user_quote, summary, is_bugfix, approved, created_at)` — Traceable `R1..Rn`, `R_sec`, `R_law`.
3. `tasks(id, req_ids_json, title, kind, risk_auto, scope_files_json, status, created_at, updated_at)` — Immutable tasks with 3-tier `risk_auto` (`SMALL` / `MEDIUM` / `LARGE`).
4. `snapshots(id, task_id, rel_path, existed_before, orig_sha256, orig_size, orig_mtime_ns, backup_path, is_locked_test)` — `__BASELINE__` index (`>10MB` zero-copy `size + mtime_ns`), per-task backups, and locked tests / immutable raw media (`is_locked_test = 1`).
5. `evidence(seq, task_id, command, strength, tests_ran, exit_code, redacted_output, prev_hash, entry_hash, created_at)` — Tamper-evident hash chain (`TEST`, `MEDIA`, `BUILD`, `RUN`).
