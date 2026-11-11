// Tệp agy_ssds_activation.go thực thi định vị ranh giới dự án gần nhất và tự động kích hoạt vòng đời SSDS 0-câu-nhắc cho Antigravity Hook.
package bridge

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/developerfs0110/Sen-VN/internal/ssds"
	"github.com/developerfs0110/Sen-VN/internal/storage"
	"github.com/developerfs0110/Sen-VN/internal/workspace"
)

// resolveHookWorkspace định vị ranh giới dự án gần nhất từ đường dẫn tệp đích và tuyệt đối không rò rỉ sang defaultStore khác thư mục.
func resolveHookWorkspace(defaultStore *storage.Store, defaultRoot string, p hookPayload) (*storage.Store, string, string, func()) {
	payloadRoot := strings.TrimSpace(p.Cwd)
	if payloadRoot == "" && len(p.WorkspacePaths) > 0 {
		payloadRoot = strings.TrimSpace(p.WorkspacePaths[0])
	}
	targetPath := extractPayloadTargetPath(p)
	if payloadRoot == "" && targetPath == "" {
		return defaultStore, defaultRoot, defaultRoot, func() {}
	}
	if payloadRoot == "" {
		payloadRoot = defaultRoot
	}

	effRoot, hasDB := workspace.ResolveNearestProjectRoot(targetPath, payloadRoot)
	if !hasDB {
		return nil, effRoot, payloadRoot, func() {}
	}
	if defaultStore != nil && sameDirPath(effRoot, defaultRoot) {
		return defaultStore, effRoot, payloadRoot, func() {}
	}
	opened, err := storage.Open(filepath.Join(effRoot, ".sen", "state.db"))
	if err != nil {
		return nil, effRoot, payloadRoot, func() {}
	}
	return opened, effRoot, payloadRoot, func() { _ = opened.Close() }
}

// extractPayloadTargetPath trích xuất đường dẫn tệp đích từ ToolCall hoặc ToolInput trong sự kiện PreToolUse.
func extractPayloadTargetPath(p hookPayload) string {
	if p.ToolCall != nil {
		return extractStringArg(p.ToolCall.Args, "Path", "TargetFile", "file_path", "path")
	}
	if len(p.ToolInput) > 0 {
		return extractStringArg(p.ToolInput, "Path", "TargetFile", "file_path", "path")
	}
	return ""
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

// shouldAttemptAutoSSDS xác định xem thư mục hiện tại chưa khởi tạo hoặc mới khởi tạo rỗng để kích hoạt SSDS tự động.
func shouldAttemptAutoSSDS(st *storage.Store) bool {
	if st == nil {
		return true
	}
	if proj, _ := st.GetSSDSProject(); proj != nil {
		return false
	}
	tasks, _ := st.ListTasks()
	reqs, _ := st.ListRequirements()
	return len(tasks) == 0 && len(reqs) == 0
}

// autoBootstrapAndInjectSSDS tự động tạo sổ cái .sen/state.db, chạy kiểm toán Brownfield (nếu có) và bơm Workflow Declaration (§3.4).
func autoBootstrapAndInjectSSDS(st *storage.Store, root, prompt string, det ssds.DetectionResult, stdout io.Writer) error {
	effSt := st
	if effSt == nil {
		if err := os.MkdirAll(filepath.Join(root, ".sen", "snapshots"), 0o755); err != nil {
			return writeJSON(stdout, map[string]any{})
		}
		opened, err := storage.Open(filepath.Join(root, ".sen", "state.db"))
		if err != nil {
			return writeJSON(stdout, map[string]any{})
		}
		defer func() { _ = opened.Close() }()
		effSt = opened
		_ = workspace.CaptureBaseline(effSt, root)
		_ = WriteSignedSSDSConfig(root, "", ssds.BundledLatestVersion, nil)
	}
	if det.HasExistingCode {
		_, _ = ssds.AuditBrownfieldProject(effSt, root)
	}
	proj, phases, err := ssds.BootstrapProject(effSt, root, filepath.Base(root), ssds.BundledLatestVersion, det)
	if err != nil {
		return writeJSON(stdout, map[string]any{})
	}
	expAuth := ContainsExpressPhrase(prompt)
	var expPtr *bool
	if expAuth {
		expPtr = &expAuth
	}
	_ = WriteSignedSSDSConfig(root, proj.ProjectID, proj.AppliedSSDSVersion, expPtr)
	_ = savePromptIfChanged(root, prompt)
	_ = ssds.SyncProjectStateFiles(effSt, root)

	primaryRole := "Technical Lead"
	for _, ph := range phases {
		if ph.PhaseID == proj.CurrentPhase {
			primaryRole = ph.PrimaryRole
			break
		}
	}
	decl := ssds.FormatActivationDeclaration(proj, phases, ssds.BundledLatestVersion)
	directive := fmt.Sprintf(
		"MANDATORY USER NOTIFICATION: Display the above [SSDS v%s — Đã kích hoạt tự động] Workflow Declaration banner to the user at the start of your reply, and execute Phase %s as %s (production code edits are physically locked until upstream gates/approvals pass).",
		proj.AppliedSSDSVersion, proj.CurrentPhase, primaryRole,
	)
	msg := decl + "\n" + directive + "\n\n" + FormatBrief(effSt, root, "")
	return writeJSON(stdout, map[string]any{
		"injectSteps": []map[string]any{
			{"systemMessage": map[string]any{"systemMessage": msg}},
		},
	})
}

// normalizeArgsForNestedProject quy chiếu đường dẫn tương đối từ Workspace mẹ về dự án con đích gần nhất.
func normalizeArgsForNestedProject(effRoot, payloadRoot string, args map[string]any) map[string]any {
	if effRoot == "" || payloadRoot == "" || sameDirPath(effRoot, payloadRoot) || len(args) == 0 {
		return args
	}
	rawPath := extractStringArg(args, "Path", "TargetFile", "file_path", "path")
	if rawPath == "" || filepath.IsAbs(rawPath) {
		return args
	}
	absTarget := filepath.Join(payloadRoot, filepath.FromSlash(rawPath))
	if rel, err := workspace.NormalizeRelPath(effRoot, absTarget); err == nil {
		cloned := make(map[string]any, len(args))
		updatedKey := false
		for k, v := range args {
			if k == "Path" || k == "TargetFile" || k == "file_path" || k == "path" {
				if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
					cloned[k] = rel
					updatedKey = true
					continue
				}
			}
			cloned[k] = v
		}
		if !updatedKey {
			cloned["Path"] = rel
		}
		return cloned
	}
	return args
}

// normalizeHookPayload hợp nhất các trường snake_case dự phòng về trường camelCase chuẩn để tương thích đa Harness.
func normalizeHookPayload(p *hookPayload) {
	if p == nil {
		return
	}
	if strings.TrimSpace(p.HookEventName) == "" {
		p.HookEventName = strings.TrimSpace(p.HookEventNameAlt)
	}
	if strings.TrimSpace(p.LastUserInput) == "" {
		if strings.TrimSpace(p.Prompt) != "" {
			p.LastUserInput = p.Prompt
		} else {
			p.LastUserInput = p.UserPrompt
		}
	}
	if len(p.WorkspacePaths) == 0 && len(p.WorkspacePathsAlt) > 0 {
		p.WorkspacePaths = p.WorkspacePathsAlt
	}
	if strings.TrimSpace(p.ToolName) == "" {
		p.ToolName = strings.TrimSpace(p.ToolNameAlt)
	}
	if len(p.ToolInput) == 0 && len(p.ToolInputAlt) > 0 {
		p.ToolInput = p.ToolInputAlt
	}
}


