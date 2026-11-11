# Sen-VN v1.1.0 — Deterministic Verification & Bookend Protocol

Always follow this 8-command workflow (`sen` on PATH or `./bin/sen`). Only `sen` creates verification evidence.

## 1. Bookend Workflow (Exact 8 Commands)
1. **Orient:** Run `sen brief` at session start (if an `OPEN` task is interrupted or in `QUARANTINE`, report or run `sen undo`).
2. **Intake:** Record verbatim user intent before planning/editing (add `--express` if user says *"làm luôn đi"* / *"just do it"*):
   `sen intake --quote "<exact user request>" --req "<summary>" [--bugfix] [--security] [--vn-law] [--express]`
   Manage persistent project rules (`<= 20` max) via `sen intake --rule "<rule>" [--glob "*"] [--replace <id>] [--delete <id>]`.
3. **Begin:** Declare requirement links and exact file scope (`SMALL` / `MEDIUM` / `LARGE` auto-sized):
   `sen begin <task_id> --reqs R1,R2 --files path/a.go,path/a_test.go [--kind feature|bugfix|ui] [--allow-delete] [--express]`
4. **Verify / Undo:** Verify via direct binaries (`go`, `pytest`, `vitest`, `ffprobe` — never `sh -c` or `cmd /C`, `<= 3` repair rounds):
   `sen verify <task_id> -- <bin> [args...]` (or rollback byte-for-byte via `sen undo [task_id]`).
5. **Status & Done:** Inspect/approve via `sen status [--approve]`. Never claim completion until `sen done` exits `0`.

## 2. Non-Negotiable Guardrails
- Never edit `.sen/`, pre-existing locked tests, or immutable raw media (`raw/`, `master/`, `footage/`, `originals/`).
- Out-of-scope edits, empty test/media runs (`0 passed`, `0 streams`), and `> 3` blind retries (`[QUARANTINE 3/3]`) are blocked.

## 3. Progressive Companion Skill Routing (0 Idle Tokens)
Check `sen brief` for installed skills in `.agents/` (install via `sen init --install-companions [--packs list]`):
- **Dialectical Proof (`MEDIUM` / `LARGE`):** Activate `sen-dialectical` (Thesis -> Antithesis -> Synthesis + **Resource Cost** table).
- **Bugfix (`--bugfix` / `kind=bugfix`):** Activate `systematic-debugging` (`superpowers`) before editing.
- **Large Architecture (`risk=LARGE` or `>= 3` reqs):** Activate `brainstorming` + `writing-plans` (`superpowers`) before `sen begin`.
- **Security & VN Law:** Activate `security-audit` (`--security` -> `R_sec`) and `vn-law-check` (`--vn-law` -> `R_law`) when applicable.
- **Repeated Workflows (2–3x):** Offer `sen-skill-capture` to package reusable skills under `.agents/skills/<kebab-name>/SKILL.md`.
