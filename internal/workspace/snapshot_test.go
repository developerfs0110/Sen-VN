package workspace_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/developerfs0110/Sen-VN/internal/storage"
	"github.com/developerfs0110/Sen-VN/internal/workspace"
)

func writeTestFile(t *testing.T, root, rel string, data []byte) string {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", full, err)
	}
	if err := os.WriteFile(full, data, 0o644); err != nil {
		t.Fatalf("write %s: %v", full, err)
	}
	return full
}

func TestComputeRiskDeterministic(t *testing.T) {
	root := t.TempDir()

	risk, _ := workspace.ComputeRisk([]string{"ui/button.tsx"}, "UI", root)
	if risk != "LOW" {
		t.Errorf("expected LOW for single UI file, got %s", risk)
	}

	risk, _ = workspace.ComputeRisk([]string{"ui/button.tsx"}, "BUGFIX", root)
	if risk != "HIGH" {
		t.Errorf("expected HIGH for BUGFIX kind, got %s", risk)
	}

	risk, _ = workspace.ComputeRisk([]string{"internal/auth/login.go"}, "FEATURE", root)
	if risk != "HIGH" {
		t.Errorf("expected HIGH for auth path, got %s", risk)
	}

	many := []string{"a.go", "b.go", "c.go", "d.go", "e.go", "f.go"}
	risk, _ = workspace.ComputeRisk(many, "FEATURE", root)
	if risk != "HIGH" {
		t.Errorf("expected HIGH for >5 files, got %s", risk)
	}
}

