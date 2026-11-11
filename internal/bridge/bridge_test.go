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
		RiskAuto: "HIGH", ScopeFiles: []string{"auth.go"}, Status: "OPEN",
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

