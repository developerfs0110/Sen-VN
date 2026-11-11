package verify_test

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/developerfs0110/Sen-VN/internal/storage"
	"github.com/developerfs0110/Sen-VN/internal/verify"
	"github.com/developerfs0110/Sen-VN/internal/workspace"
)

func TestParseTestRunAdaptersRejectZeroTests(t *testing.T) {
	// 1. go test with [no test files] -> rejected
	strength, count, err := verify.ParseTestRun("go", []string{"test", "./..."}, "?   \tsample/empty\t[no test files]\n", 0)
	if strength != "TEST" || err == nil || count != 0 {
		t.Fatalf("expected go test [no test files] rejection, got strength=%s count=%d err=%v", strength, count, err)
	}

	// 2. go test with [no tests to run] -> rejected
	strength, count, err = verify.ParseTestRun("go.exe", []string{"test", "-v", "./..."}, "testing: warning: no tests to run\nPASS\nok  \tsample/pkg\t0.012s [no tests to run]\n", 0)
	if strength != "TEST" || err == nil || count != 0 {
		t.Fatalf("expected go test [no tests to run] rejection, got strength=%s count=%d err=%v", strength, count, err)
	}

	// 3. go test with real passing tests (+ an entrypoint package with [no test files]) -> accepted
	goPassOut := "?   \tsample/cmd\t[no test files]\n=== RUN   TestAdd\n--- PASS: TestAdd (0.00s)\n=== RUN   TestSub\n--- PASS: TestSub (0.00s)\nPASS\nok  \tsample/math\t0.015s\n"
	strength, count, err = verify.ParseTestRun("go", []string{"test", "-v", "./..."}, goPassOut, 0)
	if strength != "TEST" || err != nil || count != 2 {
		t.Fatalf("expected 2 passing go tests, got strength=%s count=%d err=%v", strength, count, err)
	}

	// 4. pytest exit code 5 -> rejected
	strength, count, err = verify.ParseTestRun("pytest", nil, "collected 0 items\n", 5)
	if strength != "TEST" || err == nil || count != 0 {
		t.Fatalf("expected pytest exit 5 rejection, got strength=%s count=%d err=%v", strength, count, err)
	}

	// 5. pytest 3 passed -> accepted
	strength, count, err = verify.ParseTestRun("pytest", []string{"-q"}, "=== 3 passed in 0.12s ===\n", 0)
	if strength != "TEST" || err != nil || count != 3 {
		t.Fatalf("expected pytest 3 passed, got strength=%s count=%d err=%v", strength, count, err)
	}

	// 6. jest --passWithNoTests (0 passed) -> rejected
	strength, count, err = verify.ParseTestRun("jest", []string{"--passWithNoTests"}, "No tests found, exiting with code 0\n", 0)
	if strength != "TEST" || err == nil || count != 0 {
		t.Fatalf("expected jest 0 tests rejection, got strength=%s count=%d err=%v", strength, count, err)
	}

	// 7. jest 2 passed -> accepted
	strength, count, err = verify.ParseTestRun("jest", nil, "Test Suites: 1 passed, 1 total\nTests:       2 passed, 2 total\n", 0)
	if strength != "TEST" || err != nil || count != 2 {
		t.Fatalf("expected jest 2 passed, got strength=%s count=%d err=%v", strength, count, err)
	}

	// 8. go build -> BUILD strength, zero tests allowed
	strength, count, err = verify.ParseTestRun("go", []string{"build", "./..."}, "", 0)
	if strength != "BUILD" || err != nil || count != 0 {
		t.Fatalf("expected BUILD strength, got strength=%s count=%d err=%v", strength, count, err)
	}
}