func TestLargeMediaFileSkipsHashAndCopy(t *testing.T) {
	root := t.TempDir()
	st, err := storage.Open(filepath.Join(root, ".sen", "state.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()

	// Create >10MB dummy video file (10MB + 64 bytes).
	bigSize := int64(10*1024*1024 + 64)
	bigPath := filepath.Join(root, "assets", "demo.mp4")
	if err := os.MkdirAll(filepath.Dir(bigPath), 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := os.Create(bigPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.Truncate(bigSize); err != nil {
		f.Close()
		t.Fatal(err)
	}
	f.Close()

	if err := workspace.CaptureBaseline(st, root); err != nil {
		t.Fatalf("CaptureBaseline: %v", err)
	}
	snaps, err := st.GetSnapshots(workspace.BaselineTaskID)
	if err != nil {
		t.Fatalf("GetSnapshots: %v", err)
	}
	if len(snaps) != 1 {
		t.Fatalf("expected 1 baseline entry, got %d", len(snaps))
	}
	if snaps[0].OrigSHA256 != "" {
		t.Errorf("expected empty OrigSHA256 for >10MB file, got %q", snaps[0].OrigSHA256)
	}
	if snaps[0].OrigSize != bigSize || snaps[0].OrigMtimeNS == 0 {
		t.Errorf("expected size/mtime tracked for >10MB file, got %+v", snaps[0])
	}

	if err := st.SaveTask(storage.Task{
		ID: "T-MEDIA", ReqIDs: []string{"R1"}, Title: "media check",
		Kind: "FEATURE", RiskAuto: "LOW", ScopeFiles: []string{"assets/demo.mp4"}, Status: "OPEN",
	}); err != nil {
		t.Fatal(err)
	}
	if err := workspace.BeginTaskSnapshot(st, root, "T-MEDIA", []string{"assets/demo.mp4"}); err != nil {
		t.Fatalf("BeginTaskSnapshot: %v", err)
	}
	taskSnaps, err := st.GetSnapshots("T-MEDIA")
	if err != nil {
		t.Fatal(err)
	}
	if len(taskSnaps) != 1 || taskSnaps[0].BackupPath != "" {
		t.Errorf("expected >10MB file not to be copied to backup_path, got %+v", taskSnaps)
	}
}

func TestLockedTestsOutOfScopeAndUndoByteForByte(t *testing.T) {
	root := t.TempDir()
	st, err := storage.Open(filepath.Join(root, ".sen", "state.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()

	origMain := []byte("package app\n\nfunc Add(a, b int) int { return a - b }\n")
	origTest := []byte("package app\n\nimport \"testing\"\nfunc TestAdd(t *testing.T) {}\n")
	origUnrelated := []byte("UNTOUCHED_SECRET_CONFIG\n")

	writeTestFile(t, root, "app/calc.go", origMain)
	writeTestFile(t, root, "app/calc_test.go", origTest)
	writeTestFile(t, root, "other/readme.txt", origUnrelated)

	if err := workspace.CaptureBaseline(st, root); err != nil {
		t.Fatalf("CaptureBaseline: %v", err)
	}

	// 1. Reject pre-existing test file in scope_files at BeginTaskSnapshot.
	if err := workspace.BeginTaskSnapshot(st, root, "T-BAD", []string{"app/calc.go", "app/calc_test.go"}); err == nil {
		t.Fatalf("expected BeginTaskSnapshot to reject pre-existing test file in scope")
	}

	// 2. Allow existing non-test file + brand-new TDD test file in scope_files.
	scope := []string{"app\\calc.go", "app/calc_extra_test.go"}
	if err := st.SaveTask(storage.Task{
		ID: "T1", ReqIDs: []string{"R1"}, Title: "fix add",
		Kind: "BUGFIX", RiskAuto: "HIGH", ScopeFiles: []string{"app/calc.go", "app/calc_extra_test.go"}, Status: "OPEN",
	}); err != nil {
		t.Fatal(err)
	}
	if err := workspace.BeginTaskSnapshot(st, root, "T1", scope); err != nil {
		t.Fatalf("BeginTaskSnapshot valid scope failed: %v", err)
	}

	// Modify declared scope file and create declared new test file -> CheckIntegrity PASS.
	writeTestFile(t, root, "app/calc.go", []byte("package app\n\nfunc Add(a, b int) int { return a + b }\n"))
	writeTestFile(t, root, "app/calc_extra_test.go", []byte("package app\n"))
	if err := workspace.CheckIntegrity(st, root, "T1", []string{"app/calc.go", "app/calc_extra_test.go"}); err != nil {
		t.Fatalf("expected clean integrity for in-scope changes, got: %v", err)
	}

	// 3. Tamper with locked pre-existing test file -> CheckIntegrity FAIL.
	writeTestFile(t, root, "app/calc_test.go", []byte("package app // tampered\n"))
	err = workspace.CheckIntegrity(st, root, "T1", []string{"app/calc.go", "app/calc_extra_test.go"})
	if err == nil || !strings.Contains(err.Error(), "calc_test.go") {
		t.Fatalf("expected locked test drift error for calc_test.go, got: %v", err)
	}
	// Restore locked test manually to test out-of-scope detection next.
	writeTestFile(t, root, "app/calc_test.go", origTest)

	// 4. Out-of-scope edit and stealth untracked file -> CheckIntegrity FAIL.
	writeTestFile(t, root, "other/stealth.go", []byte("package other\n"))
	err = workspace.CheckIntegrity(st, root, "T1", []string{"app/calc.go", "app/calc_extra_test.go"})
	if err == nil || !strings.Contains(err.Error(), "other/stealth.go") {
		t.Fatalf("expected out-of-scope error for other/stealth.go, got: %v", err)
	}
	_ = os.Remove(filepath.Join(root, "other", "stealth.go"))

	// 5. UndoTask restores modified calc.go byte-for-byte and removes newly created calc_extra_test.go.
	restored, err := workspace.UndoTask(st, root, "T1")
	if err != nil {
		t.Fatalf("UndoTask failed: %v", err)
	}
	if len(restored) < 2 {
		t.Errorf("expected at least 2 restored/removed paths, got %v", restored)
	}
	gotMain, err := os.ReadFile(filepath.Join(root, "app", "calc.go"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotMain, origMain) {
		t.Errorf("UndoTask did not restore calc.go byte-for-byte:\ngot:  %q\nwant: %q", gotMain, origMain)
	}
	if _, err := os.Stat(filepath.Join(root, "app", "calc_extra_test.go")); !os.IsNotExist(err) {
		t.Errorf("expected newly created calc_extra_test.go to be deleted on undo")
	}
	taskAfter, err := st.GetTask("T1")
	if err != nil {
		t.Fatal(err)
	}
	if taskAfter.Status != "UNDONE" {
		t.Errorf("expected task status UNDONE, got %s", taskAfter.Status)
	}
}

func TestTaskIDTraversalDriveRelativeAndVerifiedUndoRejected(t *testing.T) {
	root := t.TempDir()
	st, err := storage.Open(filepath.Join(root, ".sen", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	writeTestFile(t, root, "a.go", []byte("package a\n"))
	_ = workspace.CaptureBaseline(st, root)

	for _, badID := range []string{"../../../escape", "..\\evil", "__BASELINE__", "__DONE__", "bad/id"} {
		if err := workspace.ValidateTaskID(badID); err == nil {
			t.Errorf("expected invalid task_id %q to fail ValidateTaskID", badID)
		}
		if err := workspace.BeginTaskSnapshot(st, root, badID, []string{"a.go"}); err == nil {
			t.Errorf("expected BeginTaskSnapshot to reject %q", badID)
		}
	}

	for _, badPath := range []string{"C:../../evil.go", "D:secret.txt", "../outside.go", "file.go:ads"} {
		if _, err := workspace.NormalizeRelPath(root, badPath); err == nil {
			t.Errorf("expected NormalizeRelPath to reject %q", badPath)
		}
	}

	// H5: UndoTask must reject already-VERIFIED tasks.
	_ = st.CreateTask(storage.Task{
		ID: "T-VER", ReqIDs: []string{"R1"}, Title: "done", Kind: "FEATURE",
		RiskAuto: "LOW", ScopeFiles: []string{"a.go"}, Status: "VERIFIED",
	})
	if _, err := workspace.UndoTask(st, root, "T-VER"); err == nil {
		t.Fatalf("expected UndoTask to reject VERIFIED task T-VER")
	}
}

func TestMultiTaskAdvanceBaselineBlocksCrossScopeCreepAndUnauthorizedDelete(t *testing.T) {
	root := t.TempDir()
	st, err := storage.Open(filepath.Join(root, ".sen", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	writeTestFile(t, root, "a.go", []byte("package p // v1\n"))
	writeTestFile(t, root, "b.go", []byte("package p // b1\n"))
	if err := workspace.CaptureBaseline(st, root); err != nil {
		t.Fatal(err)
	}

	// T1 modifies a.go and becomes VERIFIED -> AdvanceBaseline locks in a.go v2.
	_ = st.CreateTask(storage.Task{
		ID: "T1", ReqIDs: []string{"R1"}, Title: "t1", Kind: "FEATURE",
		RiskAuto: "LOW", ScopeFiles: []string{"a.go"}, Status: "OPEN",
	})
	if err := workspace.BeginTaskSnapshot(st, root, "T1", []string{"a.go"}); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, "a.go", []byte("package p // v2\n"))
	_ = st.UpdateTaskStatus("T1", "VERIFIED")
	if err := workspace.AdvanceBaseline(st, root, []string{"a.go"}); err != nil {
		t.Fatalf("AdvanceBaseline failed: %v", err)
	}

	// T2 declares ONLY b.go, then stealthily modifies a.go -> CheckIntegrity MUST reject!
	_ = st.CreateTask(storage.Task{
		ID: "T2", ReqIDs: []string{"R1"}, Title: "t2", Kind: "FEATURE",
		RiskAuto: "LOW", ScopeFiles: []string{"b.go"}, Status: "OPEN",
	})
	if err := workspace.BeginTaskSnapshot(st, root, "T2", []string{"b.go"}); err != nil {
		t.Fatal(err)
	}
	writeTestFile(t, root, "b.go", []byte("package p // b2\n"))
	writeTestFile(t, root, "a.go", []byte("package p // STEALTH TAMPER IN T2\n"))

	err = workspace.CheckIntegrity(st, root, "T2", []string{"b.go"})
	if err == nil || !strings.Contains(err.Error(), "a.go") {
		t.Fatalf("expected CheckIntegrity on T2 to reject stealth edit to T1's a.go, got: %v", err)
	}

	// Restore a.go to v2; test that deleting b.go WITHOUT allowDelete is rejected!
	writeTestFile(t, root, "a.go", []byte("package p // v2\n"))
	_ = os.Remove(filepath.Join(root, "b.go"))
	if err := workspace.CheckIntegrity(st, root, "T2", []string{"b.go"}); err == nil {
		t.Fatalf("expected deleting scoped file b.go without --allow-delete to be rejected")
	}
}

func TestSymlinkOrNTFSJunctionEscapeRejected(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	_ = os.WriteFile(filepath.Join(outside, "secret.txt"), []byte("OUTSIDE_SECRET"), 0o644)

	st, err := storage.Open(filepath.Join(root, ".sen", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	writeTestFile(t, root, "ok.go", []byte("package ok\n"))
	if err := workspace.CaptureBaseline(st, root); err != nil {
		t.Fatal(err)
	}

	linkDir := filepath.Join(root, "escape_link")
	if runtime.GOOS == "windows" {
		if out, err := exec.Command("cmd", "/c", "mklink", "/J", linkDir, outside).CombinedOutput(); err != nil {
			t.Skipf("mklink /J unavailable: %v (%s)", err, string(out))
		}
		defer os.Remove(linkDir)
	} else {
		if err := os.Symlink(outside, linkDir); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
	}

	if err := workspace.CheckIntegrity(st, root, "", nil); err == nil {
		t.Fatalf("expected CheckIntegrity to reject NTFS junction / symlink inside workspace")
	}
	if err := workspace.BeginTaskSnapshot(st, root, "T-SYM", []string{"escape_link/secret.txt"}); err == nil {
		t.Fatalf("expected BeginTaskSnapshot to reject path traversing junction/symlink")
	}
}
