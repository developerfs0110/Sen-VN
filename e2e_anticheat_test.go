package senmvp_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/developerfs0110/Sen-VN/internal/storage"
)

var compiledSenBin string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "sen-e2e-bin-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "mkdirtemp failed: %v\n", err)
		os.Exit(1)
	}
	binName := "sen"
	if runtime.GOOS == "windows" {
		binName = "sen.exe"
	}
	compiledSenBin = filepath.Join(tmpDir, binName)

	build := exec.Command("go", "build", "-o", compiledSenBin, "./cmd/sen")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "failed to build sen binary: %v\n%s\n", err, string(out))
		_ = os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	code := m.Run()
	_ = os.RemoveAll(tmpDir)
	os.Exit(code)
}

func runSen(t *testing.T, workDir, stdinText string, extraEnv []string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(compiledSenBin, args...)
	cmd.Dir = workDir
	cmd.Env = append(os.Environ(), extraEnv...)
	if stdinText != "" {
		cmd.Stdin = strings.NewReader(stdinText)
	}
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	err := cmd.Run()
	code := 0
	if err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			t.Fatalf("exec error running sen %v: %v", args, err)
		}
	}
	return out.String(), code
}

func mustSenOK(t *testing.T, workDir string, args ...string) string {
	t.Helper()
	out, code := runSen(t, workDir, "", nil, args...)
	if code != 0 {
		t.Fatalf("expected sen %v to exit 0, got %d:\n%s", args, code, out)
	}
	return out
}

