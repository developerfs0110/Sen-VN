package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIFrozenEightCommandsRejectsUnknown(t *testing.T) {
	dir := t.TempDir()
	var out, errBuf bytes.Buffer

	code := RunInDir(dir, []string{"nonexistent-subcommand"}, strings.NewReader(""), &out, &errBuf)
	if code != 1 {
		t.Fatalf("expected exit code 1 for unknown command, got %d", code)
	}
	for _, cmd := range []string{"init", "brief", "intake", "begin", "verify", "undo", "done", "status"} {
		if !strings.Contains(errBuf.String(), cmd) {
			t.Errorf("expected usage output to list frozen command %q, got:\n%s", cmd, errBuf.String())
		}
	}
}

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

	// Test positional arg BEFORE flags in `sen begin T1 --reqs R1 --files ...`
	mustOK("intake", "--quote", "Update greeting and check security", "--req", "Update Greet", "--security")
	mustOK("begin", "T1", "--reqs", "R1,R_sec", "--files", "hello.go,hello_test.go", "--kind", "feature")

	statusOut := mustOK("status", "--approve")
	if !strings.Contains(statusOut, "R1") || !strings.Contains(statusOut, "R_sec") || !strings.Contains(statusOut, "T1") {
		t.Fatalf("unexpected status output:\n%s", statusOut)
	}
}

