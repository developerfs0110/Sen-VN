package senmvp_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestRound1_SecurityRegressionsEndToEnd(t *testing.T) {
	dir := t.TempDir()
	writeRepoFile(t, dir, "go.mod", "module r1sec\n\ngo 1.22\n")
	writeRepoFile(t, dir, "a.go", "package r1sec\n\nfunc A() int { return 1 }\n")
	writeRepoFile(t, dir, "b.go", "package r1sec\n\nfunc B() int { return 2 }\n")

	mustSenOK(t, dir, "init")
	mustSenOK(t, dir, "intake", "--quote", "Fix A bug", "--req", "Fix A", "--bugfix")
	mustSenOK(t, dir, "intake", "--quote", "Update B", "--req", "Update B")

	// C1: Reject task_id path traversal.
	if out, code := runSen(t, dir, "", nil, "begin", "../../../escape", "--reqs", "R1", "--files", "a.go"); code == 0 {
		t.Fatalf("expected task_id traversal rejected, got 0: %s", out)
	}

	// H2: Reject invalid --kind, and prevent --kind ui from downgrading a --bugfix requirement!
	if out, code := runSen(t, dir, "", nil, "begin", "T-INVALID-KIND", "--reqs", "R2", "--files", "b.go", "--kind", "bogus"); code == 0 {
		t.Fatalf("expected invalid --kind rejected, got 0: %s", out)
	}
	beginOut := mustSenOK(t, dir, "begin", "T1", "--reqs", "R1", "--files", "a.go,a_test.go", "--kind", "ui")
	if !strings.Contains(beginOut, "kind=BUGFIX") || !strings.Contains(beginOut, "risk=HIGH") {
		t.Fatalf("expected --bugfix req to force kind=BUGFIX risk=HIGH, got: %s", beginOut)
	}

	// C2: Reject path-qualified %TEMP%\pytest.cmd spoofed binary.
	fakePytest := filepath.Join(t.TempDir(), "pytest.cmd")
	_ = os.WriteFile(fakePytest, []byte("@echo off\r\necho 1 passed\r\n"), 0o755)
	if out, code := runSen(t, dir, "", nil, "verify", "T1", "--", fakePytest); code == 0 {
		t.Fatalf("expected path-qualified pytest.cmd rejected, got 0: %s", out)
	}

	// H2: Reject `go build` (BUILD strength) for BUGFIX task T1.
	if out, code := runSen(t, dir, "", nil, "verify", "T1", "--", "go", "build", "./..."); code == 0 {
		t.Fatalf("expected BUGFIX verification with BUILD strength rejected, got 0: %s", out)
	}

	// Verify T1 properly with real unit test.
	writeRepoFile(t, dir, "a_test.go", "package r1sec\n\nimport \"testing\"\nfunc TestA(t *testing.T) { if A() != 1 { t.Fatal() } }\n")
	mustSenOK(t, dir, "verify", "T1", "--", "go", "test", "-v", "./...")

	// H5: Reject `sen undo T1` now that T1 is VERIFIED.
	if out, code := runSen(t, dir, "", nil, "undo", "T1"); code == 0 {
		t.Fatalf("expected sen undo on VERIFIED task T1 to be rejected, got 0: %s", out)
	}

	// C3a: Reject reusing task_id T1.
	if out, code := runSen(t, dir, "", nil, "begin", "T1", "--reqs", "R2", "--files", "b.go"); code == 0 {
		t.Fatalf("expected task_id reuse T1 rejected, got 0: %s", out)
	}

	// C3b: Open T2 (b.go only), stealth-edit T1's a.go -> MUST be rejected!
	mustSenOK(t, dir, "begin", "T2", "--reqs", "R2", "--files", "b.go,b_test.go")
	writeRepoFile(t, dir, "b_test.go", "package r1sec\n\nimport \"testing\"\nfunc TestB(t *testing.T) { if B() != 2 { t.Fatal() } }\n")
	writeRepoFile(t, dir, "a.go", "package r1sec\n\nfunc A() int { return 999 }\n")
	out, code := runSen(t, dir, "", nil, "verify", "T2", "--", "go", "test", "-v", "./...")
	if code == 0 || !strings.Contains(out, "a.go") {
		t.Fatalf("expected T2 stealth edit of T1's a.go rejected, got code=%d: %s", code, out)
	}

	// Restore a.go, verify T2 cleanly, and pass sen done!
	writeRepoFile(t, dir, "a.go", "package r1sec\n\nfunc A() int { return 1 }\n")
	mustSenOK(t, dir, "verify", "T2", "--", "go", "test", "-v", "./...")
	mustSenOK(t, dir, "done")
}

func TestRound1_InstallerRejectsHTTPAndMissingHTTPSHash(t *testing.T) {
	tmpInst := t.TempDir()
	if runtime.GOOS == "windows" {
		script, err := filepath.Abs(filepath.Join("scripts", "install-sen.ps1"))
		if err != nil {
			t.Fatal(err)
		}
		cmdHTTP := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script,
			"-Source", "http://example.com/sen.exe", "-ExpectedSha256", strings.Repeat("a", 64), "-InstallDir", tmpInst, "-SkipUserPath")
		if out, err := cmdHTTP.CombinedOutput(); err == nil {
			t.Fatalf("expected http:// source rejected, got success: %s", string(out))
		}
		cmdNoHash := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script,
			"-Source", "https://example.com/sen.exe", "-InstallDir", tmpInst, "-SkipUserPath")
		if out, err := cmdNoHash.CombinedOutput(); err == nil {
			t.Fatalf("expected https:// without -ExpectedSha256 rejected, got success: %s", string(out))
		}
		return
	}

	shScript, err := filepath.Abs(filepath.Join("scripts", "install-sen.sh"))
	if err != nil {
		t.Fatal(err)
	}
	cmdHTTP := exec.Command("sh", shScript, "--source", "http://example.com/sen", "--sha256", strings.Repeat("a", 64), "--install-dir", tmpInst)
	if out, err := cmdHTTP.CombinedOutput(); err == nil {
		t.Fatalf("expected http:// source rejected by install-sen.sh, got success: %s", string(out))
	}
	cmdNoHash := exec.Command("sh", shScript, "--source", "https://example.com/sen", "--install-dir", tmpInst)
	if out, err := cmdNoHash.CombinedOutput(); err == nil {
		t.Fatalf("expected https:// without --sha256 rejected by install-sen.sh, got success: %s", string(out))
	}
}