func writeRepoFile(t *testing.T, root, rel, content string) {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// Scenario 1: Empty test suite ([no test files] AND empty *_test.go with 0 TestXxx) -> REJECTED.
func TestScenario1_RejectEmptyTestRun(t *testing.T) {
	dir := t.TempDir()
	writeRepoFile(t, dir, "go.mod", "module scenario1\n\ngo 1.22\n")
	writeRepoFile(t, dir, "svc.go", "package scenario1\n\nfunc Val() int { return 42 }\n")

	mustSenOK(t, dir, "init")
	mustSenOK(t, dir, "intake", "--quote", "Add Val", "--req", "Return 42")
	mustSenOK(t, dir, "begin", "T1", "--reqs", "R1", "--files", "svc.go,svc_test.go")

	// Subcase A: No *_test.go file at all ([no test files])
	out, code := runSen(t, dir, "", nil, "verify", "T1", "--", "go", "test", "./...")
	if code == 0 || !strings.Contains(out, "zero tests executed") {
		t.Fatalf("expected [no test files] to be rejected, got code=%d:\n%s", code, out)
	}

	// Subcase B: *_test.go exists, compiles, but contains 0 TestXxx functions ([no tests to run])
	writeRepoFile(t, dir, "svc_test.go", "package scenario1\n")
	out, code = runSen(t, dir, "", nil, "verify", "T1", "--", "go", "test", "-v", "./...")
	if code == 0 || !strings.Contains(out, "zero tests executed") {
		t.Fatalf("expected [no tests to run] to be rejected, got code=%d:\n%s", code, out)
	}
}

// Scenario 2: Pre-existing test file locked -> REJECTED at begin AND at verify if tampered on disk.
func TestScenario2_RejectLockedPreExistingTestTampering(t *testing.T) {
	dir := t.TempDir()
	writeRepoFile(t, dir, "go.mod", "module scenario2\n\ngo 1.22\n")
	writeRepoFile(t, dir, "math.go", "package scenario2\n\nfunc Double(x int) int { return x }\n")
	writeRepoFile(t, dir, "math_test.go", "package scenario2\n\nimport \"testing\"\nfunc TestDouble(t *testing.T) { if Double(2) != 4 { t.Fatal(\"broken\") } }\n")

	mustSenOK(t, dir, "init")
	mustSenOK(t, dir, "intake", "--quote", "Fix Double", "--req", "Multiply by 2", "--bugfix")

	// Subcase A: Declaring pre-existing math_test.go in --files is rejected at `sen begin`
	out, code := runSen(t, dir, "", nil, "begin", "T-CHEAT", "--reqs", "R1", "--files", "math.go,math_test.go")
	if code == 0 || !strings.Contains(out, "locked") {
		t.Fatalf("expected sen begin to reject pre-existing test file, got code=%d:\n%s", code, out)
	}

	// Subcase B: Begin validly with math.go only, then weaken math_test.go on disk -> rejected at `sen verify`
	mustSenOK(t, dir, "begin", "T-REAL", "--reqs", "R1", "--files", "math.go")
	writeRepoFile(t, dir, "math_test.go", "package scenario2\n\nimport \"testing\"\nfunc TestDouble(t *testing.T) {}\n")

	out, code = runSen(t, dir, "", nil, "verify", "T-REAL", "--", "go", "test", "-v", "./...")
	if code == 0 || !strings.Contains(out, "locked test file modified") {
		t.Fatalf("expected sen verify to reject tampered locked test, got code=%d:\n%s", code, out)
	}
}

// Scenario 3: Uncovered requirement (R1 & R2 registered, only R1 verified) -> `sen done` EXITS 1.
func TestScenario3_RejectUncoveredRequirementAtDone(t *testing.T) {
	dir := t.TempDir()
	writeRepoFile(t, dir, "go.mod", "module scenario3\n\ngo 1.22\n")
	writeRepoFile(t, dir, "feat.go", "package scenario3\n\nfunc A() int { return 1 }\n")

	mustSenOK(t, dir, "init")
	mustSenOK(t, dir, "intake", "--quote", "Build A and B", "--req", "Implement A")
	mustSenOK(t, dir, "intake", "--quote", "Build A and B", "--req", "Implement B")

	mustSenOK(t, dir, "begin", "T1", "--reqs", "R1", "--files", "feat_test.go")
	writeRepoFile(t, dir, "feat_test.go", "package scenario3\n\nimport \"testing\"\nfunc TestA(t *testing.T) { if A() != 1 { t.Fatal() } }\n")
	mustSenOK(t, dir, "verify", "T1", "--", "go", "test", "-v", "./...")

	out, code := runSen(t, dir, "", nil, "done")
	if code == 0 || !strings.Contains(out, "R2") {
		t.Fatalf("expected sen done to fail on uncovered R2, got code=%d:\n%s", code, out)
	}
}

// Scenario 4: Out-of-scope stealth file edit/creation in non-Git folder -> REJECTED vs __BASELINE__.
func TestScenario4_RejectOutOfScopeStealthEditWithoutGit(t *testing.T) {
	dir := t.TempDir() // No git init here — pure __BASELINE__ tracking!
	writeRepoFile(t, dir, "go.mod", "module scenario4\n\ngo 1.22\n")
	writeRepoFile(t, dir, "core.go", "package scenario4\n\nfunc Ok() bool { return true }\n")
	writeRepoFile(t, dir, "config.yaml", "mode: production\n")

	mustSenOK(t, dir, "init")
	mustSenOK(t, dir, "intake", "--quote", "Test Ok", "--req", "Verify Ok")
	mustSenOK(t, dir, "begin", "T1", "--reqs", "R1", "--files", "core_test.go")

	writeRepoFile(t, dir, "core_test.go", "package scenario4\n\nimport \"testing\"\nfunc TestOk(t *testing.T) { if !Ok() { t.Fatal() } }\n")
	// Stealth untracked file outside scope:
	writeRepoFile(t, dir, "backdoor.go", "package scenario4\n")

	out, code := runSen(t, dir, "", nil, "verify", "T1", "--", "go", "test", "-v", "./...")
	if code == 0 || !strings.Contains(out, "out-of-scope") {
		t.Fatalf("expected out-of-scope stealth file rejection, got code=%d:\n%s", code, out)
	}
}

// Scenario 5: SEN_API_KEY env leak & stdout token leak -> child env stripped + DB output [REDACTED].
func TestScenario5_EnvFilterAndEvidenceSecretRedaction(t *testing.T) {
	dir := t.TempDir()
	writeRepoFile(t, dir, "go.mod", "module scenario5\n\ngo 1.22\n")
	writeRepoFile(t, dir, "ping.go", "package scenario5\n\nfunc Ping() string { return \"pong\" }\n")

	mustSenOK(t, dir, "init")
	mustSenOK(t, dir, "intake", "--quote", "Test ping", "--req", "Ping check")
	mustSenOK(t, dir, "begin", "T1", "--reqs", "R1", "--files", "ping_test.go")

	testCode := `package scenario5

import (
	"fmt"
	"os"
	"testing"
)

func TestEnvAndStdoutScrub(t *testing.T) {
	if got := os.Getenv("SEN_API_KEY"); got != "" {
		t.Fatalf("LEAK! SEN_API_KEY reached child process: %s", got)
	}
	fmt.Println("accidental token sk-proj-9876543210abcdefghijk and ghp_zzzzzzzzzzzzzzzzzzzz123456")
}
`
	writeRepoFile(t, dir, "ping_test.go", testCode)

	out, code := runSen(t, dir, "", []string{"SEN_API_KEY=sk-live-topsecretvalue99999"}, "verify", "T1", "--", "go", "test", "-v", "./...")
	if code != 0 {
		t.Fatalf("expected verify to pass (meaning child SEN_API_KEY was empty), got code=%d:\n%s", code, out)
	}

	st, err := storage.Open(filepath.Join(dir, ".sen", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	evs, err := st.ListEvidence()
	if err != nil || len(evs) != 1 {
		t.Fatalf("expected 1 evidence record, got %d (err=%v)", len(evs), err)
	}
	if strings.Contains(evs[0].RedactedOutput, "sk-proj-9876543210abcdefghijk") ||
		strings.Contains(evs[0].RedactedOutput, "ghp_zzzzzzzzzzzzzzzzzzzz123456") ||
		!strings.Contains(evs[0].RedactedOutput, "[REDACTED]") {
		t.Fatalf("expected secrets redacted in SQLite evidence, got:\n%s", evs[0].RedactedOutput)
	}
}

// Scenario 6: Mid-task crash -> `sen brief` warns + `sen undo` restores byte-for-byte & deletes new files.
func TestScenario6_CrashRecoveryBriefWarningAndByteExactUndo(t *testing.T) {
	dir := t.TempDir()
	origBytes := "package scenario6\n\n// Exact original header\nfunc Stable() int { return 100 }\n"
	writeRepoFile(t, dir, "go.mod", "module scenario6\n\ngo 1.22\n")
	writeRepoFile(t, dir, "stable.go", origBytes)

	mustSenOK(t, dir, "init")
	mustSenOK(t, dir, "intake", "--quote", "Refactor stable", "--req", "Refactor")
	mustSenOK(t, dir, "begin", "T-CRASH", "--reqs", "R1", "--files", "stable.go,temp_broken.go")

	// Simulate mid-task crash leaving corrupted file + newly created file
	writeRepoFile(t, dir, "stable.go", "CORRUPTED HALF WRITTEN CODE {{{")
	writeRepoFile(t, dir, "temp_broken.go", "package scenario6\nBROKEN")

	briefOut := mustSenOK(t, dir, "brief")
	if !strings.Contains(briefOut, "WARNING") || !strings.Contains(briefOut, "T-CRASH") || !strings.Contains(briefOut, "sen undo") {
		t.Fatalf("expected sen brief to warn about interrupted T-CRASH, got:\n%s", briefOut)
	}

	mustSenOK(t, dir, "undo")
	restored, err := os.ReadFile(filepath.Join(dir, "stable.go"))
	if err != nil || string(restored) != origBytes {
		t.Fatalf("expected byte-for-byte restoration of stable.go, got %q (err=%v)", string(restored), err)
	}
	if _, err := os.Stat(filepath.Join(dir, "temp_broken.go")); !os.IsNotExist(err) {
		t.Fatalf("expected newly created temp_broken.go deleted by sen undo")
	}
}

// Full Happy Path: Antigravity hook stdin -> intake (--security) -> begin -> verify -> status --approve -> done!
func TestHappyPathEndToEndWithHooksAndDoneGate(t *testing.T) {
	dir := t.TempDir()
	writeRepoFile(t, dir, "go.mod", "module happy\n\ngo 1.22\n")
	writeRepoFile(t, dir, "mul.go", "package happy\n\nfunc Mul(a, b int) int { return a * b }\n")

	mustSenOK(t, dir, "init", "--agy-hooks")

	hookIn := `{"hookEventName":"PreInvocation","lastUserInput":"Verify Mul and run security check"}`
	hookOut, code := runSen(t, dir, hookIn, nil, "brief", "--hook")
	if code != 0 || !strings.Contains(hookOut, "systemMessage") {
		t.Fatalf("PreInvocation hook failed (code=%d): %s", code, hookOut)
	}

	// Quote omitted -> auto-loaded from .sen/last_prompt.txt written by PreInvocation hook!
	mustSenOK(t, dir, "intake", "--req", "Verify Mul(a, b)", "--security")
	mustSenOK(t, dir, "begin", "T-MUL", "--reqs", "R1,R_sec", "--files", "mul_test.go")
	writeRepoFile(t, dir, "mul_test.go", "package happy\n\nimport \"testing\"\nfunc TestMul(t *testing.T) { if Mul(6, 7) != 42 { t.Fatal() } }\n")

	mustSenOK(t, dir, "verify", "T-MUL", "--", "go", "test", "-v", "./...")
	mustSenOK(t, dir, "status", "--approve")
	doneOut := mustSenOK(t, dir, "done")

	for _, want := range []string{"SEN DONE", "R1", "R_sec", "Verify Mul and run security check", "T-MUL", "TEST (1)"} {
		if !strings.Contains(doneOut, want) {
			t.Errorf("expected sen done output to contain %q, got:\n%s", want, doneOut)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, ".sen", "delivery-report.md")); err != nil {
		t.Fatalf("expected .sen/delivery-report.md generated: %v", err)
	}
}

