package senmvp_test

import (
	"bytes"
	"encoding/json"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func copyTreeFiltered(t *testing.T, srcRoot, dstRoot string) {
	t.Helper()
	err := filepath.WalkDir(srcRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, err := filepath.Rel(srcRoot, path)
		if err != nil || rel == "." {
			return err
		}
		top := strings.Split(filepath.ToSlash(rel), "/")[0]
		if top == ".git" || top == ".sen" || top == ".cache" || top == ".agents" || rel == "sen.exe" || rel == "sen" {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		dst := filepath.Join(dstRoot, rel)
		if d.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
		if err != nil {
			return err
		}
		if _, err := io.Copy(out, in); err != nil {
			_ = out.Close()
			return err
		}
		return out.Close()
	})
	if err != nil {
		t.Fatalf("copyTreeFiltered failed: %v", err)
	}
	_ = os.Chmod(filepath.Join(dstRoot, "bin", "sen"), 0o755)
	_ = os.Chmod(filepath.Join(dstRoot, "scripts", "install-sen.sh"), 0o755)
}

func assertPluginManifestAndFiles(t *testing.T) map[string]any {
	t.Helper()
	rawPlugin, err := os.ReadFile("plugin.json")
	if err != nil {
		t.Fatalf("missing root plugin.json: %v", err)
	}
	var manifest map[string]any
	if err := json.Unmarshal(rawPlugin, &manifest); err != nil {
		t.Fatalf("invalid plugin.json: %v", err)
	}
	if manifest["name"] != "sen-vn" || manifest["version"] != "1.1.0" || manifest["repository"] != "https://github.com/developerfs0110/Sen-VN" {
		t.Fatalf("unexpected plugin.json metadata: %+v", manifest)
	}

	rawHooks, err := os.ReadFile("hooks.json")
	if err != nil {
		t.Fatalf("missing root hooks.json: %v", err)
	}
	var hooksMap map[string]any
	if err := json.Unmarshal(rawHooks, &hooksMap); err != nil {
		t.Fatalf("invalid root hooks.json: %v", err)
	}
	senHooks, ok := hooksMap["sen-mvp"].(map[string]any)
	if !ok {
		t.Fatalf("expected top-level sen-mvp key in hooks.json, got: %+v", hooksMap)
	}
	for _, ev := range []string{"SessionStart", "PreInvocation", "PreToolUse", "Stop"} {
		if _, exists := senHooks[ev]; !exists {
			t.Errorf("hooks.json missing event %q", ev)
		}
	}

	for _, reqFile := range []string{
		"packages.lock.json",
		filepath.Join("rules", "AGENTS.md"),
		filepath.Join("skills", "sen-setup", "SKILL.md"),
		filepath.Join("skills", "sen-dialectical", "SKILL.md"),
		filepath.Join("skills", "sen-skill-capture", "SKILL.md"),
		filepath.Join("bin", "sen"),
		filepath.Join("bin", "sen.cmd"),
		filepath.Join("scripts", "install-sen.sh"),
	} {
		if _, err := os.Stat(reqFile); err != nil {
			t.Fatalf("missing required plugin/zero-build file %s: %v", reqFile, err)
		}
	}
	return senHooks
}

func TestZeroBuildPluginLayoutAndSmartWrapperHookForwarding(t *testing.T) {
	senHooks := assertPluginManifestAndFiles(t)

	// Simulate fresh `git clone` with ZERO pre-built `.exe` files!
	freshClone := t.TempDir()
	copyTreeFiltered(t, ".", freshClone)
	pluginData := t.TempDir()
	userProj := t.TempDir()
	writeRepoFile(t, userProj, "go.mod", "module userproj\n\ngo 1.26\n")
	mustSenOK(t, userProj, "init")

	escUser := strings.ReplaceAll(userProj, `\`, `\\`)
	hookStdin := `{"hookEventName":"PreInvocation","invocationNum":1,"cwd":"` + escUser + `","lastUserInput":"Zero-build plugin wrapper stdin check"}`

	runWrapper := func(stdin string, shellCmd string) (string, string) {
		t.Helper()
		cmd := newHookShellCommand(freshClone, shellCmd)
		if stdin != "" {
			cmd.Stdin = strings.NewReader(stdin)
		}
		cmd.Env = append(os.Environ(),
			"PLUGIN_DATA="+pluginData,
			"SEN_GITHUB_REPO=invalid-local-test/nonexistent-sen-mvp",
		)
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		if err := cmd.Run(); err != nil {
			t.Fatalf("wrapper %q failed: %v\nstdout: %s\nstderr: %s", shellCmd, err, stdout.String(), stderr.String())
		}
		return stdout.String(), stderr.String()
	}

	preInvArr, ok := senHooks["PreInvocation"].([]any)
	if !ok || len(preInvArr) == 0 {
		t.Fatalf("missing PreInvocation handler in hooks.json")
	}
	preInvCmd, _ := preInvArr[0].(map[string]any)["command"].(string)

	// Cold-start invocation using exact hooks.json command: compiles into PLUGIN_DATA/bin/sen(.exe)
	// without consuming stdin or polluting stdout, resolving bin/sen.cmd alongside extensionless bin/sen!
	stdoutStr, stderrStr := runWrapper(hookStdin, preInvCmd)
	var hookResp map[string]any
	if err := json.Unmarshal([]byte(stdoutStr), &hookResp); err != nil {
		t.Fatalf("expected pure JSON on wrapper stdout, got %v:\nstdout=%s\nstderr=%s", err, stdoutStr, stderrStr)
	}
	if _, ok := hookResp["injectSteps"]; !ok {
		t.Fatalf("expected injectSteps in PreInvocation hook response, got: %s", stdoutStr)
	}

	gotPrompt, err := os.ReadFile(filepath.Join(userProj, ".sen", "last_prompt.txt"))
	if err != nil || string(gotPrompt) != "Zero-build plugin wrapper stdin check" {
		t.Fatalf("expected userProj last_prompt.txt populated, got %q (err=%v)", string(gotPrompt), err)
	}

	cachedExe := filepath.Join(pluginData, "bin", "sen")
	if runtime.GOOS == "windows" {
		cachedExe += ".exe"
	}
	if _, err := os.Stat(cachedExe); err != nil {
		t.Fatalf("expected bootstrapped binary cached at %s: %v", cachedExe, err)
	}

	// Second call hits `<2ms` cached fast-path directly using exact Stop hook command!
	stopArr, _ := senHooks["Stop"].([]any)
	stopCmd, _ := stopArr[0].(map[string]any)["command"].(string)
	stdout2, _ := runWrapper(`{"hookEventName":"Stop","cwd":"`+escUser+`","terminationReason":"NO_TOOL_CALL"}`, stopCmd)
	if !strings.Contains(stdout2, `"allow"`) {
		t.Fatalf("expected fast-path Stop hook allow, got: %s", stdout2)
	}

	// Also verify direct CLI invocation `"./bin/sen" status` inside an initialized clone workspace.
	runWrapper("", `"./bin/sen" init`)
	statusOut, _ := runWrapper("", `"./bin/sen" status`)
	if !strings.Contains(statusOut, "=== SEN STATUS ===") {
		t.Fatalf("expected './bin/sen status' output, got: %s", statusOut)
	}
}
