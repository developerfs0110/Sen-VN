// Tệp bridge_test.go kiểm thử phát hiện gói đồng hành, mẫu AGENTS.md v1.2.0 và 4 sự kiện JSON hook của Antigravity.
package bridge_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/developerfs0110/Sen-VN/internal/bridge"
	"github.com/developerfs0110/Sen-VN/internal/storage"
	"github.com/developerfs0110/Sen-VN/internal/workspace"
)

// TestDetectCompanionsWalksParentDirs xác minh khả năng dò ngược cây thư mục cha để nhận diện các gói kỹ năng .agents/.
func TestDetectCompanionsWalksParentDirs(t *testing.T) {
	parent := t.TempDir()
	project := filepath.Join(parent, "Code", "my-app")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(parent, ".agents", "plugins", "superpowers"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(parent, ".agents", "skills", "security-audit"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(parent, ".agents", "skills", "vn-law-check"), 0o755); err != nil {
		t.Fatal(err)
	}

	got := bridge.DetectCompanions(project)
	if !got.Superpowers || !got.SecurityAudit || !got.VnLawCheck || !got.AntigravityEnv {
		t.Fatalf("expected all 3 companions + AntigravityEnv detected, got: %+v", got)
	}

	isolated := t.TempDir()
	empty := bridge.DetectCompanions(isolated)
	if empty.Superpowers || empty.SecurityAudit || empty.VnLawCheck {
		t.Fatalf("expected graceful false on isolated temp dir, got: %+v", empty)
	}
}

// TestRenderAgentsMDConciseAndComplete kiểm tra độ dài súc tích và sự hiện diện đủ 9 lệnh (kể cả sen sdlc) trong AGENTS.md.
func TestRenderAgentsMDConciseAndComplete(t *testing.T) {
	md := bridge.RenderAgentsMD()
	lines := strings.Split(strings.TrimSpace(md), "\n")
	if len(lines) < 20 || len(lines) > 45 {
		t.Errorf("expected AGENTS.md around ~25-40 lines, got %d lines:\n%s", len(lines), md)
	}
	for _, must := range []string{
		"Sen-VN v1.2.0", "SSDS v1.0.0", "sen sdlc", "sen intake",
		"sen begin", "sen verify", "sen done", "systematic-debugging", "security-audit", "vn-law-check",
	} {
		if !strings.Contains(md, must) {
			t.Errorf("AGENTS.md missing %q", must)
		}
	}
}

// TestAntigravityHooksAllFourEventsAndUninitializedSafety kiểm chứng đầy đủ SessionStart, PreInvocation, PreToolUse và Stop.
func TestAntigravityHooksAllFourEventsAndUninitializedSafety(t *testing.T) {
	root := t.TempDir()

	// 0. Khi chưa khởi tạo DB (st == nil): read_file luôn cho phép ("allow") và không gây lỗi.
	var out bytes.Buffer
	in := `{"hookEventName":"PreToolUse","toolCall":{"name":"read_file","args":{"Path":"README.md"}}}`
	if err := bridge.HandleAntigravityHook(nil, root, strings.NewReader(in), &out); err != nil {
		t.Fatalf("uninitialized hook failed: %v", err)
	}
	if !strings.Contains(out.String(), `"allow"`) {
		t.Fatalf("expected allow when uninitialized, got: %s", out.String())
	}

	// Thiết lập sổ cái + baseline + khóa tệp test sẵn có + task OPEN bậc LARGE.
	_ = os.WriteFile(filepath.Join(root, "app_test.go"), []byte("package app\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "auth.go"), []byte("package app\n"), 0o644)

	st, err := storage.Open(filepath.Join(root, ".sen", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	_, _ = st.AddRequirement("", "Harden auth", "Update auth token check", false)
	if err := workspace.CaptureBaseline(st, root); err != nil {
		t.Fatal(err)
	}
	if err := st.SaveTask(storage.Task{
		ID: "T-AUTH", ReqIDs: []string{"R1"}, Title: "auth", Kind: "FEATURE",
		RiskAuto: "LARGE", ScopeFiles: []string{"auth.go"}, Status: "OPEN",
	}); err != nil {
		t.Fatal(err)
	}
	if err := workspace.BeginTaskSnapshot(st, root, "T-AUTH", []string{"auth.go"}); err != nil {
		t.Fatal(err)
	}

	// 1. SessionStart -> bơm systemMessage cảnh báo task T-AUTH đang OPEN.
	out.Reset()
	in = `{"hookEventName":"SessionStart","cwd":"` + strings.ReplaceAll(root, `\`, `\\`) + `"}`
	if err := bridge.HandleAntigravityHook(st, root, strings.NewReader(in), &out); err != nil {
		t.Fatalf("SessionStart failed: %v", err)
	}
	if !strings.Contains(out.String(), "systemMessage") || !strings.Contains(out.String(), "T-AUTH") {
		t.Errorf("expected SessionStart systemMessage with T-AUTH warning, got: %s", out.String())
	}

	// 2. PreInvocation -> ghi lastUserInput vào .sen/last_prompt.txt và giữ nguyên đúng 8 bảng chuẩn trong SQLite.
	out.Reset()
	in = `{"hookEventName":"PreInvocation","lastUserInput":"Please fix auth login bug without breaking API"}`
	if err := bridge.HandleAntigravityHook(st, root, strings.NewReader(in), &out); err != nil {
		t.Fatalf("PreInvocation failed: %v", err)
	}
	savedPrompt, err := os.ReadFile(filepath.Join(root, ".sen", "last_prompt.txt"))
	if err != nil || string(savedPrompt) != "Please fix auth login bug without breaking API" {
		t.Fatalf("expected .sen/last_prompt.txt saved, got %q (err=%v)", savedPrompt, err)
	}
	tables, _ := st.UserTableNames()
	if len(tables) != 8 {
		t.Fatalf("SQLite table count must remain strictly 8, got %d (%v)", len(tables), tables)
	}

	// 3a. PreToolUse -> chặn chỉnh sửa thư mục bảo vệ .sen/
	out.Reset()
	in = `{"hookEventName":"PreToolUse","toolCall":{"name":"write_file","args":{"Path":".sen/state.db"}}}`
	if err := bridge.HandleAntigravityHook(st, root, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	var preResp map[string]any
	_ = json.Unmarshal(out.Bytes(), &preResp)
	if preResp["decision"] != "deny" {
		t.Fatalf("expected deny on .sen/, got: %s", out.String())
	}

	// 3b. PreToolUse -> chặn chỉnh sửa tệp test đã khóa app_test.go
	out.Reset()
	in = `{"hookEventName":"PreToolUse","toolCall":{"name":"edit_file","args":{"Path":"app_test.go"}}}`
	if err := bridge.HandleAntigravityHook(st, root, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	preResp = nil
	_ = json.Unmarshal(out.Bytes(), &preResp)
	if preResp["decision"] != "deny" {
		t.Fatalf("expected deny on locked test app_test.go, got: %s", out.String())
	}

	// 3c. PreToolUse -> trả về force_ask trên tệp auth.go thuộc bậc rủi ro LARGE
	out.Reset()
	in = `{"hookEventName":"PreToolUse","toolCall":{"name":"edit_file","args":{"Path":"auth.go"}}}`
	if err := bridge.HandleAntigravityHook(st, root, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	preResp = nil
	_ = json.Unmarshal(out.Bytes(), &preResp)
	if preResp["decision"] != "force_ask" {
		t.Fatalf("expected force_ask on HIGH/LARGE risk task edit, got: %s", out.String())
	}

	// 4. Stop -> trả về "continue" khi task đang OPEN và nguyên nhân dừng là NO_TOOL_CALL.
	out.Reset()
	in = `{"hookEventName":"Stop","terminationReason":"NO_TOOL_CALL"}`
	if err := bridge.HandleAntigravityHook(st, root, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	var stopResp map[string]any
	_ = json.Unmarshal(out.Bytes(), &stopResp)
	if stopResp["decision"] != "continue" {
		t.Fatalf("expected Stop decision=continue while T-AUTH is OPEN, got: %s", out.String())
	}
}

// TestHooksEnforceOpenTaskGateSenProtectionDedupAndStopIdle kiểm tra khóa task OPEN, ẩn khóa API nhạy cảm và chống lặp tin nhắn.
func TestHooksEnforceOpenTaskGateSenProtectionDedupAndStopIdle(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o644)
	st, err := storage.Open(filepath.Join(root, ".sen", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	_ = workspace.CaptureBaseline(st, root)

	var out bytes.Buffer

	// Khi đã có .sen/state.db và không có task OPEN, ghi mã nguồn phải bị từ chối ("deny").
	in := `{"hookEventName":"PreToolUse","toolCall":{"name":"write_file","args":{"Path":"main.go"}}}`
	if err := bridge.HandleAntigravityHook(st, root, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"deny"`) || !strings.Contains(out.String(), "No active SEN task") {
		t.Fatalf("expected deny when no active OPEN task, got: %s", out.String())
	}

	// Lệnh shell đọc trực tiếp .sen/config.json phải bị chặn ("deny").
	out.Reset()
	in = `{"hookEventName":"PreToolUse","toolCall":{"name":"run_command","args":{"CommandLine":"Get-Content .sen/config.json"}}}`
	_ = bridge.HandleAntigravityHook(st, root, strings.NewReader(in), &out)
	if !strings.Contains(out.String(), `"deny"`) {
		t.Fatalf("expected deny on shell access to .sen/config.json, got: %s", out.String())
	}

	// PreInvocation tự ẩn secret trong last_prompt.txt và không bơm lặp lại ở invocationNum > 1.
	out.Reset()
	in = `{"hookEventName":"PreInvocation","invocationNum":1,"lastUserInput":"Use key sk-proj-1234567890abcdefghij"}`
	_ = bridge.HandleAntigravityHook(st, root, strings.NewReader(in), &out)
	saved, _ := os.ReadFile(filepath.Join(root, ".sen", "last_prompt.txt"))
	if strings.Contains(string(saved), "sk-proj-1234567890abcdefghij") || !strings.Contains(string(saved), "[REDACTED]") {
		t.Fatalf("expected secret scrubbed in last_prompt.txt, got: %q", string(saved))
	}

	out.Reset()
	in = `{"hookEventName":"PreInvocation","invocationNum":3,"lastUserInput":"Use key sk-proj-1234567890abcdefghij"}`
	_ = bridge.HandleAntigravityHook(st, root, strings.NewReader(in), &out)
	if strings.Contains(out.String(), "systemMessage") {
		t.Fatalf("expected duplicate PreInvocation (invocationNum=3, unchanged input) not to spam systemMessage, got: %s", out.String())
	}

	// JSON sai định dạng tại SessionStart trả về {} an toàn; Stop với fullyIdle=false cho phép dừng.
	out.Reset()
	_ = bridge.HandleAntigravityHookWithEvent(st, root, "SessionStart", strings.NewReader("{bad-json"), &out)
	if strings.TrimSpace(out.String()) != "{}" {
		t.Fatalf("expected {} on malformed SessionStart JSON, got: %s", out.String())
	}

	_ = st.CreateTask(storage.Task{
		ID: "T-BG", ReqIDs: []string{"R1"}, Title: "bg", Kind: "FEATURE",
		RiskAuto: "LOW", ScopeFiles: []string{"main.go"}, Status: "OPEN",
	})
	out.Reset()
	in = `{"hookEventName":"Stop","terminationReason":"NO_TOOL_CALL","fullyIdle":false}`
	_ = bridge.HandleAntigravityHook(st, root, strings.NewReader(in), &out)
	if !strings.Contains(out.String(), `"allow"`) {
		t.Fatalf("expected Stop allow when fullyIdle=false, got: %s", out.String())
	}
}

// TestPluginHookResolvesPayloadCwdSeparateFromPluginRoot đảm bảo hook chạy từ thư mục plugin không bao giờ làm bẩn pluginDir.
func TestPluginHookResolvesPayloadCwdSeparateFromPluginRoot(t *testing.T) {
	pluginDir := t.TempDir()
	pluginStore, err := storage.Open(filepath.Join(pluginDir, ".sen", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer pluginStore.Close()

	// 1. Thư mục người dùng chưa khởi tạo: cho phép ghi ghi chú .md nhưng chặn ghi mã nguồn .go theo SSDS §3.1 (không làm bẩn pluginDir).
	uninitUserDir := t.TempDir()
	escUninit := strings.ReplaceAll(uninitUserDir, `\`, `\\`)
	var out bytes.Buffer
	in := `{"hookEventName":"PreToolUse","cwd":"` + escUninit + `","toolCall":{"name":"write_file","args":{"Path":"notes.md"}}}`
	if err := bridge.HandleAntigravityHook(pluginStore, pluginDir, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"allow"`) {
		t.Fatalf("expected uninitialized user workspace to allow plain markdown notes, got: %s", out.String())
	}

	out.Reset()
	in = `{"hookEventName":"PreToolUse","cwd":"` + escUninit + `","toolCall":{"name":"write_file","args":{"Path":"hello.go"}}}`
	if err := bridge.HandleAntigravityHook(pluginStore, pluginDir, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"deny"`) || !strings.Contains(out.String(), "SSDS Mandatory Activation") {
		t.Fatalf("expected uninitialized user workspace to block software source write per SSDS §3.1, got: %s", out.String())
	}

	out.Reset()
	in = `{"hookEventName":"PreInvocation","invocationNum":1,"cwd":"` + escUninit + `","lastUserInput":"Hello from uninit workspace"}`
	_ = bridge.HandleAntigravityHook(pluginStore, pluginDir, strings.NewReader(in), &out)
	if _, err := os.Stat(filepath.Join(pluginDir, ".sen", "last_prompt.txt")); !os.IsNotExist(err) {
		t.Fatalf("pluginDir must NEVER be contaminated with user workspace last_prompt.txt")
	}

	// 2. Thư mục người dùng đã có .sen/state.db: áp dụng đúng sổ cái của người dùng và khóa tệp test.
	initUserDir := t.TempDir()
	_ = os.WriteFile(filepath.Join(initUserDir, "user_test.go"), []byte("package u\n"), 0o644)
	userStore, err := storage.Open(filepath.Join(initUserDir, ".sen", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	_ = workspace.CaptureBaseline(userStore, initUserDir)
	_ = userStore.Close()

	escInit := strings.ReplaceAll(initUserDir, `\`, `\\`)
	out.Reset()
	in = `{"hookEventName":"PreInvocation","invocationNum":1,"cwd":"` + escInit + `","lastUserInput":"Fix user project"}`
	if err := bridge.HandleAntigravityHook(pluginStore, pluginDir, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	gotPrompt, err := os.ReadFile(filepath.Join(initUserDir, ".sen", "last_prompt.txt"))
	if err != nil || string(gotPrompt) != "Fix user project" {
		t.Fatalf("expected last_prompt.txt written inside initUserDir, got %q (err=%v)", string(gotPrompt), err)
	}

	out.Reset()
	in = `{"hookEventName":"PreToolUse","cwd":"` + escInit + `","toolCall":{"name":"write_file","args":{"Path":"user_test.go"}}}`
	_ = bridge.HandleAntigravityHook(pluginStore, pluginDir, strings.NewReader(in), &out)
	if !strings.Contains(out.String(), `"deny"`) || !strings.Contains(out.String(), "locked") {
		t.Fatalf("expected user workspace locked test denied, got: %s", out.String())
	}
}

// TestExpressHooksRawMediaFFmpegGuardAndQuarantineStop kiểm chứng --express, chống sửa config.json, chặn ffmpeg ghi đè raw và thoát Stop khi QUARANTINE.
func TestExpressHooksRawMediaFFmpegGuardAndQuarantineStop(t *testing.T) {
	root := t.TempDir()
	_ = os.MkdirAll(filepath.Join(root, "raw"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "raw", "clip.mp4"), []byte("RAW_BYTES"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "svc.go"), []byte("package svc\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, ".env"), []byte("PORT=8080\n"), 0o644)

	st, err := storage.Open(filepath.Join(root, ".sen", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	_ = workspace.CaptureBaseline(st, root)
	_ = st.CreateTask(storage.Task{
		ID: "T-EXP", ReqIDs: []string{"R1"}, Title: "exp", Kind: "BUGFIX",
		RiskAuto: "LARGE", ScopeFiles: []string{"svc.go", ".env"}, Status: "OPEN",
	})
	_ = workspace.BeginTaskSnapshot(st, root, "T-EXP", []string{"svc.go", ".env"})

	if err := bridge.WriteSignedRuntimeConfig(root, true, "auto"); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	in := `{"hookEventName":"PreInvocation","invocationNum":1,"lastUserInput":"Sửa lỗi svc.go, làm luôn đi nhé"}`
	_ = bridge.HandleAntigravityHook(st, root, strings.NewReader(in), &out)
	if !strings.Contains(out.String(), "--express") {
		t.Fatalf("expected PreInvocation to mention --express for 'làm luôn đi', got: %s", out.String())
	}

	out.Reset()
	in = `{"hookEventName":"PreToolUse","toolCall":{"name":"edit_file","args":{"Path":"svc.go"}}}`
	_ = bridge.HandleAntigravityHook(st, root, strings.NewReader(in), &out)
	if !strings.Contains(out.String(), `"allow"`) {
		t.Fatalf("expected express_mode=true to allow non-destructive LARGE edit on svc.go, got: %s", out.String())
	}

	out.Reset()
	in = `{"hookEventName":"PreToolUse","toolCall":{"name":"edit_file","args":{"Path":".env"}}}`
	_ = bridge.HandleAntigravityHook(st, root, strings.NewReader(in), &out)
	if !strings.Contains(out.String(), `"force_ask"`) {
		t.Fatalf("expected express_mode=true to brake with force_ask on critical .env file, got: %s", out.String())
	}

	out.Reset()
	in = `{"hookEventName":"PreToolUse","toolCall":{"name":"run_command","args":{"CommandLine":"ffmpeg -y -i raw/clip.mp4 raw/clip.mp4"}}}`
	_ = bridge.HandleAntigravityHook(st, root, strings.NewReader(in), &out)
	if !strings.Contains(out.String(), `"deny"`) {
		t.Fatalf("expected PreToolUse to deny ffmpeg overwriting raw/clip.mp4, got: %s", out.String())
	}

	_ = os.WriteFile(filepath.Join(root, ".sen", "config.json"), []byte(`{"version":"1.2","express_mode":true}`), 0o644)
	out.Reset()
	in = `{"hookEventName":"PreToolUse","toolCall":{"name":"edit_file","args":{"Path":"svc.go"}}}`
	_ = bridge.HandleAntigravityHook(st, root, strings.NewReader(in), &out)
	if !strings.Contains(out.String(), `"deny"`) || !strings.Contains(out.String(), "tampering") {
		t.Fatalf("expected tampered config.json to be denied, got: %s", out.String())
	}
	_ = bridge.WriteSignedRuntimeConfig(root, true, "video")

	for i := 0; i < 3; i++ {
		_, _ = st.AppendEvidence("T-EXP", "go test ./...", "TEST", 0, 1, "fail")
	}
	out.Reset()
	in = `{"hookEventName":"Stop","terminationReason":"NO_TOOL_CALL"}`
	_ = bridge.HandleAntigravityHook(st, root, strings.NewReader(in), &out)
	if !strings.Contains(out.String(), `"allow"`) {
		t.Fatalf("expected Stop hook to return allow when OPEN task is in QUARANTINE (3/3), got: %s", out.String())
	}
	brief := bridge.FormatBrief(st, root, "")
	if !strings.Contains(brief, "QUARANTINE (3/3 failed verifies)") || !strings.Contains(brief, "profile=video") {
		t.Fatalf("expected FormatBrief to display QUARANTINE and profile=video, got:\n%s", brief)
	}
}
