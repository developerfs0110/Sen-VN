// Tệp templates.go cung cấp mẫu quy tắc AGENTS.md (~31 dòng) và trình tạo cấu hình .agents/hooks.json cho Antigravity.
package bridge

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// agentsMDTemplate định nghĩa quy trình Bookend 8 lệnh gọn nhẹ dùng chung cho mọi AI IDE.
const agentsMDTemplate = `# Sen MVP — Deterministic Verification & Bookend Protocol

Always follow this 8-command workflow in this repository. Only ` + "`sen`" + ` creates verification evidence.

## 1. Bookend Workflow (Exact 8 Commands)
1. **Orient:** Run ` + "`sen brief`" + ` at session start (if an ` + "`OPEN`" + ` task is interrupted, run ` + "`sen undo`" + ` or finish verifying it).
2. **Intake:** Before planning/coding, record verbatim user intent:
   ` + "`sen intake --quote \"<exact user request>\" --req \"<requirement summary>\" [--bugfix] [--security] [--vn-law]`" + `
   Record persistent user constraints (` + "`<= 20`" + ` max) via ` + "`sen intake --rule \"<rule>\" [--glob \"*\"] [--replace <id>] [--delete <id>]`" + `.
3. **Begin:** Declare requirement links and exact file scope before editing:
   ` + "`sen begin <task_id> --reqs R1,R2 --files path/a.go,path/a_test.go [--kind feature|bugfix|ui]`" + `
4. **Verify / Undo:** Verify using real direct test/build binaries (never ` + "`sh -c`" + ` or ` + "`cmd /C`" + `):
   ` + "`sen verify <task_id> -- <bin> [args...]`" + ` (or rollback cleanly via ` + "`sen undo [task_id]`" + `).
5. **Status & Done:** Inspect via ` + "`sen status [--approve]`" + `. Never claim work is finished until ` + "`sen done`" + ` exits ` + "`0`" + `.

## 2. Non-Negotiable Guardrails
- Never edit ` + "`.sen/`" + ` or pre-existing locked test files; put brand-new TDD test files in ` + "`--files`" + `.
- Out-of-scope file edits, zero-test runs (` + "`[no test files]`" + `, ` + "`0 passed`" + `), and uncovered ` + "`R_i`" + ` fail ` + "`sen verify`" + ` / ` + "`sen done`" + `.

## 3. Progressive Companion Skill Routing (0 Idle Tokens)
Check ` + "`sen brief`" + ` for installed Companion Skills in ` + "`.agents/`" + ` (degrade gracefully if absent):
- **Bugfix (` + "`--bugfix`" + ` / ` + "`kind=bugfix`" + `):** Activate ` + "`systematic-debugging`" + ` (` + "`superpowers`" + `) to isolate root cause before editing.
- **Complex / High Risk (` + "`risk=HIGH`" + ` or ` + "`>= 3`" + ` reqs):** Activate ` + "`brainstorming`" + ` + ` + "`writing-plans`" + ` (` + "`superpowers`" + `) before ` + "`sen begin`" + `.
- **Security Surface (auth, API, DB, secrets, crypto):** Activate ` + "`security-audit`" + ` and anchor ` + "`sen intake --security`" + ` (` + "`R_sec`" + `).
- **Vietnam Compliance (personal data, consent, e-commerce, AI output, release):** Activate ` + "`vn-law-check`" + ` and anchor ` + "`sen intake --vn-law`" + ` (` + "`R_law`" + `).
`

// RenderAgentsMD trả về nội dung chuẩn của tệp AGENTS.md khi khởi tạo dự án.
func RenderAgentsMD() string {
	return agentsMDTemplate
}

// RenderAntigravityHooksMap xây dựng cấu trúc 4 sự kiện vòng đời (SessionStart, PreInvocation, PreToolUse, Stop).
func RenderAntigravityHooksMap() map[string]any {
	return map[string]any{
		"SessionStart": []map[string]any{
			{
				"type":    "command",
				"command": "sen brief --hook SessionStart",
			},
		},
		"PreInvocation": []map[string]any{
			{
				"type":    "command",
				"command": "sen brief --hook PreInvocation",
			},
		},
		"PreToolUse": []map[string]any{
			{
				"matcher": "*",
				"hooks": []map[string]any{
					{
						"type":    "command",
						"command": "sen brief --hook PreToolUse",
					},
				},
			},
		},
		"Stop": []map[string]any{
			{
				"type":    "command",
				"command": "sen brief --hook Stop",
			},
		},
	}
}

// WriteAntigravityHooksFile hợp nhất mục cấu hình "sen-mvp" vào tệp hooks.json mà không xóa các hook sẵn có khác.
func WriteAntigravityHooksFile(hooksPath string) error {
	existing := make(map[string]any)
	if raw, err := os.ReadFile(hooksPath); err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &existing)
	}
	existing["sen-mvp"] = RenderAntigravityHooksMap()
	if err := os.MkdirAll(filepath.Dir(hooksPath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(existing, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(hooksPath, append(data, '\n'), 0o644)
}
