// Tệp e2e_v11_test.go kiểm thử tích hợp đầu-cuối (E2E) toàn bộ 9 hạng mục nâng cấp của Sen-VN v1.1.0 trên tệp thực thi sen.
package senmvp_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/developerfs0110/Sen-VN/internal/storage"
)

// TestV11_ThreeTierSizingExpressModeAndTamperGuard kiểm chứng định cỡ SMALL/MEDIUM/LARGE, --express và chống sửa trộm config.json.
func TestV11_ThreeTierSizingExpressModeAndTamperGuard(t *testing.T) {
	dir := t.TempDir()
	writeRepoFile(t, dir, "go.mod", "module v11exp\n\ngo 1.22\n")
	for _, f := range []string{"a.go", "b.go", "c.go", "d.go"} {
		writeRepoFile(t, dir, f, "package v11exp\n")
	}
	mustSenOK(t, dir, "init", "--agy-hooks")

	// PreInvocation với cụm từ "Làm luôn đi" tự động bật express_mode có chữ ký SHA-256.
	hookOut, code := runSen(t, dir, `{"hookEventName":"PreInvocation","lastUserInput":"Làm luôn đi anh Gấu"}`, nil, "brief", "--hook")
	if code != 0 || !strings.Contains(hookOut, "express=on") {
		t.Fatalf("expected PreInvocation to enable express=on, got code=%d out=%s", code, hookOut)
	}

	mustSenOK(t, dir, "intake", "--quote", "Add 2 files", "--req", "Small change")
	mustSenOK(t, dir, "intake", "--quote", "Add 4 files", "--req", "Medium change")
	mustSenOK(t, dir, "intake", "--quote", "Delete legacy", "--req", "Destructive change")

	// 1) SMALL (2 tệp), sau đó undo để thử MEDIUM (4 tệp).
	smallOut := mustSenOK(t, dir, "begin", "T-SMALL", "--reqs", "R1", "--files", "a.go,b.go")
	if !strings.Contains(smallOut, "risk=SMALL") {
		t.Fatalf("expected risk=SMALL for 2 files, got: %s", smallOut)
	}
	mustSenOK(t, dir, "undo", "T-SMALL")

	medOut := mustSenOK(t, dir, "begin", "T-MED", "--reqs", "R2", "--files", "a.go,b.go,c.go,d.go")
	if !strings.Contains(medOut, "risk=MEDIUM") {
		t.Fatalf("expected risk=MEDIUM for 4 files, got: %s", medOut)
	}
	// Trong chế độ express=on, chỉnh sửa tệp khai báo của MEDIUM được tự động allow (không hỏi lại).
	allowMed, _ := runSen(t, dir, `{"hookEventName":"PreToolUse","toolCall":{"name":"edit_file","args":{"Path":"a.go"}}}`, nil, "brief", "--hook")
	if !strings.Contains(allowMed, `"allow"`) {
		t.Fatalf("expected express=on to allow MEDIUM scope edit, got: %s", allowMed)
	}
	mustSenOK(t, dir, "undo", "T-MED")

	// 2) LARGE có --allow-delete bắt buộc force_ask ngay cả khi đang bật express=on!
	lgOut := mustSenOK(t, dir, "begin", "T-DEL", "--reqs", "R3", "--files", "d.go", "--allow-delete")
	if !strings.Contains(lgOut, "risk=LARGE") {
		t.Fatalf("expected risk=LARGE for --allow-delete, got: %s", lgOut)
	}
	askDel, _ := runSen(t, dir, `{"hookEventName":"PreToolUse","toolCall":{"name":"edit_file","args":{"Path":"d.go"}}}`, nil, "brief", "--hook")
	if !strings.Contains(askDel, `"force_ask"`) {
		t.Fatalf("expected force_ask on --allow-delete even in express mode, got: %s", askDel)
	}

	// 3) Sửa trộm .sen/config.json không cập nhật .sen/config.sha256 -> bị chặn ngay ở cả Hook ("deny") và CLI!
	writeRepoFile(t, dir, ".sen/config.json", `{"version":"1.1","express_mode":true}`)
	tamperHook, _ := runSen(t, dir, `{"hookEventName":"PreToolUse","toolCall":{"name":"edit_file","args":{"Path":"d.go"}}}`, nil, "brief", "--hook")
	if !strings.Contains(tamperHook, `"deny"`) || !strings.Contains(tamperHook, "tampering") {
		t.Fatalf("expected tampered config.json to trigger deny warning, got: %s", tamperHook)
	}
	if out, c := runSen(t, dir, "", nil, "status"); c == 0 || !strings.Contains(out, "tampering") {
		t.Fatalf("expected CLI status to reject tampered config.json, got code=%d out=%s", c, out)
	}
}

