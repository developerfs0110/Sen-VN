---
name: sen-dialectical
description: Objective 3-step Dialectical Audit (Thesis, Antithesis, Synthesis + mandatory Resource Cost table) and <= 3 bounded repair loop discipline for MEDIUM and LARGE tasks.
---

# Sen Dialectical Audit & Bounded Repair (`sen-dialectical`)

Activate this skill automatically before proposing or implementing **`MEDIUM`** or **`LARGE`** tasks, or whenever a task hits repeated verification failures.

## 1. Three-Step Objective Dialectical Proof (Before Coding `MEDIUM` / `LARGE`)
Evaluate 2–3 realistic candidate approaches from a strict domain-expert perspective before locking scope with `sen begin`:

1. **Thesis (Tech Stack & Candidate Strength):**
   - Inspect the workspace's actual architecture, dependencies, and `DomainProfile` (`code`, `web`, `video`, `media`, `docs`).
   - State why the shortlisted options are the strongest industry-standard candidates for this project's constraints.
2. **Antithesis (Counterexamples, Edge Cases & STRIDE):**
   - Challenge each candidate objectively: failure modes, data/asset loss risks, OS portability (`Windows` / `macOS` / `Linux`), security boundaries (STRIDE: spoofing, tampering, secret leakage, injection), and hidden coupling.
3. **Synthesis & Mandatory Resource Cost Table:**
   - Prove why the recommended pick is **Best-of-the-Best** for this workspace and present a concise comparison table that **ALWAYS includes a `Resource Cost (Tài nguyên tiêu tốn)` column** covering:
     - **Tokens:** Context & reasoning overhead.
     - **Time:** Implementation & verification latency.
     - **CPU / RAM / Disk:** Runtime footprint & asset storage impact.
     - **Maintenance:** Long-term upgrade & debugging cost.

| Option | Core Mechanism | Key Trade-offs / Risks | Resource Cost (Tài nguyên tiêu tốn: Token, Time, CPU/RAM/Disk, Maintenance) | Verdict |
|---|---|---|---|---|
| **Option A (Recommended)** | ... | ... | Low tokens, fast execution, minimal RAM/disk, zero external maintenance | **Best-of-the-Best** |
| Option B | ... | ... | ... | Rejected |

*(Skip waiting for interactive approval only when `--express` / "Làm luôn đi" mode is active AND no file deletion or critical `.env`/`auth`/`payment`/`schema` path is touched.)*

## 2. Bounded Repair Loop (`<= 3` Rounds Hard Cap)
`sen verify` enforces `MaxRepairRounds = 3` consecutive failed verification attempts per task in SQLite (`evidence`):
- **Round 1–2 Failure:** Read the exact compiler/test/`ffprobe` failure output, isolate root cause (never guess blindly or weaken tests), and apply a targeted fix within declared `--files`.
- **Round 3 Failure (`[QUARANTINE 3/3]`):** STOP blind retries immediately (`sen verify` blocks a 4th consecutive failure). Present an objective root-cause diagnosis to the user:
  - Ask the user to review and run `sen status --approve` to grant a fresh 3-round repair cycle, **OR**
  - Roll back cleanly byte-for-byte via `sen undo <task_id>` and redesign the approach.