func TestRunTaskVerificationScrubsEnvOutputAndDoneGateEnforcesCoverage(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module sample\n\ngo 1.22\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "calc.go"), []byte("package sample\n\nfunc Add(a, b int) int { return a + b }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	st, err := storage.Open(filepath.Join(root, ".sen", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// Register R1 and R2; we will verify R1 first and confirm DoneGate blocks uncovered R2.
	if _, err := st.AddRequirement("", "Add calc and sub", "Implement Add", false); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AddRequirement("", "Add calc and sub", "Implement Sub", false); err != nil {
		t.Fatal(err)
	}
	if err := workspace.CaptureBaseline(st, root); err != nil {
		t.Fatal(err)
	}

	// Disallowed binary must be rejected.
	if err := st.SaveTask(storage.Task{
		ID: "T1", ReqIDs: []string{"R1"}, Title: "test add", Kind: "FEATURE",
		RiskAuto: "LOW", ScopeFiles: []string{"calc_test.go"}, Status: "OPEN",
	}); err != nil {
		t.Fatal(err)
	}
	if err := workspace.BeginTaskSnapshot(st, root, "T1", []string{"calc_test.go"}); err != nil {
		t.Fatal(err)
	}

	testSrc := `package sample

import (
	"fmt"
	"os"
	"testing"
)

func TestAddAndSecretScrub(t *testing.T) {
	if os.Getenv("SEN_API_KEY") != "" {
		t.Fatalf("expected SEN_API_KEY stripped from child env, got %q", os.Getenv("SEN_API_KEY"))
	}
	fmt.Println("stdout leak sk-proj-1234567890abcdefghij")
	if Add(2, 3) != 5 {
		t.Fatal("bad math")
	}
}
`
	if err := os.WriteFile(filepath.Join(root, "calc_test.go"), []byte(testSrc), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("SEN_API_KEY", "sk-parent-secret-value-999999")

	ctx := context.Background()
	if _, err := verify.RunTaskVerification(ctx, st, root, "T1", []string{"go"}, []string{"cmd", "/C", "echo", "hi"}, 30*time.Second); err == nil {
		t.Fatalf("expected disallowed binary cmd to be rejected")
	}

	ev, err := verify.RunTaskVerification(ctx, st, root, "T1", []string{"go"}, []string{"go", "test", "-v", "./..."}, 30*time.Second)
	if err != nil {
		t.Fatalf("RunTaskVerification failed: %v", err)
	}
	if ev.Strength != "TEST" || ev.TestsRan != 1 || ev.ExitCode != 0 {
		t.Fatalf("unexpected evidence: %+v", ev)
	}
	if strings.Contains(ev.RedactedOutput, "sk-proj-1234567890abcdefghij") || !strings.Contains(ev.RedactedOutput, "[REDACTED]") {
		t.Fatalf("expected secret in stdout to be redacted, got:\n%s", ev.RedactedOutput)
	}

	// DoneGate must FAIL right now because R2 has no VERIFIED task!
	_, err = verify.EvaluateDoneGate(ctx, st, root, []string{"go"}, []string{"go", "test", "-v", "./..."}, 30*time.Second)
	if err == nil || !strings.Contains(err.Error(), "R2") {
		t.Fatalf("expected EvaluateDoneGate to reject uncovered R2, got: %v", err)
	}

	// Now satisfy R2 with T2 and confirm DoneGate PASSES and writes .sen/delivery-report.md.
	if err := st.SaveTask(storage.Task{
		ID: "T2", ReqIDs: []string{"R2"}, Title: "implement sub", Kind: "FEATURE",
		RiskAuto: "LOW", ScopeFiles: []string{"sub.go", "sub_test.go"}, Status: "OPEN",
	}); err != nil {
		t.Fatal(err)
	}
	if err := workspace.BeginTaskSnapshot(st, root, "T2", []string{"sub.go", "sub_test.go"}); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(root, "sub.go"), []byte("package sample\n\nfunc Sub(a, b int) int { return a - b }\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "sub_test.go"), []byte("package sample\n\nimport \"testing\"\nfunc TestSub(t *testing.T) { if Sub(5, 2) != 3 { t.Fatal() } }\n"), 0o644)

	if _, err := verify.RunTaskVerification(ctx, st, root, "T2", []string{"go"}, []string{"go", "test", "-v", "./..."}, 30*time.Second); err != nil {
		t.Fatalf("RunTaskVerification T2 failed: %v", err)
	}

	report, err := verify.EvaluateDoneGate(ctx, st, root, []string{"go"}, []string{"go", "test", "-v", "./..."}, 30*time.Second)
	if err != nil {
		t.Fatalf("EvaluateDoneGate expected PASS, got: %v", err)
	}
	if len(report.Rows) != 2 {
		t.Fatalf("expected 2 requirement coverage rows, got %d", len(report.Rows))
	}
	if _, err := os.Stat(filepath.Join(root, ".sen", "delivery-report.md")); err != nil {
		t.Fatalf("expected .sen/delivery-report.md to exist: %v", err)
	}
}

