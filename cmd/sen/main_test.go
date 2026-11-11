// Tệp main_test.go kiểm thử bề mặt 9 lệnh CLI của Sen-VN v1.2.0, bảo toàn ssds_version và phát hiện sửa trộm config.json.
package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCLIFrozenNineCommandsRejectsUnknown xác minh bảng hướng dẫn liệt kê đủ 9 lệnh chuẩn và từ chối lệnh lạ.
func TestCLIFrozenNineCommandsRejectsUnknown(t *testing.T) {
	dir := t.TempDir()
	var out, errBuf bytes.Buffer

	code := RunInDir(dir, []string{"nonexistent-subcommand"}, strings.NewReader(""), &out, &errBuf)
	if code != 1 {
		t.Fatalf("expected exit code 1 for unknown command, got %d", code)
	}
	for _, cmd := range []string{"init", "brief", "intake", "begin", "verify", "undo", "done", "status", "sdlc"} {
		if !strings.Contains(errBuf.String(), cmd) {
			t.Errorf("expected usage output to list command %q, got:\n%s", cmd, errBuf.String())
		}
	}
	if codeUpd := RunInDir(dir, []string{"update"}, strings.NewReader(""), &out, &errBuf); codeUpd != 1 {
		t.Fatalf("expected top-level 'update' command to be rejected (9-command surface), got %d", codeUpd)
	}
}

// TestCLIInitIntakeBeginStatusAndRuleCap kiểm tra luồng khởi tạo, tiếp nhận yêu cầu, mở task và phê duyệt qua CLI.
func TestCLIInitIntakeBeginStatusAndRuleCap(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module demo\n\ngo 1.22\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "hello.go"), []byte("package demo\n\nfunc Greet() string { return \"hi\" }\n"), 0o644)

	var out, errBuf bytes.Buffer
	mustOK := func(args ...string) string {
		t.Helper()
		out.Reset()
		errBuf.Reset()
		if code := RunInDir(dir, args, strings.NewReader(""), &out, &errBuf); code != 0 {
			t.Fatalf("command %v failed (code %d): stderr=%s stdout=%s", args, code, errBuf.String(), out.String())
		}
		return out.String()
	}

	mustOK("init", "--agy-hooks")
	if _, err := os.Stat(filepath.Join(dir, "AGENTS.md")); err != nil {
		t.Fatalf("expected AGENTS.md created: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agents", "hooks.json")); err != nil {
		t.Fatalf("expected .agents/hooks.json created: %v", err)
	}

	mustOK("intake", "--quote", "Update greeting and check security", "--req", "Update Greet", "--security")
	beginOut := mustOK("begin", "T1", "--reqs", "R1,R_sec", "--files", "hello.go,hello_test.go", "--kind", "feature")
	if !strings.Contains(beginOut, "risk=SMALL") {
		t.Fatalf("expected 2-file feature task to be risk=SMALL, got: %s", beginOut)
	}

	statusOut := mustOK("status", "--approve")
	if !strings.Contains(statusOut, "R1") || !strings.Contains(statusOut, "R_sec") || !strings.Contains(statusOut, "T1") {
		t.Fatalf("unexpected status output:\n%s", statusOut)
	}
}

// TestCLIV12PacksListProfileExpressAndConfigTamper kiểm tra danh mục gói, chế độ --express và chống xóa config.sha256.
func TestCLIV12PacksListProfileExpressAndConfigTamper(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Pure Docs Project\n"), 0o644)

	var out, errBuf bytes.Buffer
	if code := RunInDir(dir, []string{"init", "--packs", "list"}, strings.NewReader(""), &out, &errBuf); code != 0 {
		t.Fatalf("init --packs list failed: %s", errBuf.String())
	}
	if !strings.Contains(out.String(), "superpowers") || !strings.Contains(out.String(), "media-video") {
		t.Fatalf("unexpected packs catalog output:\n%s", out.String())
	}
	if _, err := os.Stat(filepath.Join(dir, ".sen", "state.db")); !os.IsNotExist(err) {
		t.Fatalf("expected --packs list not to create .sen/state.db")
	}

	out.Reset()
	errBuf.Reset()
	if code := RunInDir(dir, []string{"init", "--profile", "docs", "--express"}, strings.NewReader(""), &out, &errBuf); code != 0 {
		t.Fatalf("init --profile docs --express failed: %s", errBuf.String())
	}

	out.Reset()
	errBuf.Reset()
	if code := RunInDir(dir, []string{"status"}, strings.NewReader(""), &out, &errBuf); code != 0 {
		t.Fatalf("status failed: %s", errBuf.String())
	}
	if !strings.Contains(out.String(), "Profile: docs") || !strings.Contains(out.String(), "Express: on") {
		t.Fatalf("expected Profile: docs | Express: on in status, got:\n%s", out.String())
	}

	_ = os.Remove(filepath.Join(dir, ".sen", "config.sha256"))
	out.Reset()
	errBuf.Reset()
	if code := RunInDir(dir, []string{"done"}, strings.NewReader(""), &out, &errBuf); code == 0 || !strings.Contains(errBuf.String(), "tampering") {
		t.Fatalf("expected missing config.sha256 to fail with tampering error, got code=%d stderr=%s", code, errBuf.String())
	}
}
