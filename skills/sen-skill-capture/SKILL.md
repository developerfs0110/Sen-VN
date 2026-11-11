---
name: sen-skill-capture
description: Package repeated multi-step workflows (2-3x repetitions) or user process corrections into a reusable workspace skill under .agents/skills/<kebab-name>/SKILL.md. Trigger when workflows repeat or when the user says "đóng gói thành skill" or "capture skill".
---

# Sen Workflow Skill Capture (`sen-skill-capture`)

Use this skill to turn proven, repeated project workflows or user process corrections into a concise, zero-bloat workspace skill.

## 1. When to Trigger
Offer or run skill capture when ANY of the following occurs:
1. The same multi-step workflow (build/render pipeline, subtitle styling, release check, migration routine, etc.) is executed **2–3 times** across tasks.
2. The user explicitly corrects an agent process mistake and wants the lesson persisted beyond a 1-line `sen intake --rule`.
3. The user says **"đóng gói thành skill"**, **"lưu quy trình này thành skill"**, or **"capture skill"**.

## 2. Mandatory Safety & Naming Rules
1. **Ask Before Writing:** Unless the user explicitly asked to create the skill right now, propose the skill name, 1-sentence trigger description, and step outline, and wait for user confirmation before writing files.
2. **Target Path:** Always write workspace-captured skills to:
   `<workspace>/.agents/skills/<kebab-name>/SKILL.md`
3. **Reserved Prefix (`sen-` Forbidden):**
   - **NEVER** use the reserved `sen-` prefix (e.g. `sen-deploy` is forbidden; use `project-deploy` or `video-export-preset`).
   - Use lowercase `kebab-case` (`^[a-z0-9]+(-[a-z0-9]+)*$`).
4. **No Secrets or Bloat:** Never hardcode API keys, tokens, credentials, or machine-specific user paths; keep `SKILL.md` under `120` lines so idle context stays `0` tokens until triggered.

## 3. Standard `SKILL.md` Template
```markdown
---
name: <kebab-name>
description: <Exact trigger condition and 1-sentence capability summary>
---

# <Title>

## Trigger & Preconditions
- When to activate and required tools/inputs.

## Deterministic Steps
1. Concrete step/command 1.
2. Concrete step/command 2.

## Verification
- Exact `sen verify <task_id> -- <bin> [args...]` check proving clean output.
```

