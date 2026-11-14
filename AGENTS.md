# Sen-VN v1.2.0 + SSDS v1.0.0 — Deterministic SDLC & Verification Protocol (9 Commands)

Always follow this 9-command workflow (`sen` on PATH or `./bin/sen`). Only `sen` creates verification & SSDS gate evidence.

## 1. SSDS Automatic Activation & Mandatory User Notifications (SEN-STD-SDLC-001)
1. **Auto-Activation (§3.4):** Software creation/modification requests auto-activate SSDS via `PreInvocation` or `sen sdlc classify`. Always display the `[SSDS v1.0.0 — Đã kích hoạt tự động]` Workflow Declaration banner at Turn 1.
2. **SDLC Lifecycle & Gates:** Execute phases in order (`P0..P9` per `WF_MICRO | WF_STANDARD | WF_CRITICAL`):
   `sen sdlc status [--report]` | `sen sdlc gate [P]` | `sen sdlc review --phase P --author A --reviewer B --verdict PASS` | `sen sdlc approve --phase P --quote "..."` | `sen sdlc impact ...` | `sen sdlc exception ...` | `sen sdlc sync` | `sen sdlc version ...`.
3. **User Notifications:** Notify user on `REVIEW_REQUIRED` gates (silence != approval, §21), emit §25 Progress Report at milestone end, and surface `ssds_notice` only as an optional upgrade hint (active projects stay pinned to `applied_ssds_version`).

## 2. Bookend Implementation & Verification Commands
1. **Orient & Intake:** Run `sen brief` at session start; capture user quotes/rules via `sen intake --quote "<exact request>" --req "<summary>" [--bugfix] [--security] [--vn-law] [--express]` (`--rule` max 20).
2. **Begin (P4/P5):** Declare requirement/component links & exact file scope:
   `sen begin <task_id> --reqs REQ-F-001 --phase P5 --components COMP-01 --files path/a.go,path/a_test.go [--kind feature|bugfix|ui] [--allow-delete] [--express]`
3. **Verify / Undo / Done:** Verify via direct binaries (`go`, `pytest`, `vitest`, `ffprobe` — never `sh -c`/`cmd /C`, `<= 3` repair rounds):
   `sen verify <task_id> -- <bin> [args...]`, rollback via `sen undo [task_id]`, inspect via `sen status [--approve]`, and close via `sen done`.

## 3. Non-Negotiable Guardrails & Progressive Companion Routing (0 Idle Tokens)
- Never edit `.sen/`, cross into nested subprojects, write code in `P0..P3`, mutate approved `P1/P2` baselines without `CIA-*`, claim `PASS` without `evidence`, or touch locked tests / raw media (`raw/`, `master/`, `footage/`, `originals/`).
- Route installed `.agents/` companions (`sen init --install-companions`): `sen-dialectical` (`MEDIUM/LARGE`), `systematic-debugging` (`--bugfix`), `brainstorming` + `writing-plans` (`LARGE`), `security-audit` (`--security`), `vn-law-check` (`--vn-law`), `sen-skill-capture`.