// TestV11_BoundedQuarantineThreeRoundsStopHookAndApproveUnlock kiểm chứng chốt chặn <= 3 vòng lỗi, Stop hook allow và mở khóa --approve.
func TestV11_BoundedQuarantineThreeRoundsStopHookAndApproveUnlock(t *testing.T) {
	dir := t.TempDir()
	writeRepoFile(t, dir, "go.mod", "module v11quar\n\ngo 1.22\n")
	writeRepoFile(t, dir, "calc.go", "package v11quar\n\nfunc Add(a, b int) int { return a - b }\n")

	mustSenOK(t, dir, "init")
	mustSenOK(t, dir, "intake", "--quote", "Fix Add function", "--req", "Add returns a+b", "--bugfix")
	mustSenOK(t, dir, "begin", "T-QUAR", "--reqs", "R1", "--files", "calc.go,calc_test.go")
	writeRepoFile(t, dir, "calc_test.go", "package v11quar\n\nimport \"testing\"\nfunc TestAdd(t *testing.T) { if Add(2, 3) != 5 { t.Fatal(\"wrong\") } }\n")

	// Chạy verify thất bại 3 vòng liên tiếp; vòng 3 phải gắn nhãn [QUARANTINE 3/3].
	var thirdOut string
	for i := 1; i <= 3; i++ {
		out, code := runSen(t, dir, "", nil, "verify", "T-QUAR", "--", "go", "test", "-v", "./...")
		if code == 0 {
			t.Fatalf("round %d expected failure, got exit 0", i)
		}
		thirdOut = out
	}
	if !strings.Contains(thirdOut, "[QUARANTINE 3/3]") {
		t.Fatalf("expected 3rd failure to report [QUARANTINE 3/3], got:\n%s", thirdOut)
	}

	// Vòng thứ 4 bị chặn tức thì trước khi gọi tiến trình con.
	fourthOut, fourthCode := runSen(t, dir, "", nil, "verify", "T-QUAR", "--", "go", "test", "-v", "./...")
	if fourthCode == 0 || !strings.Contains(fourthOut, "bounded repair loop exceeded") {
		t.Fatalf("expected 4th attempt blocked by quarantine, got code=%d:\n%s", fourthCode, fourthOut)
	}

	// Stop hook trả về allow khi task đang ở QUARANTINE để tránh vòng lặp vô hạn với Agent.
	stopOut, _ := runSen(t, dir, `{"hookEventName":"Stop"}`, nil, "brief", "--hook")
	if !strings.Contains(stopOut, `"allow"`) {
		t.Fatalf("expected Stop hook to allow exit during QUARANTINE, got: %s", stopOut)
	}

	// Người dùng duyệt `sen status --approve` để mở khóa, sửa code đúng và hoàn tất `sen done`.
	unlockOut := mustSenOK(t, dir, "status", "--approve")
	if !strings.Contains(unlockOut, "Unlocked repair loop for task T-QUAR") {
		t.Fatalf("expected status --approve to unlock T-QUAR, got:\n%s", unlockOut)
	}
	writeRepoFile(t, dir, "calc.go", "package v11quar\n\nfunc Add(a, b int) int { return a + b }\n")
	mustSenOK(t, dir, "verify", "T-QUAR", "--", "go", "test", "-v", "./...")
	doneOut := mustSenOK(t, dir, "done")
	if !strings.Contains(doneOut, "Risk Tier") || !strings.Contains(doneOut, "LARGE") || !strings.Contains(doneOut, "TEST (1)") {
		t.Fatalf("unexpected v1.1 delivery report output:\n%s", doneOut)
	}
}

