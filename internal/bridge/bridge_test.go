package bridge_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/developerfs0110/Sen-VN/internal/bridge"
	"github.com/developerfs0110/Sen-VN/internal/storage"
	"github.com/developerfs0110/Sen-VN/internal/workspace"
)

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

func TestRenderAgentsMDConciseAndComplete(t *testing.T) {
	md := bridge.RenderAgentsMD()
	lines := strings.Split(strings.TrimSpace(md), "\n")
	if len(lines) < 25 || len(lines) > 45 {
		t.Errorf("expected AGENTS.md around ~35 lines, got %d lines:\n%s", len(lines), md)
	}
	for _, must := range []string{"sen intake", "sen begin", "sen verify", "sen done", "systematic-debugging", "security-audit", "vn-law-check"} {
		if !strings.Contains(md, must) {
			t.Errorf("AGENTS.md missing %q", must)
		}
	}
}

func TestAntigravityHooksAllFourEventsAndUninitializedSafety(t *testing.T) {
	root := t.TempDir()

	// 0. Uninitialized DB (st == nil): read_file must allow and never crash.
	var out bytes.Buffer
	in := `{"hookEventName":"PreToolUse","toolCall":{"name":"read_file","args":{"Path":"README.md"}}}`
	if err := bridge.HandleAntigravityHook(nil, root, strings.NewReader(in), &out); err != nil {
		t.Fatalf("uninitialized hook failed: %v", err)
	}
	if !strings.Contains(out.String(), `"allow"`) {
		t.Fatalf("expected allow when uninitialized, got: %s", out.String())
	}

	// Setup initialized store + baseline + locked test + HIGH risk OPEN task.
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

	// 1. SessionStart -> injects systemMessage brief warning about OPEN task T-AUTH.
	out.Reset()
	in = `{"hookEventName":"SessionStart","cwd":"` + strings.ReplaceAll(root, `\`, `\\`) + `"}`
	if err := bridge.HandleAntigravityHook(st, root, strings.NewReader(in), &out); err != nil {
		t.Fatalf("SessionStart failed: %v", err)
	}
	if !strings.Contains(out.String(), "systemMessage") || !strings.Contains(out.String(), "T-AUTH") {
		t.Errorf("expected SessionStart systemMessage with T-AUTH warning, got: %s", out.String())
	}

	// 2. PreInvocation -> captures raw lastUserInput to .sen/last_prompt.txt without adding SQLite tables.
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
	if len(tables) != 5 {
		t.Fatalf("SQLite table count must remain strictly 5, got %d", len(tables))
	}

	// 3a. PreToolUse -> deny edits to .sen/
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

	// 3b. PreToolUse -> deny edits to locked test app_test.go
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

	// 3c. PreToolUse -> force_ask on HIGH risk scoped file auth.go
	out.Reset()
	in = `{"hookEventName":"PreToolUse","toolCall":{"name":"edit_file","args":{"Path":"auth.go"}}}`
	if err := bridge.HandleAntigravityHook(st, root, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	preResp = nil
	_ = json.Unmarshal(out.Bytes(), &preResp)
	if preResp["decision"] != "force_ask" {
		t.Fatalf("expected force_ask on HIGH risk task edit, got: %s", out.String())
	}

	// 4. Stop -> decision "continue" when task is OPEN and terminationReason is NO_TOOL_CALL.
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

	// H4a: When .sen/state.db exists and NO task is OPEN, write_file MUST be denied!
	in := `{"hookEventName":"PreToolUse","toolCall":{"name":"write_file","args":{"Path":"main.go"}}}`
	if err := bridge.HandleAntigravityHook(st, root, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"deny"`) || !strings.Contains(out.String(), "No active SEN task") {
		t.Fatalf("expected deny when no active OPEN task, got: %s", out.String())
	}

	// H4b: run_command referencing .sen/config.json (not a sen CLI command) MUST be denied!
	out.Reset()
	in = `{"hookEventName":"PreToolUse","toolCall":{"name":"run_command","args":{"CommandLine":"Get-Content .sen/config.json"}}}`
	_ = bridge.HandleAntigravityHook(st, root, strings.NewReader(in), &out)
	if !strings.Contains(out.String(), `"deny"`) {
		t.Fatalf("expected deny on shell access to .sen/config.json, got: %s", out.String())
	}

	// M1 + M3: PreInvocation scrubs secrets in last_prompt.txt and skips duplicate systemMessage on invocationNum > 1.
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

	// M2: Malformed JSON on SessionStart returns {} (not {"decision":"allow"}), and Stop with fullyIdle=false allows stop.
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

func TestPluginHookResolvesPayloadCwdSeparateFromPluginRoot(t *testing.T) {
	pluginDir := t.TempDir()
	pluginStore, err := storage.Open(filepath.Join(pluginDir, ".sen", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer pluginStore.Close()

	// 1. User workspace WITHOUT .sen/state.db -> must allow edits and NOT contaminate pluginDir!
	uninitUserDir := t.TempDir()
	escUninit := strings.ReplaceAll(uninitUserDir, `\`, `\\`)
	var out bytes.Buffer
	in := `{"hookEventName":"PreToolUse","cwd":"` + escUninit + `","toolCall":{"name":"write_file","args":{"Path":"hello.go"}}}`
	if err := bridge.HandleAntigravityHook(pluginStore, pluginDir, strings.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), `"allow"`) {
		t.Fatalf("expected uninitialized user workspace to allow edits even when pluginDir has state.db, got: %s", out.String())
	}

	out.Reset()
	in = `{"hookEventName":"PreInvocation","invocationNum":1,"cwd":"` + escUninit + `","lastUserInput":"Hello from uninit workspace"}`
	_ = bridge.HandleAntigravityHook(pluginStore, pluginDir, strings.NewReader(in), &out)
	if _, err := os.Stat(filepath.Join(pluginDir, ".sen", "last_prompt.txt")); !os.IsNotExist(err) {
		t.Fatalf("pluginDir must NEVER be contaminated with user workspace last_prompt.txt")
	}

	// 2. User workspace WITH .sen/state.db -> must enforce user workspace's DB & write last_prompt.txt in user workspace!
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

	// Bật cấu hình hợp lệ với express_mode=true kèm chữ ký SHA-256.
	if err := bridge.WriteSignedRuntimeConfig(root, true, "auto"); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	// 1. PreInvocation phát hiện "Làm luôn đi" -> nhắc truyền --express.
	in := `{"hookEventName":"PreInvocation","invocationNum":1,"lastUserInput":"Sửa lỗi svc.go, làm luôn đi nhé"}`
	_ = bridge.HandleAntigravityHook(st, root, strings.NewReader(in), &out)
	if !strings.Contains(out.String(), "--express") {
		t.Fatalf("expected PreInvocation to mention --express for 'làm luôn đi', got: %s", out.String())
	}

	// 2. express_mode=true cho phép ("allow") sửa svc.go dù task thuộc bậc LARGE (BUGFIX), nhưng vẫn "force_ask" trên .env!
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

	// 3. Chặn lệnh ffmpeg ghi đè trực tiếp tư liệu gốc raw/clip.mp4 trong PreToolUse!
	out.Reset()
	in = `{"hookEventName":"PreToolUse","toolCall":{"name":"run_command","args":{"CommandLine":"ffmpeg -y -i raw/clip.mp4 raw/clip.mp4"}}}`
	_ = bridge.HandleAntigravityHook(st, root, strings.NewReader(in), &out)
	if !strings.Contains(out.String(), `"deny"`) {
		t.Fatalf("expected PreToolUse to deny ffmpeg overwriting raw/clip.mp4, got: %s", out.String())
	}

	// 4. Sửa trộm .sen/config.json không cập nhật config.sha256 -> bị từ chối ("deny") ngay!
	_ = os.WriteFile(filepath.Join(root, ".sen", "config.json"), []byte(`{"version":"1.1","express_mode":true}`), 0o644)
	out.Reset()
	in = `{"hookEventName":"PreToolUse","toolCall":{"name":"edit_file","args":{"Path":"svc.go"}}}`
	_ = bridge.HandleAntigravityHook(st, root, strings.NewReader(in), &out)
	if !strings.Contains(out.String(), `"deny"`) || !strings.Contains(out.String(), "tampering") {
		t.Fatalf("expected tampered config.json to be denied, got: %s", out.String())
	}
	_ = bridge.WriteSignedRuntimeConfig(root, true, "video")

	// 5. Khi task chạm ngưỡng 3/3 lần verify lỗi liên tiếp (QUARANTINE), Stop hook trả về "allow" thay vì lặp vô tận!
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

// initLocalGitSkillRepo khởi tạo một kho Git cục bộ thật chứa tệp SKILL.md và trả về mã băm commit HEAD 40 ký tự.
func initLocalGitSkillRepo(t *testing.T, subPath, body string) (string, string) {
	t.Helper()
	repoDir := t.TempDir()
	runGit := func(args ...string) string {
		cmd := exec.Command("git", args...)
		cmd.Dir = repoDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v (%s)", args, err, string(out))
		}
		return strings.TrimSpace(string(out))
	}
	runGit("init")
	runGit("config", "user.email", "test@sen-vn.local")
	runGit("config", "user.name", "Sen Test")
	target := filepath.Join(repoDir, filepath.FromSlash(subPath), "SKILL.md")
	_ = os.MkdirAll(filepath.Dir(target), 0o755)
	_ = os.WriteFile(target, []byte(body), 0o644)
	runGit("add", ".")
	runGit("commit", "-m", "seed skill")
	return repoDir, runGit("rev-parse", "HEAD")
}

// TestPackagesFallbackInstallAndWorkspaceUpdateWithRollback kiểm chứng fallback sang fork khi nguồn chính chết và cập nhật --update có rollback .bak.
func TestPackagesFallbackInstallAndWorkspaceUpdateWithRollback(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	fbRepo, commitV1 := initLocalGitSkillRepo(t, "skills/ffmpeg-skill", "# FFmpeg Skill v1\n")
	deadPrimary := filepath.Join(t.TempDir(), "dead-nonexistent-primary.git")

	overrides := map[string]bridge.PackageURLOverride{
		"ffmpeg-skill": {PrimaryURL: deadPrimary, FallbackURL: fbRepo, PinnedCommit: commitV1},
	}
	ovBytes, _ := json.Marshal(overrides)
	t.Setenv("SEN_PACKAGE_URL_OVERRIDES", string(ovBytes))

	ws := t.TempDir()
	var out bytes.Buffer
	res, err := bridge.InstallCompanionsAndPacks(context.Background(), ws, false, "workflow-diagram,voice-tts,media-video", &out)
	if err != nil {
		t.Fatalf("InstallCompanionsAndPacks failed: %v\n%s", err, out.String())
	}
	if len(res) < 4 {
		t.Fatalf("expected >=4 installed items (archify, voicestudio, ffmpeg-skill, video-subtitles-effects), got %d", len(res))
	}
	gotCommit, err := os.ReadFile(filepath.Join(ws, ".agents", "skills", "ffmpeg-skill", ".sen-commit"))
	if err != nil || strings.TrimSpace(string(gotCommit)) != commitV1 {
		t.Fatalf("expected fallback repo commit %s installed, got %q (err=%v)", commitV1, string(gotCommit), err)
	}

	// Kiểm chứng sen init --update qua mock httptest.NewServer + rollback từ .bak nếu commit mới bị lỗi!
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.0","html_url":"https://github.com/developerfs0110/Sen-VN/releases/tag/v1.2.0"}`))
	}))
	defer srv.Close()
	t.Setenv("SEN_UPDATE_API_URL", srv.URL)

	// Gây lỗi cập nhật bằng mã commit không tồn tại -> phải tự khôi phục bản v1 nguyên vẹn từ .bak!
	badOverrides := map[string]bridge.PackageURLOverride{
		"ffmpeg-skill": {PrimaryURL: deadPrimary, FallbackURL: fbRepo, PinnedCommit: strings.Repeat("f", 40)},
	}
	badBytes, _ := json.Marshal(badOverrides)
	t.Setenv("SEN_PACKAGE_URL_OVERRIDES", string(badBytes))

	out.Reset()
	if err := bridge.RunWorkspaceUpdate(context.Background(), ws, &out); err == nil {
		t.Fatalf("expected RunWorkspaceUpdate to fail on bogus commit and trigger rollback")
	}
	restoredMD, err := os.ReadFile(filepath.Join(ws, ".agents", "skills", "ffmpeg-skill", "SKILL.md"))
	if err != nil || string(restoredMD) != "# FFmpeg Skill v1\n" {
		t.Fatalf("expected .bak atomic rollback to preserve FFmpeg Skill v1, got %q (err=%v)", string(restoredMD), err)
	}

	// Tạo commit v2 hợp lệ trên kho fallback -> RunWorkspaceUpdate nâng cấp thành công và báo có bản v1.2.0!
	_ = os.WriteFile(filepath.Join(fbRepo, "skills", "ffmpeg-skill", "SKILL.md"), []byte("# FFmpeg Skill v2\n"), 0o644)
	cmdAdd := exec.Command("git", "add", ".")
	cmdAdd.Dir = fbRepo
	_ = cmdAdd.Run()
	cmdCom := exec.Command("git", "commit", "-m", "v2")
	cmdCom.Dir = fbRepo
	_ = cmdCom.Run()
	revOut, _ := exec.Command("git", "-C", fbRepo, "rev-parse", "HEAD").Output()
	commitV2 := strings.TrimSpace(string(revOut))

	okOverrides := map[string]bridge.PackageURLOverride{
		"ffmpeg-skill": {PrimaryURL: deadPrimary, FallbackURL: fbRepo, PinnedCommit: commitV2},
	}
	okBytes, _ := json.Marshal(okOverrides)
	t.Setenv("SEN_PACKAGE_URL_OVERRIDES", string(okBytes))

	out.Reset()
	if err := bridge.RunWorkspaceUpdate(context.Background(), ws, &out); err != nil {
		t.Fatalf("RunWorkspaceUpdate v2 failed: %v\n%s", err, out.String())
	}
	if !strings.Contains(out.String(), "v1.2.0") {
		t.Fatalf("expected update check output to report v1.2.0, got:\n%s", out.String())
	}
	updatedMD, _ := os.ReadFile(filepath.Join(ws, ".agents", "skills", "ffmpeg-skill", "SKILL.md"))
	if string(updatedMD) != "# FFmpeg Skill v2\n" {
		t.Fatalf("expected skill updated to v2, got %q", string(updatedMD))
	}
}



