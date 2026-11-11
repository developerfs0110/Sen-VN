// Tệp bridge_packages_test.go kiểm thử cài đặt gói mở rộng qua fork dự phòng và cập nhật nguyên tử có rollback .bak.
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
)

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

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tag_name":"v1.3.0","html_url":"https://github.com/developerfs0110/Sen-VN/releases/tag/v1.3.0"}`))
	}))
	defer srv.Close()
	t.Setenv("SEN_UPDATE_API_URL", srv.URL)

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
	if !strings.Contains(out.String(), "v1.3.0") {
		t.Fatalf("expected update check output to report v1.3.0, got:\n%s", out.String())
	}
	updatedMD, _ := os.ReadFile(filepath.Join(ws, ".agents", "skills", "ffmpeg-skill", "SKILL.md"))
	if string(updatedMD) != "# FFmpeg Skill v2\n" {
		t.Fatalf("expected skill updated to v2, got %q", string(updatedMD))
	}
}