// TestV11_VideoRawMediaLockFFmpegPreToolUseAndFFprobeReferee kiểm chứng khóa tệp raw >10MB, chặn ffmpeg ghi đè và trọng tài ffprobe.
func TestV11_VideoRawMediaLockFFmpegPreToolUseAndFFprobeReferee(t *testing.T) {
	dir := t.TempDir()
	rawRel := "raw/master_clip.mp4"
	rawAbs := filepath.Join(dir, filepath.FromSlash(rawRel))
	if err := os.MkdirAll(filepath.Dir(rawAbs), 0o755); err != nil {
		t.Fatal(err)
	}
	// Tạo tệp media gốc > 10MB (11 MiB) để kiểm chứng đối soát size + mtime_ns tốc độ cao.
	if err := os.WriteFile(rawAbs, make([]byte, 11*1024*1024), 0o644); err != nil {
		t.Fatal(err)
	}
	origInfo, err := os.Stat(rawAbs)
	if err != nil {
		t.Fatal(err)
	}
	origMtime := origInfo.ModTime()
	writeRepoFile(t, dir, "edits/out.mp4", "placeholder")

	initOut := mustSenOK(t, dir, "init")
	if !strings.Contains(initOut, "profile=video") {
		t.Fatalf("expected auto-detected profile=video, got: %s", initOut)
	}
	mustSenOK(t, dir, "intake", "--quote", "Render highlight reel", "--req", "Export 1080p MP4 with audio")

	// Chặn khai báo sửa tệp gốc trong raw/ khi gọi sen begin.
	if out, c := runSen(t, dir, "", nil, "begin", "T-BADRAW", "--reqs", "R1", "--files", rawRel); c == 0 || !strings.Contains(out, "locked (immutable)") {
		t.Fatalf("expected raw media rejected in sen begin, got code=%d out=%s", c, out)
	}
	// Chặn lệnh shell ffmpeg ghi đè trực tiếp lên raw/master_clip.mp4 tại PreToolUse hook.
	ffHook, _ := runSen(t, dir, `{"hookEventName":"PreToolUse","toolCall":{"name":"run_command","args":{"CommandLine":"ffmpeg -y -i raw/master_clip.mp4 raw/master_clip.mp4"}}}`, nil, "brief", "--hook")
	if !strings.Contains(ffHook, `"deny"`) {
		t.Fatalf("expected PreToolUse to deny destructive ffmpeg on raw media, got: %s", ffHook)
	}

	mustSenOK(t, dir, "begin", "T-VID", "--reqs", "R1", "--files", "edits/out.mp4")
	mockBinDir := buildMockFFprobeBinary(t)
	envWithFFprobe := []string{"PATH=" + mockBinDir + string(os.PathListSeparator) + os.Getenv("PATH")}

	// Tệp xuất bị hỏng (0 stream) dù ffprobe trả mã thoát 0 vẫn phải bị Sen từ chối!
	writeRepoFile(t, dir, "edits/out.mp4", "CORRUPT_EMPTY")
	if out, c := runSen(t, dir, "", envWithFFprobe, "verify", "T-VID", "--", "ffprobe", "-v", "error", "-print_format", "json", "edits/out.mp4"); c == 0 || !strings.Contains(out, "invalid media output") {
		t.Fatalf("expected corrupt media rejected by ffprobe referee, got code=%d out=%s", c, out)
	}

	// Tệp xuất hợp lệ (1 luồng video 1080p + 1 luồng audio = 2 streams) vượt qua sen verify.
	writeRepoFile(t, dir, "edits/out.mp4", "VALID_MP4_2_STREAMS")
	verOut, verCode := runSen(t, dir, "", envWithFFprobe, "verify", "T-VID", "--", "ffprobe", "-v", "error", "-print_format", "json", "edits/out.mp4")
	if verCode != 0 || !strings.Contains(verOut, "strength=MEDIA tests_ran=2") {
		t.Fatalf("expected valid ffprobe verification to pass with MEDIA/2, got code=%d out=%s", verCode, verOut)
	}

	// Chạm mtime tệp raw >10MB ngoài luồng phải bị phát hiện tại sen done; khôi phục mtime thì sen done thành công!
	future := origMtime.Add(10 * time.Second)
	_ = os.Chtimes(rawAbs, future, future)
	if out, c := runSen(t, dir, "", nil, "done"); c == 0 || !strings.Contains(out, "locked raw/master media file modified") {
		t.Fatalf("expected >10MB raw media mtime drift rejected at done, got code=%d out=%s", c, out)
	}
	_ = os.Chtimes(rawAbs, origMtime, origMtime)
	doneOut := mustSenOK(t, dir, "done")
	if !strings.Contains(doneOut, "MEDIA (2)") || !strings.Contains(doneOut, "Domain Profile:** `video`") {
		t.Fatalf("expected MEDIA (2) and video profile in delivery report, got:\n%s", doneOut)
	}
}

