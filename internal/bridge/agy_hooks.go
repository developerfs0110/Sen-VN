// Tệp agy_hooks.go thực thi giao thức JSON stdin/stdout cho 4 sự kiện vòng đời của Antigravity (hỗ trợ --express & QUARANTINE).
package bridge

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/developerfs0110/Sen-VN/internal/redact"
	"github.com/developerfs0110/Sen-VN/internal/storage"
	"github.com/developerfs0110/Sen-VN/internal/workspace"
)

// hookPayload ánh xạ cấu trúc JSON camelCase mà Antigravity (toolCall) hoặc Claude (toolName/toolInput) truyền qua stdin.
type hookPayload struct {
	HookEventName     string         `json:"hookEventName"`
	InvocationNum     int            `json:"invocationNum"`
	LastUserInput     string         `json:"lastUserInput"`
	Cwd               string         `json:"cwd"`
	WorkspacePaths    []string       `json:"workspacePaths"`
	TerminationReason string         `json:"terminationReason"`
	FullyIdle         *bool          `json:"fullyIdle"`
	ToolCall          *hookToolCall  `json:"toolCall"`
	ToolName          string         `json:"toolName"`
	ToolInput         map[string]any `json:"toolInput"`
}

// hookToolCall mô tả tên công cụ và tập tham số sắp được Agent gọi trong sự kiện PreToolUse.
type hookToolCall struct {
	ID   string         `json:"id"`
	Name string         `json:"name"`
	Args map[string]any `json:"args"`
}

// reDotSenRef phát hiện lệnh shell cố tình đọc/ghi trực tiếp vào thư mục nội bộ .sen/.
var reDotSenRef = regexp.MustCompile(`(?i)(^|[\s"'/\\=])\.sen($|[\s"'/\\])`)

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

	event := strings.TrimSpace(p.HookEventName)
	if event == "" {
		event = strings.TrimSpace(fallbackEvent)
	}

	effStore, effRoot, cleanup := resolveHookWorkspace(st, root, p)
	defer cleanup()

	switch event {
	case "SessionStart":
		return handleSessionStart(effStore, effRoot, stdout)
	case "PreInvocation":
		return handlePreInvocation(effStore, effRoot, p, stdout)
	case "PreToolUse":
		return handlePreToolUse(effStore, effRoot, p, stdout)
	case "Stop":
		return handleStopHook(effStore, effRoot, p, stdout)
	default:
		return writeJSON(stdout, map[string]any{})
	}
}

// resolveHookWorkspace ưu tiên mở .sen/state.db tại workspace người dùng (payload.Cwd) thay vì thư mục cài đặt plugin.
func resolveHookWorkspace(defaultStore *storage.Store, defaultRoot string, p hookPayload) (*storage.Store, string, func()) {
	payloadRoot := strings.TrimSpace(p.Cwd)
	if payloadRoot == "" && len(p.WorkspacePaths) > 0 {
		payloadRoot = strings.TrimSpace(p.WorkspacePaths[0])
	}
	if payloadRoot == "" || sameDirPath(payloadRoot, defaultRoot) {
		if defaultRoot == "" {
			defaultRoot = payloadRoot
		}
		return defaultStore, defaultRoot, func() {}
	}
	dbPath := filepath.Join(payloadRoot, ".sen", "state.db")
	if info, err := os.Stat(dbPath); err == nil && !info.IsDir() {
		opened, err := storage.Open(dbPath)
		if err == nil {
			return opened, payloadRoot, func() { _ = opened.Close() }
		}
	}
	return nil, payloadRoot, func() {}
}

// sameDirPath so sánh hai đường dẫn thư mục sau khi chuẩn hóa tuyệt đối theo hệ điều hành.
func sameDirPath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	absA, errA := filepath.Abs(a)
	absB, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return a == b
	}
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(absA), filepath.Clean(absB))
	}
	return filepath.Clean(absA) == filepath.Clean(absB)
}

// handleSessionStart bơm bản tóm tắt ngữ cảnh vào đầu phiên làm việc nếu dự án đã khởi tạo Sen.
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

// handlePreInvocation lưu câu nhắc nguyên văn, nhận diện cụm từ ủy quyền tự hành ("làm luôn đi") và tránh bơm trùng.
func handlePreInvocation(st *storage.Store, root string, p hookPayload, stdout io.Writer) error {
	if st == nil {
		return writeJSON(stdout, map[string]any{})
	}
	prompt := redact.ScrubText(strings.TrimSpace(p.LastUserInput))
	prevPrompt := ""
	if root != "" {
		promptFile := filepath.Join(root, ".sen", "last_prompt.txt")
		if oldBytes, err := os.ReadFile(promptFile); err == nil {
			prevPrompt = strings.TrimSpace(string(oldBytes))
		}
		if prompt != "" && prompt != prevPrompt {
			if err := os.MkdirAll(filepath.Dir(promptFile), 0o755); err == nil {
				_ = os.WriteFile(promptFile, []byte(prompt), 0o644)
			}
		}
	}

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

// handleStopHook yêu cầu hoàn tất task OPEN nhưng tự động cho phép dừng ("allow") nếu task đã chạm ngưỡng QUARANTINE 3/3.
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
			return writeJSON(stdout, map[string]any{"decision": "allow"})
		}
		return writeJSON(stdout, map[string]any{
			"decision": "continue",
			"reason":   fmt.Sprintf("Sen task %q is still OPEN. Run `sen verify %s -- <cmd>` or `sen undo %s` before ending turn.", open.ID, open.ID, open.ID),
		})
	}
	return writeJSON(stdout, map[string]any{"decision": "allow"})
}

// handlePreToolUse chặn lệnh shell can thiệp vào .sen/, chặn ffmpeg/magick ghi đè raw media và kiểm soát công cụ ghi tệp.
func handlePreToolUse(st *storage.Store, root string, p hookPayload, stdout io.Writer) error {
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
				"reason":   "Direct shell access to .sen/ is blocked by Sen; use `sen` CLI subcommands.",
			})
		}
		if rawReason := commandMutatesRawMedia(cmdLine); rawReason != "" {
			return writeJSON(stdout, map[string]any{
				"decision": "deny",
				"reason":   rawReason,
			})
		}
		return writeJSON(stdout, map[string]any{"decision": "allow"})
	}

	if !writeToolNames[toolName] {
		return writeJSON(stdout, map[string]any{"decision": "allow"})
	}
	return evaluateWriteToolDecision(st, root, toolName, tc.Args, stdout)
}

// commandTouchesDotSen kiểm tra xem câu lệnh shell có tham chiếu trực tiếp tới thư mục bảo vệ .sen hay không.
func commandTouchesDotSen(cmdLine string) bool {
	trimmed := strings.TrimSpace(cmdLine)
	if trimmed == "" {
		return false
	}
	return reDotSenRef.MatchString(trimmed)
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
