// Tệp agy_hooks.go thực thi giao thức JSON stdin/stdout cho 4 sự kiện vòng đời của Antigravity (SessionStart, PreInvocation, PreToolUse, Stop).
package bridge

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/developerfs0110/Sen-VN/internal/redact"
	"github.com/developerfs0110/Sen-VN/internal/ssds"
	"github.com/developerfs0110/Sen-VN/internal/storage"
	"github.com/developerfs0110/Sen-VN/internal/workspace"
)

// hookPayload ánh xạ cấu trúc JSON camelCase (Antigravity) và snake_case (Claude/Codex) truyền qua stdin.
type hookPayload struct {
	HookEventName     string         `json:"hookEventName"`
	HookEventNameAlt  string         `json:"hook_event_name"`
	InvocationNum     int            `json:"invocationNum"`
	LastUserInput     string         `json:"lastUserInput"`
	Prompt            string         `json:"prompt"`
	UserPrompt        string         `json:"user_prompt"`
	Cwd               string         `json:"cwd"`
	WorkspacePaths    []string       `json:"workspacePaths"`
	WorkspacePathsAlt []string       `json:"workspace_paths"`
	TerminationReason string         `json:"terminationReason"`
	FullyIdle         *bool          `json:"fullyIdle"`
	ToolCall          *hookToolCall  `json:"toolCall"`
	ToolName          string         `json:"toolName"`
	ToolNameAlt       string         `json:"tool_name"`
	ToolInput         map[string]any `json:"toolInput"`
	ToolInputAlt      map[string]any `json:"tool_input"`
}