// buildMockFFprobeBinary biên dịch binary ffprobe giả lập đặt trong .cache/ (ngoài workspace và ngoài %TEMP%) để test E2E đa nền tảng.
func buildMockFFprobeBinary(t *testing.T) string {
	t.Helper()
	cacheRoot, err := filepath.Abs(".cache")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(cacheRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	binDir, err := os.MkdirTemp(cacheRoot, "mock-ffprobe-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(binDir) })

	srcDir := t.TempDir()
	srcFile := filepath.Join(srcDir, "main.go")
	srcCode := `package main
import ("fmt"; "os"; "strings")
func main() {
	target := os.Args[len(os.Args)-1]
	b, _ := os.ReadFile(target)
	if strings.Contains(string(b), "CORRUPT") {
		fmt.Println("{\"streams\":[],\"format\":{\"duration\":\"0\"}}")
		return
	}
	fmt.Println("{\"streams\":[{\"codec_type\":\"video\",\"codec_name\":\"h264\",\"width\":1920,\"height\":1080,\"r_frame_rate\":\"30/1\"},{\"codec_type\":\"audio\",\"codec_name\":\"aac\",\"channels\":2,\"sample_rate\":\"48000\"}],\"format\":{\"duration\":\"12.500\"}}")
}
`
	if err := os.WriteFile(srcFile, []byte(srcCode), 0o644); err != nil {
		t.Fatal(err)
	}
	binName := "ffprobe"
	if runtime.GOOS == "windows" {
		binName = "ffprobe.exe"
	}
	outBin := filepath.Join(binDir, binName)
	cmd := exec.Command("go", "build", "-o", outBin, srcFile)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build mock ffprobe failed: %v\n%s", err, string(out))
	}
	return binDir
}

// TestV11_DocsDomainCompanionAutoInstallForkFallbackAndUpdate kiểm chứng profile docs, cài gói tự động + fork fallback và --update.
func TestV11_DocsDomainCompanionAutoInstallForkFallbackAndUpdate(t *testing.T) {
	dir := t.TempDir()
	writeRepoFile(t, dir, "docs/handbook.md", "# Team Handbook v1\n")

	// Tạo kho Git cục bộ đóng vai trò bản fork dự phòng developerfs0110 với 2 commit v1 và v2.
	forkRepo, commitV1, commitV2 := createLocalCompanionGitRepo(t)
	ovV1, _ := json.Marshal(map[string]map[string]string{
		"*": {
			"primaryUrl":   filepath.Join(t.TempDir(), "unreachable-upstream.git"),
			"fallbackUrl":  forkRepo,
			"pinnedCommit": commitV1,
		},
	})

	initOut, initCode := runSen(t, dir, "", []string{"SEN_PACKAGE_URL_OVERRIDES=" + string(ovV1)},
		"init", "--install-companions", "--packs", "media-video,voice-tts,workflow-diagram")
	if initCode != 0 {
		t.Fatalf("init --install-companions --packs failed (code=%d):\n%s", initCode, initOut)
	}
	if !strings.Contains(initOut, "profile=docs") || !strings.Contains(initOut, "via fallback") {
		t.Fatalf("expected profile=docs and fallback install in output, got:\n%s", initOut)
	}
	for _, wantSkill := range []string{
		".agents/plugins/superpowers/SKILL.md",
		".agents/skills/sen-dialectical/SKILL.md",
		".agents/skills/sen-skill-capture/SKILL.md",
		".agents/skills/video-subtitles-effects/SKILL.md",
		".agents/skills/voicestudio/SKILL.md",
		".agents/skills/archify/SKILL.md",
	} {
		if _, err := os.Stat(filepath.Join(dir, filepath.FromSlash(wantSkill))); err != nil {
			t.Fatalf("missing expected installed skill %s: %v", wantSkill, err)
		}
	}

	// Chạy lại --install-companions phải báo [exists] chứ không ghi đè!
	reInitOut, _ := runSen(t, dir, "", []string{"SEN_PACKAGE_URL_OVERRIDES=" + string(ovV1)}, "init", "--install-companions")
	if !strings.Contains(reInitOut, "[exists]") {
		t.Fatalf("expected re-run to skip existing companions with [exists], got:\n%s", reInitOut)
	}

	// Kiểm chứng `sen init --update` đối chiếu Releases API + nâng cấp gói từ commitV1 -> commitV2!
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"tag_name":"v1.2.0","html_url":"https://github.com/developerfs0110/Sen-VN/releases/tag/v1.2.0"}`))
	}))
	defer srv.Close()
	ovV2, _ := json.Marshal(map[string]map[string]string{
		"*": {"primaryUrl": forkRepo, "fallbackUrl": forkRepo, "pinnedCommit": commitV2},
	})
	updOut, updCode := runSen(t, dir, "", []string{
		"SEN_UPDATE_API_URL=" + srv.URL,
		"SEN_PACKAGE_URL_OVERRIDES=" + string(ovV2),
	}, "init", "--update")
	if updCode != 0 || !strings.Contains(updOut, "v1.2.0") || !strings.Contains(updOut, "Updated package superpowers") {
		t.Fatalf("expected sen init --update to report v1.2.0 and update packages, got code=%d:\n%s", updCode, updOut)
	}

	// Hoàn thành trọn vòng đời dự án thuần tài liệu (docs) không cần bộ test Go/Node và xác nhận đúng 5 bảng SQLite!
	mustSenOK(t, dir, "intake", "--quote", "Add onboarding section", "--req", "Update handbook")
	mustSenOK(t, dir, "begin", "T-DOC", "--reqs", "R1", "--files", "docs/handbook.md")
	writeRepoFile(t, dir, "docs/handbook.md", "# Team Handbook v1\n\n## Onboarding\nWelcome!\n")
	mustSenOK(t, dir, "verify", "T-DOC", "--", "go", "version")
	mustSenOK(t, dir, "done")

	st, err := storage.Open(filepath.Join(dir, ".sen", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	tables, err := st.UserTableNames()
	wantTables := []string{"evidence", "requirements", "rules", "snapshots", "ssds_governance", "ssds_phases", "ssds_project", "tasks"}
	if err != nil || !reflect.DeepEqual(tables, wantTables) {
		t.Fatalf("expected exact 8 canonical SQLite tables, got %v (err=%v)", tables, err)
	}
}

// createLocalCompanionGitRepo khởi tạo kho Git cục bộ chứa cấu trúc skills/security-audit, skills/vn-law-check và SKILL.md với 2 commit.
func createLocalCompanionGitRepo(t *testing.T) (string, string, string) {
	t.Helper()
	repo := t.TempDir()
	runGit := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repo
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=SenTest", "GIT_AUTHOR_EMAIL=sen@example.com",
			"GIT_COMMITTER_NAME=SenTest", "GIT_COMMITTER_EMAIL=sen@example.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, string(out))
		}
		return strings.TrimSpace(string(out))
	}
	runGit("init")
	runGit("config", "core.autocrlf", "false")
	writeRepoFile(t, repo, "SKILL.md", "# Superpowers Fork v1\n")
	writeRepoFile(t, repo, "skills/security-audit/SKILL.md", "# Security Audit Fork v1\n")
	writeRepoFile(t, repo, "plugins/vn-law-check/skills/vn-law-check/SKILL.md", "# VN Law Check Fork v1\n")
	writeRepoFile(t, repo, "skills/ffmpeg-skill/SKILL.md", "# FFmpeg Skill Fork v1\n")
	runGit("add", ".")
	runGit("commit", "-m", "v1")
	c1 := runGit("rev-parse", "HEAD")

	writeRepoFile(t, repo, "SKILL.md", "# Superpowers Fork v2\n")
	writeRepoFile(t, repo, "skills/security-audit/SKILL.md", "# Security Audit Fork v2\n")
	writeRepoFile(t, repo, "plugins/vn-law-check/skills/vn-law-check/SKILL.md", "# VN Law Check Fork v2\n")
	writeRepoFile(t, repo, "skills/ffmpeg-skill/SKILL.md", "# FFmpeg Skill Fork v2\n")
	runGit("add", ".")
	runGit("commit", "-m", "v2")
	c2 := runGit("rev-parse", "HEAD")
	return repo, c1, c2
}