func TestSpoofedTempCmdBinaryAndBugfixBuildStrengthRejected(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "go.mod"), []byte("module spoof\n\ngo 1.22\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "bug.go"), []byte("package spoof\n\nfunc Fix() int { return 1 }\n"), 0o644)

	st, err := storage.Open(filepath.Join(root, ".sen", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	_, _ = st.AddRequirement("", "Fix crash", "Fix bug", true)
	_ = workspace.CaptureBaseline(st, root)
	_ = st.CreateTask(storage.Task{
		ID: "T-BUG", ReqIDs: []string{"R1"}, Title: "bug", Kind: "BUGFIX",
		RiskAuto: "HIGH", ScopeFiles: []string{"bug.go"}, Status: "OPEN",
	})
	_ = workspace.BeginTaskSnapshot(st, root, "T-BUG", []string{"bug.go"})

	// C2: Fake temp-dir pytest binary/shim must be rejected both path-qualified and via PATH!
	fakeName := "pytest"
	fakeContent := "#!/bin/sh\necho 1 passed\n"
	if runtime.GOOS == "windows" {
		fakeName = "pytest.cmd"
		fakeContent = "@echo off\r\necho 1 passed\r\n"
	}
	fakeCmd := filepath.Join(os.TempDir(), "sen_fake_pytest_check", fakeName)
	_ = os.MkdirAll(filepath.Dir(fakeCmd), 0o755)
	defer os.RemoveAll(filepath.Dir(fakeCmd))
	_ = os.WriteFile(fakeCmd, []byte(fakeContent), 0o755)
	_ = os.Chmod(fakeCmd, 0o755)

	ctx := context.Background()
	if _, err := verify.RunTaskVerification(ctx, st, root, "T-BUG", []string{"go", "pytest"}, []string{fakeCmd}, 10*time.Second); err == nil {
		t.Fatalf("expected path-qualified %s to be rejected", fakeCmd)
	}
	t.Setenv("PATH", filepath.Dir(fakeCmd)+string(os.PathListSeparator)+os.Getenv("PATH"))
	if _, err := verify.RunTaskVerification(ctx, st, root, "T-BUG", []string{"go", "pytest"}, []string{"pytest"}, 10*time.Second); err == nil {
		t.Fatalf("expected %%TEMP%%\\pytest.cmd shim on PATH to be rejected")
	}

	// H2: BUGFIX task cannot be verified with `go build` (BUILD strength, 0 tests)!
	if _, err := verify.RunTaskVerification(ctx, st, root, "T-BUG", []string{"go"}, []string{"go", "build", "./..."}, 30*time.Second); err == nil {
		t.Fatalf("expected BUGFIX task verified with `go build` (BUILD strength) to be rejected")
	}
}

// TestFFprobeParserValidAndCorruptMedia kiểm chứng bộ đọc ffprobe JSON + key=value và từ chối video 0 stream / duration 0.
func TestFFprobeParserValidAndCorruptMedia(t *testing.T) {
	validJSON := `{
		"streams": [
			{"codec_type": "video", "codec_name": "h264", "width": 1920, "height": 1080, "r_frame_rate": "30/1"},
			{"codec_type": "audio", "codec_name": "aac", "channels": 2, "sample_rate": "48000"}
		],
		"format": {"duration": "14.250000"}
	}`
	strength, streams, err := verify.ParseTestRun("ffprobe", []string{"-print_format", "json", "out.mp4"}, validJSON, 0)
	if strength != "MEDIA" || streams != 2 || err != nil {
		t.Fatalf("expected valid 2-stream MEDIA pass, got strength=%s streams=%d err=%v", strength, streams, err)
	}

	validKV := "[STREAM]\ncodec_type=video\ncodec_name=hevc\nwidth=1080\nheight=1920\nr_frame_rate=60/1\nduration=8.5\n[/STREAM]\n"
	strength, streams, err = verify.ParseTestRun("ffprobe.exe", []string{"out.mp4"}, validKV, 0)
	if strength != "MEDIA" || streams != 1 || err != nil {
		t.Fatalf("expected valid key=value MEDIA pass, got strength=%s streams=%d err=%v", strength, streams, err)
	}

	corruptCases := []string{
		`{"streams": [], "format": {"duration": "10.0"}}`,
		`{"streams": [{"codec_type": "video", "codec_name": "h264", "width": 0, "height": 1080}], "format": {"duration": "10.0"}}`,
		`{"streams": [{"codec_type": "video", "codec_name": "h264", "width": 1920, "height": 1080, "r_frame_rate": "0/0"}], "format": {"duration": "10.0"}}`,
		`{"streams": [{"codec_type": "video", "codec_name": "h264", "width": 1920, "height": 1080}], "format": {"duration": "0"}}`,
		`{"streams": [{"codec_type": "audio", "codec_name": "aac", "channels": 0, "sample_rate": "48000"}], "format": {"duration": "5.0"}}`,
	}
	for i, bad := range corruptCases {
		_, sCount, bErr := verify.ParseTestRun("ffprobe", []string{"-print_format", "json", "bad.mp4"}, bad, 0)
		if bErr == nil || sCount != 0 || !strings.Contains(bErr.Error(), "invalid media output") {
			t.Errorf("corrupt media case #%d expected rejection, got count=%d err=%v", i, sCount, bErr)
		}
	}
}

// TestBoundedRepairLoopQuarantineAndUnlock kiểm chứng khóa QUARANTINE sau 3 lần verify lỗi và mở khóa bằng WriteQuarantineReset.
func TestBoundedRepairLoopQuarantineAndUnlock(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "go.mod"), []byte("module qmod\n\ngo 1.22\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "calc.go"), []byte("package qmod\n\nfunc Val() int { return 0 }\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "calc_test.go"), []byte("package qmod\n\nimport \"testing\"\nfunc TestVal(t *testing.T) { if Val() != 42 { t.Fatal(\"fail\") } }\n"), 0o644)

	st, err := storage.Open(filepath.Join(root, ".sen", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	_, _ = st.AddRequirement("", "Make Val return 42", "Val=42", true)
	_ = workspace.CaptureBaseline(st, root)
	_ = st.CreateTask(storage.Task{
		ID: "T-Q", ReqIDs: []string{"R1"}, Title: "q", Kind: "BUGFIX",
		RiskAuto: "LARGE", ScopeFiles: []string{"calc.go"}, Status: "OPEN",
	})
	_ = workspace.BeginTaskSnapshot(st, root, "T-Q", []string{"calc.go"})

	ctx := context.Background()
	var thirdErr error
	for round := 1; round <= 3; round++ {
		_, thirdErr = verify.RunTaskVerification(ctx, st, root, "T-Q", []string{"go"}, []string{"go", "test", "-v", "./..."}, 30*time.Second)
		if thirdErr == nil {
			t.Fatalf("expected round %d to fail", round)
		}
	}
	if !strings.Contains(thirdErr.Error(), "[QUARANTINE 3/3]") {
		t.Fatalf("expected 3rd failure tagged with [QUARANTINE 3/3], got: %v", thirdErr)
	}

	// Lần thứ 4 bị chặn cứng không cho chạy tiến trình con khi chưa được --approve mở khóa!
	_, fourthErr := verify.RunTaskVerification(ctx, st, root, "T-Q", []string{"go"}, []string{"go", "test", "-v", "./..."}, 30*time.Second)
	if fourthErr == nil || !strings.Contains(fourthErr.Error(), "bounded repair loop exceeded") {
		t.Fatalf("expected 4th blind attempt blocked by bounded repair loop, got: %v", fourthErr)
	}

	// Mở khóa qua WriteQuarantineReset (tương đương sen status --approve) rồi sửa đúng mã nguồn -> verify PASS!
	if err := workspace.WriteQuarantineReset(st, root, "T-Q"); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(root, "calc.go"), []byte("package qmod\n\nfunc Val() int { return 42 }\n"), 0o644)
	if _, err := verify.RunTaskVerification(ctx, st, root, "T-Q", []string{"go"}, []string{"go", "test", "-v", "./..."}, 30*time.Second); err != nil {
		t.Fatalf("expected verify after quarantine unlock to PASS, got: %v", err)
	}
	rep, err := verify.EvaluateDoneGateWithOpts(ctx, st, root, []string{"go"}, nil, 30*time.Second, verify.DoneOptions{
		Version: "1.1.0", DomainProfile: "code", ExpressMode: true, Companions: "superpowers=on",
	})
	if err != nil {
		t.Fatalf("EvaluateDoneGateWithOpts failed: %v", err)
	}
	mdBytes, _ := os.ReadFile(rep.MarkdownPath)
	if !strings.Contains(string(mdBytes), "v1.1.0") || !strings.Contains(string(mdBytes), "LARGE") {
		t.Fatalf("expected enhanced delivery report metadata, got:\n%s", string(mdBytes))
	}
}

