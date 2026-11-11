// Tệp agy_hooks.go thực thi giao thức JSON stdin/stdout cho 4 sự kiện tiền kiểm và hậu kiểm của Antigravity.
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

// hookPayload ánh xạ cấu trúc JSON camelCase mà Antigravity truyền qua luồng stdin cho mỗi sự kiện hook.
type hookPayload struct {
	HookEventName     string        `json:"hookEventName"`
	InvocationNum     int           `json:"invocationNum"`
	LastUserInput     string        `json:"lastUserInput"`
	Cwd               string        `json:"cwd"`
	WorkspacePaths    []string      `json:"workspacePaths"`
	TerminationReason string        `json:"terminationReason"`
	FullyIdle         *bool         `json:"fullyIdle"`
	ToolCall          *hookToolCall `json:"toolCall"`
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
		return handleStopHook(effStore, p, stdout)
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

// handlePreInvocation lưu câu nhắc nguyên văn (đã lọc secret) vào .sen/last_prompt.txt và tránh bơm trùng ở các bước sau.
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

	if p.InvocationNum > 1 && (prompt == "" || prompt == prevPrompt) {
		return writeJSON(stdout, map[string]any{})
	}

	msg := FormatBrief(st, root, "")
	if prompt != "" {
		msg += "Note: Verbatim prompt saved to .sen/last_prompt.txt (`sen intake --req \"...\"` auto-loads quote if --quote is omitted)."
	}
	return writeJSON(stdout, map[string]any{
		"injectSteps": []map[string]any{
			{"systemMessage": map[string]any{"systemMessage": msg}},
		},
	})
}

// handleStopHook yêu cầu Agent tiếp tục hoàn tất verify hoặc undo nếu vẫn còn task đang treo ở trạng thái OPEN.
func handleStopHook(st *storage.Store, p hookPayload, stdout io.Writer) error {
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
		return writeJSON(stdout, map[string]any{
			"decision": "continue",
			"reason":   fmt.Sprintf("Sen task %q is still OPEN. Run `sen verify %s -- <cmd>` or `sen undo %s` before ending turn.", open.ID, open.ID, open.ID),
		})
	}
	return writeJSON(stdout, map[string]any{"decision": "allow"})
}

// handlePreToolUse chặn lệnh shell can thiệp trực tiếp vào .sen/ và kiểm tra quyền ghi tệp của Agent.
func handlePreToolUse(st *storage.Store, root string, p hookPayload, stdout io.Writer) error {
	if p.ToolCall == nil {
		return writeJSON(stdout, map[string]any{"decision": "allow"})
	}
	toolName := strings.ToLower(strings.TrimSpace(p.ToolCall.Name))

	if toolName == "run_command" {
		cmdLine := extractStringArg(p.ToolCall.Args, "CommandLine", "command", "cmd")
		if commandTouchesDotSen(cmdLine) {
			return writeJSON(stdout, map[string]any{
				"decision": "deny",
				"reason":   "Direct shell access to .sen/ is blocked by Sen; use `sen` CLI subcommands.",
			})
		}
		return writeJSON(stdout, map[string]any{"decision": "allow"})
	}

	if !writeToolNames[toolName] {
		return writeJSON(stdout, map[string]any{"decision": "allow"})
	}
	return evaluateWriteToolDecision(st, root, p.ToolCall.Args, stdout)
}

// evaluateWriteToolDecision từ chối sửa .sen/, tệp test bị khóa, tệp ngoài scope_files và buộc hỏi xác nhận khi rủi ro HIGH.
func evaluateWriteToolDecision(st *storage.Store, root string, args map[string]any, stdout io.Writer) error {
	rawPath := extractStringArg(args, "Path", "TargetFile", "file_path", "path")
	if rawPath == "" {
		return writeJSON(stdout, map[string]any{"decision": "allow"})
	}
	rel, err := workspace.NormalizeRelPath(root, rawPath)
	if err != nil {
		return writeJSON(stdout, map[string]any{"decision": "allow"})
	}
	lowerRel := strings.ToLower(rel)
	if lowerRel == ".sen" || strings.HasPrefix(lowerRel, ".sen/") {
		return writeJSON(stdout, map[string]any{
			"decision": "deny",
			"reason":   "Protected .sen/ ledger cannot be edited by agent tools.",
		})
	}
	if st == nil {
		return writeJSON(stdout, map[string]any{"decision": "allow"})
	}
	if isLockedTestInStore(st, rel) {
		return writeJSON(stdout, map[string]any{
			"decision": "deny",
			"reason":   fmt.Sprintf("Pre-existing test file %q is locked by Sen; add a new test file via `sen begin --files`.", rel),
		})
	}

	openTask := findOpenTask(st)
	if openTask == nil {
		return writeJSON(stdout, map[string]any{
			"decision": "deny",
			"reason":   "No active SEN task. Run sen intake and sen begin first.",
		})
	}

	inScope := false
	for _, sf := range openTask.ScopeFiles {
		if nsf, err := workspace.NormalizeRelPath(root, sf); err == nil && workspace.PathKey(nsf) == workspace.PathKey(rel) {
			inScope = true
			break
		}
	}
	if !inScope {
		return writeJSON(stdout, map[string]any{
			"decision": "deny",
			"reason":   fmt.Sprintf("File %q is outside active OPEN task %q scope_files %v.", rel, openTask.ID, openTask.ScopeFiles),
		})
	}
	if strings.EqualFold(openTask.RiskAuto, "HIGH") {
		return writeJSON(stdout, map[string]any{
			"decision": "force_ask",
			"reason":   fmt.Sprintf("Active Sen task %q has risk=HIGH; confirm editing %q.", openTask.ID, rel),
		})
	}
	risk, _ := workspace.ComputeRisk([]string{rel}, "FEATURE", root)
	if risk == "HIGH" {
		return writeJSON(stdout, map[string]any{
			"decision": "force_ask",
			"reason":   fmt.Sprintf("File %q is classified as HIGH risk by Sen.", rel),
		})
	}
	return writeJSON(stdout, map[string]any{"decision": "allow"})
}

// commandTouchesDotSen kiểm tra xem câu lệnh shell có tham chiếu trực tiếp tới thư mục bảo vệ .sen hay không.
func commandTouchesDotSen(cmdLine string) bool {
	trimmed := strings.TrimSpace(cmdLine)
	if trimmed == "" {
		return false
	}
	return reDotSenRef.MatchString(trimmed)
}

// isLockedTestInStore kiểm tra xem tệp mục tiêu có phải là tệp kiểm thử đã bị khóa trong sổ cái hay không.
func isLockedTestInStore(st *storage.Store, rel string) bool {
	if !workspace.IsTestFile(rel) {
		return false
	}
	targetKey := workspace.PathKey(rel)
	snaps, _ := st.GetSnapshots(workspace.BaselineTaskID)
	for _, s := range snaps {
		if s.IsLockedTest && s.ExistedBefore && workspace.PathKey(s.RelPath) == targetKey {
			return true
		}
	}
	if open := findOpenTask(st); open != nil {
		tsnaps, _ := st.GetSnapshots(open.ID)
		for _, s := range tsnaps {
			if s.IsLockedTest && s.ExistedBefore && workspace.PathKey(s.RelPath) == targetKey {
				return true
			}
		}
	}
	return false
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