// hookToolCall mô tả tên công cụ và tập tham số sắp được Agent gọi trong sự kiện PreToolUse.
type hookToolCall struct {
	ID   string         `json:"id"`
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

// writeToolNames liệt kê các công cụ chỉnh sửa tệp tin cần được kiểm soát phạm vi (scope_files) trước khi chạy.
var writeToolNames = map[string]bool{
	"write_file":                 true,
	"edit_file":                  true,
	"replace_file_content":       true,
	"multi_replace_file_content": true,
	"create_file":                true,
	"delete_file":                true,
}

// terminalStopBypass liệt kê các nguyên nhân dừng bất khả kháng không được phép chặn lặp vô hạn tại Stop hook.
var terminalStopBypass = map[string]bool{
	"USER_CANCELED":             true,
	"ERROR":                     true,
	"MAX_INVOCATIONS":           true,
	"MAX_FORCED_INVOCATIONS":    true,
	"MAX_TOKEN_BUDGET_EXCEEDED": true,
	"TERMINAL_CUSTOM_HOOK":      true,
	"TERMINAL_STEP_TYPE":        true,
	"INJECTED_RESPONSE":         true,
}

// HandleAntigravityHook đọc JSON từ stdin và ghi phản hồi JSON chuẩn Antigravity ra stdout.
func HandleAntigravityHook(st *storage.Store, root string, stdin io.Reader, stdout io.Writer) error {
	return HandleAntigravityHookWithEvent(st, root, "", stdin, stdout)
}

// HandleAntigravityHookWithEvent điều phối xử lý sự kiện hook với tên sự kiện dự phòng từ dòng lệnh.
func HandleAntigravityHookWithEvent(st *storage.Store, root, fallbackEvent string, stdin io.Reader, stdout io.Writer) error {
	var p hookPayload
	if stdin != nil {
		raw, err := io.ReadAll(stdin)
		if err == nil && len(strings.TrimSpace(string(raw))) > 0 {
			if jsonErr := json.Unmarshal(raw, &p); jsonErr != nil {
				ev := strings.TrimSpace(fallbackEvent)
				if ev == "PreToolUse" || ev == "Stop" {
					return writeJSON(stdout, map[string]any{"decision": "allow"})
				}
				return writeJSON(stdout, map[string]any{})
			}
		}
	}

	normalizeHookPayload(&p)
	event := strings.TrimSpace(p.HookEventName)
	if event == "" {
		event = strings.TrimSpace(fallbackEvent)
	}

	effStore, effRoot, payloadRoot, cleanup := resolveHookWorkspace(st, root, p)
	defer cleanup()

	switch event {
	case "SessionStart":
		return handleSessionStart(effStore, effRoot, stdout)
	case "PreInvocation":
		return handlePreInvocation(effStore, effRoot, p, stdout)
	case "PreToolUse":
		return handlePreToolUse(effStore, effRoot, payloadRoot, p, stdout)
	case "Stop":
		return handleStopHook(effStore, effRoot, p, stdout)
	default:
		return writeJSON(stdout, map[string]any{})
	}
}

// handleSessionStart bơm bản tóm tắt ngữ cảnh (bao gồm pha SSDS, vai trò và task kế tiếp) vào đầu phiên làm việc.
func handleSessionStart(st *storage.Store, root string, stdout io.Writer) error {
	if st == nil {
		return writeJSON(stdout, map[string]any{})
	}
	brief := FormatBrief(st, root, "")
	return writeJSON(stdout, map[string]any{
		"injectSteps": []map[string]any{
			{"systemMessage": map[string]any{"systemMessage": brief}},
		},
	})
}

// handlePreInvocation tự động nhận diện yêu cầu phần mềm để kích hoạt SSDS không cần nhắc lại (bảo vệ thư mục mẹ đa dự án).
func handlePreInvocation(st *storage.Store, root string, p hookPayload, stdout io.Writer) error {
	prompt := redact.ScrubText(strings.TrimSpace(p.LastUserInput))
	if st == nil && root != "" && workspace.IsUmbrellaMultiProjectDir(root) {
		if prompt != "" && ssds.DetectSoftwareIntent(root, prompt).IsSoftwareRequest {
			guide := fmt.Sprintf(
				"[SSDS Multi-Project Isolation] %q is an umbrella workspace containing independent child projects. Do NOT initialize `.sen/state.db` at the umbrella root; target the specific child project directory (`sen --project <subdir> sdlc classify` or edit files inside `<subdir>/`).",
				root,
			)
			return writeJSON(stdout, map[string]any{
				"injectSteps": []map[string]any{
					{"systemMessage": map[string]any{"systemMessage": guide}},
				},
			})
		}
		return writeJSON(stdout, map[string]any{})
	}
	if prompt != "" && root != "" && shouldAttemptAutoSSDS(st) {
		det := ssds.DetectSoftwareIntent(root, prompt)
		if det.IsSoftwareRequest {
			return autoBootstrapAndInjectSSDS(st, root, prompt, det, stdout)
		}
		if st == nil {
			return writeJSON(stdout, map[string]any{})
		}
	}
	if st == nil {
		return writeJSON(stdout, map[string]any{})
	}

	prevPrompt := savePromptIfChanged(root, prompt)
	expressAuthorized := prompt != "" && ContainsExpressPhrase(prompt)
	if expressAuthorized && root != "" {
		_ = WriteSignedRuntimeConfig(root, true, "")
	}
	if p.InvocationNum > 1 && (prompt == "" || prompt == prevPrompt) {
		return writeJSON(stdout, map[string]any{})
	}

	msg := FormatBrief(st, root, "")
	if prompt != "" {
		msg += "Note: Verbatim prompt saved to .sen/last_prompt.txt (`sen intake --req \"...\"` auto-loads quote if --quote is omitted)."
		if expressAuthorized {
			msg += "\nExpress Authorization Detected: User delegated autonomous execution — pass `--express` on `sen intake` and `sen begin` (still brakes on file deletions or critical .env/auth/payment/schema paths)."
		}
	}
	return writeJSON(stdout, map[string]any{
		"injectSteps": []map[string]any{
			{"systemMessage": map[string]any{"systemMessage": msg}},
		},
	})
}

// savePromptIfChanged ghi lại câu nhắc của người dùng vào .sen/last_prompt.txt và trả về câu nhắc liền trước.
func savePromptIfChanged(root, prompt string) string {
	if root == "" {
		return ""
	}
	promptFile := filepath.Join(root, ".sen", "last_prompt.txt")
	prevPrompt := ""
	if oldBytes, err := os.ReadFile(promptFile); err == nil {
		prevPrompt = strings.TrimSpace(string(oldBytes))
	}
	if prompt != "" && prompt != prevPrompt {
		if err := os.MkdirAll(filepath.Dir(promptFile), 0o755); err == nil {
			_ = os.WriteFile(promptFile, []byte(prompt), 0o644)
		}
	}
	return prevPrompt
}

// handleStopHook yêu cầu hoàn tất task OPEN, cho phép dừng khi chạm QUARANTINE 3/3 và tự động đồng bộ tệp trạng thái SSDS cuối lượt.
func handleStopHook(st *storage.Store, root string, p hookPayload, stdout io.Writer) error {
	if st == nil {
		return writeJSON(stdout, map[string]any{"decision": "allow"})
	}
	if p.FullyIdle != nil && !*p.FullyIdle {
		return writeJSON(stdout, map[string]any{"decision": "allow"})
	}
	if terminalStopBypass[strings.ToUpper(strings.TrimSpace(p.TerminationReason))] {
		return writeJSON(stdout, map[string]any{"decision": "allow"})
	}
	if open := findOpenTask(st); open != nil {
		if failed, _ := workspace.ActiveFailedVerifyCount(st, root, open.ID); failed >= workspace.MaxRepairRounds {
			if proj, _ := st.GetSSDSProject(); proj != nil {
				_ = ssds.SyncProjectStateFiles(st, root)
			}
			return writeJSON(stdout, map[string]any{"decision": "allow"})
		}
		return writeJSON(stdout, map[string]any{
			"decision": "continue",
			"reason":   fmt.Sprintf("Sen task %q is still OPEN. Run `sen verify %s -- <cmd>` or `sen undo %s` before ending turn.", open.ID, open.ID, open.ID),
		})
	}
	if proj, _ := st.GetSSDSProject(); proj != nil {
		_ = ssds.SyncProjectStateFiles(st, root)
	}
	return writeJSON(stdout, map[string]any{"decision": "allow"})
}

// handlePreToolUse chặn lệnh shell can thiệp vào .sen/, chặn ffmpeg/magick ghi đè raw media và kiểm soát công cụ ghi tệp theo dự án.
func handlePreToolUse(st *storage.Store, root, payloadRoot string, p hookPayload, stdout io.Writer) error {
	tc := p.ToolCall
	if tc == nil && strings.TrimSpace(p.ToolName) != "" {
		tc = &hookToolCall{Name: p.ToolName, Args: p.ToolInput}
	}
	if tc == nil {
		return writeJSON(stdout, map[string]any{"decision": "allow"})
	}
	toolName := strings.ToLower(strings.TrimSpace(tc.Name))

	if toolName == "run_command" {
		cmdLine := extractStringArg(tc.Args, "CommandLine", "command", "cmd")
		if commandTouchesDotSen(cmdLine) {
			return writeJSON(stdout, map[string]any{
				"decision": "deny",
				"reason":   "Direct shell access to .sen/ or internal ledger files is blocked by Sen; use `sen` CLI subcommands.",
			})
		}
		if envReason := commandOverridesProtectedEnv(cmdLine); envReason != "" {
			return writeJSON(stdout, map[string]any{
				"decision": "deny",
				"reason":   envReason,
			})
		}
		if rawReason := commandMutatesRawMedia(cmdLine); rawReason != "" {
			return writeJSON(stdout, map[string]any{
				"decision": "deny",
				"reason":   rawReason,
			})
		}
		if ask, govReason := commandRequiresHumanGovernanceConfirmation(cmdLine); ask {
			return writeJSON(stdout, map[string]any{
				"decision": "force_ask",
				"reason":   govReason,
			})
		}
		return writeJSON(stdout, map[string]any{"decision": "allow"})
	}

	if !writeToolNames[toolName] {
		return writeJSON(stdout, map[string]any{"decision": "allow"})
	}
	args := normalizeArgsForNestedProject(root, payloadRoot, tc.Args)
	return evaluateWriteToolDecision(st, root, toolName, args, stdout)
}

// findOpenTask trả về con trỏ tới task đang ở trạng thái OPEN (nếu có).
func findOpenTask(st *storage.Store) *storage.Task {
	tasks, err := st.ListTasks()
	if err != nil {
		return nil
	}
	for i := range tasks {
		if tasks[i].Status == "OPEN" {
			return &tasks[i]
		}
	}
	return nil
}

// extractStringArg trích xuất giá trị chuỗi không rỗng đầu tiên khớp với danh sách tên khóa ứng viên.
func extractStringArg(args map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := args[k]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return s
			}
		}
	}
	return ""
}

// writeJSON mã hóa đối tượng phản hồi thành một dòng JSON thuần túy trên luồng stdout.
func writeJSON(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}
