// Tệp ssds_isolation_version_test.go kiểm thử đối kháng Suite 6 (Cô lập đa dự án) và Suite 7 (Liên tục phiên & Đa phiên bản SSDS độc lập).
package senmvp_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/developerfs0110/Sen-VN/internal/bridge"
	"github.com/developerfs0110/Sen-VN/internal/ssds"
	"github.com/developerfs0110/Sen-VN/internal/storage"
)

// TestSSDSSuite6_MultiProjectIsolationZeroContamination kiểm chứng Suite 6: Không rò rỉ dữ liệu giữa ProjectERP, ProjectTrading và Workspace mẹ.
func TestSSDSSuite6_MultiProjectIsolationZeroContamination(t *testing.T) {
	umbrella := t.TempDir()
	erpDir := filepath.Join(umbrella, "ProjectERP")
	tradingDir := filepath.Join(umbrella, "ProjectTrading")
	if err := os.MkdirAll(erpDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(tradingDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Khởi tạo 2 dự án con từ thư mục mẹ `umbrella` thông qua cờ `--project`!
	mustSenOK(t, umbrella, "--project", "ProjectERP", "sdlc", "classify",
		"--prompt", "Xây dựng phần mềm ERP quản lý hóa đơn", "--workflow", "WF_STANDARD")
	mustSenOK(t, umbrella, "--project", "ProjectTrading", "sdlc", "classify",
		"--prompt", "Xây dựng hệ thống đặt lệnh chứng khoán tần suất cao", "--workflow", "WF_CRITICAL")

	// Ghi rule, charter, requirements và ADR hoàn toàn khác nhau cho từng dự án con.
	mustSenOK(t, umbrella, "--project", "ProjectERP", "intake", "--rule", "ERP_DECIMAL_VND_ONLY", "--quote", "Luật ERP")
	writeRepoFile(t, erpDir, "docs/00-project-charter.md", validCharterMD("ProjectERP"))
	mustSenOK(t, erpDir, "sdlc", "gate", "P0")
	writeRepoFile(t, erpDir, "docs/01-requirements.md", validRequirementsMD("Quản lý hóa đơn GTGT", "Xuất báo cáo công nợ"))
	mustSenOK(t, erpDir, "sdlc", "review", "--phase", "P1", "--author", "Senior Business Analyst", "--reviewer", "System Analyst", "--ref", "erp-r1", "--verdict", "PASS")
	mustSenOK(t, erpDir, "sdlc", "gate", "P1")
	mustSenOK(t, erpDir, "sdlc", "approve", "--phase", "P1", "--quote", "Duyệt P1 ERP")
	writeRepoFile(t, erpDir, "docs/adr/ADR-001-erp-invoice.md", "# ADR-001 ERP Invoice Ledger\n")

	mustSenOK(t, umbrella, "--project", "ProjectTrading", "intake", "--rule", "TRADING_STOP_LOSS_MANDATORY", "--quote", "Luật Trading")
	writeRepoFile(t, tradingDir, "docs/00-project-charter.md", validCharterMD("ProjectTrading"))
	mustSenOK(t, tradingDir, "sdlc", "review", "--phase", "P0", "--author", "Product Manager", "--reviewer", "Technical Lead", "--ref", "trd-r0", "--verdict", "PASS")
	mustSenOK(t, tradingDir, "sdlc", "gate", "P0")
	writeRepoFile(t, tradingDir, "docs/01-requirements.md", validRequirementsMD("Đặt lệnh cắt lỗ tự động", "Khớp lệnh ký quỹ"))
	mustSenOK(t, tradingDir, "sdlc", "review", "--phase", "P1", "--author", "Senior Business Analyst", "--reviewer", "System Analyst", "--ref", "trd-r1", "--verdict", "PASS")
	mustSenOK(t, tradingDir, "sdlc", "gate", "P1")
	// Lưu ý: ProjectTrading cố tình để ở REVIEW_REQUIRED tại P1 (chưa approve sang P2), còn ProjectERP đã ở P2!

	// Gửi PreToolUse với cwd = umbrella (thư mục mẹ!) nhắm vào tệp con trong ProjectERP so với ProjectTrading:
	// ProjectERP đã ở P2 -> cho phép ghi docs/02-architecture.md; ProjectTrading còn ở P1 REVIEW_REQUIRED -> từ chối!
	_, erpHookResp := invokeHookJSON(t, umbrella, "PreToolUse", map[string]any{
		"hookEventName": "PreToolUse",
		"cwd":           umbrella,
		"toolCall":      map[string]any{"name": "write_file", "args": map[string]any{"Path": "ProjectERP/docs/02-architecture.md"}},
	})
	if erpHookResp["decision"] != "allow" {
		t.Fatalf("expected umbrella PreToolUse targeting ProjectERP/docs/02-architecture.md (in P2) to allow, got: %+v", erpHookResp)
	}
	_, trdHookResp := invokeHookJSON(t, umbrella, "PreToolUse", map[string]any{
		"hookEventName": "PreToolUse",
		"cwd":           umbrella,
		"toolCall":      map[string]any{"name": "write_file", "args": map[string]any{"Path": "ProjectTrading/docs/02-architecture.md"}},
	})
	if trdHookResp["decision"] != "deny" {
		t.Fatalf("expected umbrella PreToolUse targeting ProjectTrading/docs/02-architecture.md (still P1) to deny, got: %+v", trdHookResp)
	}

	// Mở trực tiếp 2 tệp SQLite chứng minh 0% nhiễm chéo project_id, requirements, rules, phases!
	stERP, err := storage.Open(filepath.Join(erpDir, ".sen", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer stERP.Close()
	stTrd, err := storage.Open(filepath.Join(tradingDir, ".sen", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer stTrd.Close()

	pERP, _ := stERP.GetSSDSProject()
	pTrd, _ := stTrd.GetSSDSProject()
	if pERP.ProjectID == pTrd.ProjectID || pERP.WorkflowTier != "WF_STANDARD" || pTrd.WorkflowTier != "WF_CRITICAL" || pERP.CurrentPhase != "P2" || pTrd.CurrentPhase != "P1" {
		t.Fatalf("cross-project contamination detected: ERP=%+v vs Trading=%+v", pERP, pTrd)
	}
	reqsERP, _ := stERP.ListRequirements()
	reqsTrd, _ := stTrd.ListRequirements()
	if !strings.Contains(reqsERP[0].Summary, "Quản lý hóa đơn") || strings.Contains(reqsERP[0].Summary, "cắt lỗ") {
		t.Fatalf("ERP requirements contaminated: %+v", reqsERP)
	}
	if !strings.Contains(reqsTrd[0].Summary, "Đặt lệnh cắt lỗ") || strings.Contains(reqsTrd[0].Summary, "hóa đơn") {
		t.Fatalf("Trading requirements contaminated: %+v", reqsTrd)
	}
	rulesERP, _ := stERP.ListRules()
	rulesTrd, _ := stTrd.ListRules()
	if len(rulesERP) != 1 || rulesERP[0].RuleText != "ERP_DECIMAL_VND_ONLY" || len(rulesTrd) != 1 || rulesTrd[0].RuleText != "TRADING_STOP_LOSS_MANDATORY" {
		t.Fatalf("project rules leaked across sibling projects: ERP=%+v Trading=%+v", rulesERP, rulesTrd)
	}

	// Khởi tạo ProjectWorkspace ở thư mục mẹ `umbrella` và chứng minh task của thư mục mẹ cố chạm `ProjectERP/main.go` bị CheckNoSubprojectCrossing chặn đứng!
	mustSenOK(t, umbrella, "init")
	mustSenOK(t, umbrella, "intake", "--quote", "Root workspace readme", "--req", "Root note")
	crossOut, crossCode := runSen(t, umbrella, "", nil, "begin", "T-CROSS", "--reqs", "R1", "--files", "ProjectERP/main.go")
	if crossCode == 0 || !strings.Contains(crossOut, "sub-project boundary violation") {
		t.Fatalf("expected parent workspace task crossing into ProjectERP/main.go to fail CheckNoSubprojectCrossing, got code=%d:\n%s", crossCode, crossOut)
	}
}

// TestSSDSSuite7_CrossSessionContinuityAndIndependentSSDSVersioning kiểm chứng Suite 7: Khôi phục phiên mới & Quản lý đa phiên bản SSDS độc lập (v1.0.0 -> v2.0.0 -> rollback).
func TestSSDSSuite7_CrossSessionContinuityAndIndependentSSDSVersioning(t *testing.T) {
	defer func() {
		ssds.UnregisterRulepackForTest("1.1.0")
		ssds.UnregisterRulepackForTest("2.0.0")
	}()

	dir := t.TempDir()
	mustSenOK(t, dir, "sdlc", "classify", "--prompt", "Tạo dịch vụ thanh toán nội bộ", "--workflow", "WF_STANDARD")
	writeRepoFile(t, dir, "docs/00-project-charter.md", validCharterMD("PayCore"))
	mustSenOK(t, dir, "sdlc", "gate", "P0")
	writeRepoFile(t, dir, "docs/01-requirements.md", validRequirementsMD("Tạo giao dịch chuyển khoản", "Đối soát bút toán cuối ngày"))
	mustSenOK(t, dir, "sdlc", "review", "--phase", "P1", "--author", "Senior Business Analyst", "--reviewer", "System Analyst", "--ref", "s7-r1", "--verdict", "PASS")
	mustSenOK(t, dir, "sdlc", "gate", "P1")
	mustSenOK(t, dir, "sdlc", "approve", "--phase", "P1", "--quote", "Duyệt P1 PayCore")

	// Ghi nhận một task đang chờ ở P2/P5 vào sổ cái để kiểm tra khôi phục phiên mới qua SessionStart!
	stInit, err := storage.Open(filepath.Join(dir, ".sen", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	_ = stInit.CreateTask(storage.Task{
		ID: "TASK-ARCH-01", ReqIDs: []string{"REQ-F-001"}, Title: "Draft architecture",
		Kind: "FEATURE", RiskAuto: "SMALL", ScopeFiles: []string{"docs/02-architecture.md"},
		Status: "PENDING", PhaseID: "P2", ExecState: "IMPLEMENTED", Verdict: "NOT_EXECUTED",
	})
	_ = stInit.Close()
	mustSenOK(t, dir, "sdlc", "sync")

	// 7A. Mô phỏng mở phiên làm việc mới hoàn toàn (SessionStart hook) -> khôi phục chính xác phase=P2, role=Principal Software Architect và next_task=TASK-ARCH-01!
	sessOut, _ := invokeHookJSON(t, dir, "SessionStart", map[string]any{
		"hookEventName": "SessionStart",
		"cwd":           dir,
	})
	for _, want := range []string{"phase=P2:IN_PROGRESS", "role=Principal Software Architect", "next_task: TASK-ARCH-01"} {
		if !strings.Contains(sessOut, want) {
			t.Fatalf("expected SessionStart to reconstruct %q without chat history, got:\n%s", want, sessOut)
		}
	}
	projStatusBytes, err := os.ReadFile(filepath.Join(dir, "PROJECT_STATUS.md"))
	if err != nil || !strings.Contains(string(projStatusBytes), "SEN-STD-SDLC-001 v1.0.0") || !strings.Contains(string(projStatusBytes), "`P2`") {
		t.Fatalf("expected PROJECT_STATUS.md synced on disk with v1.0.0 and P2, got:\n%s", string(projStatusBytes))
	}

	// 7B. Đăng ký thêm SSDS v1.1.0 và SSDS v2.0.0 (yêu cầu mục mới "Chaos Resilience" trong docs/01-requirements.md) cho cả in-process lẫn subprocess CLI!
	baseRP, err := ssds.LoadRulepack("1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	rp11 := *baseRP
	rp11.Version = "1.1.0"
	ssds.RegisterRulepackForTest(rp11)

	rp20 := *baseRP
	rp20.Version = "2.0.0"
	rp20Phases := make([]ssds.PhaseSpec, len(baseRP.Phases))
	copy(rp20Phases, baseRP.Phases)
	for i := range rp20Phases {
		if rp20Phases[i].PhaseID == "P1" {
			newMap := make(map[string][]string)
			for k, v := range rp20Phases[i].RequiredHeadings {
				cp := append([]string(nil), v...)
				if k == "docs/01-requirements.md" {
					cp = append(cp, "Chaos Resilience")
				}
				newMap[k] = cp
			}
			rp20Phases[i].RequiredHeadings = newMap
		}
	}
	rp20.Phases = rp20Phases
	ssds.RegisterRulepackForTest(rp20)

	extraVersDir := t.TempDir()
	for _, item := range []struct {
		dir string
		rp  ssds.Rulepack
	}{
		{"v1.1.0", rp11},
		{"v2.0.0", rp20},
	} {
		b, _ := json.MarshalIndent(item.rp, "", "  ")
		writeRepoFile(t, extraVersDir, item.dir+"/manifest.json", string(b))
	}
	extraEnv := []string{ssds.ExtraVersionsEnvVar + "=" + extraVersDir}

	// Dự án đang ghim v1.0.0 nhận thông báo không xâm lấn `ssds_notice`, chạy lại `sen init` và bật `--express` VẪN giữ nguyên ssds_version = "1.0.0", và gate P1 v1.0.0 vẫn PASS!
	sessWithNotice, _ := invokeHookJSON(t, dir, "SessionStart", map[string]any{"hookEventName": "SessionStart", "cwd": dir})
	if !strings.Contains(sessWithNotice, "ssds_notice: pinned v1.0.0 (v2.0.0 available") {
		t.Fatalf("expected non-intrusive ssds_notice for v2.0.0 in SessionStart, got:\n%s", sessWithNotice)
	}
	if out, code := runSen(t, dir, "", extraEnv, "init", "--express"); code != 0 {
		t.Fatalf("re-running sen init --express failed:\n%s", out)
	}
	assertSignedConfigSSDSVersion(t, dir, "1.0.0")
	if out, code := runSen(t, dir, "", extraEnv, "sdlc", "gate", "P1"); code != 0 {
		t.Fatalf("active v1.0.0 project must continue passing v1.0.0 P1 gate untouched after v2.0.0 release:\n%s", out)
	}

	// 7C. Kiểm tra tương thích chỉ-đọc `--check 2.0.0` phát hiện thiếu mục "Chaos Resilience" mà không làm đổi phiên bản đang ghim!
	checkOut, checkCode := runSen(t, dir, "", extraEnv, "sdlc", "version", "--check", "2.0.0")
	if checkCode != 0 || !strings.Contains(checkOut, "Chaos Resilience") || !strings.Contains(checkOut, "Compatible=false") {
		t.Fatalf("expected --check 2.0.0 to detect missing 'Chaos Resilience' read-only, got code=%d:\n%s", checkCode, checkOut)
	}
	assertSignedConfigSSDSVersion(t, dir, "1.0.0")

	// 7D. Nâng cấp `--upgrade 2.0.0` thiếu `--approve` BẮT BUỘC bị từ chối (§21 CRITICAL)!
	if out, code := runSen(t, dir, "", extraEnv, "sdlc", "version", "--upgrade", "2.0.0"); code == 0 {
		t.Fatalf("expected --upgrade 2.0.0 without --approve to FAIL, got 0:\n%s", out)
	}

	// Có `--approve` + `--quote` -> nâng cấp lên 2.0.0 thành công và ký lại `.sen/config.sha256`!
	upgOut, upgCode := runSen(t, dir, "", extraEnv, "sdlc", "version", "--upgrade", "2.0.0", "--approve", "--quote", "Duyệt nâng cấp SSDS 2.0")
	if upgCode != 0 || !strings.Contains(upgOut, "v2.0.0") {
		t.Fatalf("expected approved --upgrade 2.0.0 to succeed, got code=%d:\n%s", upgCode, upgOut)
	}
	assertSignedConfigSSDSVersion(t, dir, "2.0.0")

	// Sau khi lên 2.0.0, chạy gate P1 sẽ thực thi quy tắc mới ("Chaos Resilience")!
	if out, code := runSen(t, dir, "", extraEnv, "sdlc", "gate", "P1"); code == 0 || !strings.Contains(out, "Chaos Resilience") {
		t.Fatalf("expected upgraded v2.0.0 project to enforce 'Chaos Resilience' on gate P1, got code=%d:\n%s", code, out)
	}

	// 7E. Hoàn tác `--rollback` -> khôi phục nguyên trạng v1.0.0 và chữ ký `.sen/config.sha256`!
	rbOut, rbCode := runSen(t, dir, "", extraEnv, "sdlc", "version", "--rollback")
	if rbCode != 0 || !strings.Contains(rbOut, "v1.0.0") {
		t.Fatalf("expected --rollback to restore v1.0.0, got code=%d:\n%s", rbCode, rbOut)
	}
	assertSignedConfigSSDSVersion(t, dir, "1.0.0")
	if out, code := runSen(t, dir, "", extraEnv, "sdlc", "gate", "P1"); code != 0 {
		t.Fatalf("expected rolled-back v1.0.0 project to pass gate P1 again:\n%s", out)
	}

	// 7F. Kiểm chứng RenderAntigravityHooksMapWithBin và trình cài đặt hợp nhất khóa "sen-vn" vào hooks.json cô lập mà không xóa hook khác!
	verifyIsolatedInstallerHooksMerge(t)
}

// assertSignedConfigSSDSVersion đối chiếu trường ssds_version trong .sen/config.json và chữ ký SHA-256 trong .sen/config.sha256.
func assertSignedConfigSSDSVersion(t *testing.T, root, wantVer string) {
	t.Helper()
	rawCfg, err := os.ReadFile(filepath.Join(root, ".sen", "config.json"))
	if err != nil {
		t.Fatalf("read config.json: %v", err)
	}
	rawSum, err := os.ReadFile(filepath.Join(root, ".sen", "config.sha256"))
	if err != nil {
		t.Fatalf("read config.sha256: %v", err)
	}
	sum := sha256.Sum256(rawCfg)
	if strings.TrimSpace(string(rawSum)) != hex.EncodeToString(sum[:]) {
		t.Fatalf(".sen/config.sha256 mismatch after version operation")
	}
	var parsed map[string]any
	if err := json.Unmarshal(rawCfg, &parsed); err != nil {
		t.Fatalf("parse config.json: %v", err)
	}
	if parsed["ssds_version"] != wantVer {
		t.Fatalf("expected config.json ssds_version=%q, got %v", wantVer, parsed["ssds_version"])
	}
	if _, tampered := func() (string, bool) {
		_, prof, tamp := bridge.ReadVerifiedRuntimeConfig(root)
		return prof, tamp
	}(); tampered {
		t.Fatalf("ReadVerifiedRuntimeConfig reported tampering on valid signed config")
	}
}

// verifyIsolatedInstallerHooksMerge kiểm tra hợp nhất khóa "sen-vn" vào tệp hooks.json tạm (không đụng tới ~/.gemini thật của máy).
func verifyIsolatedInstallerHooksMerge(t *testing.T) {
	t.Helper()
	hooksMap := bridge.RenderAntigravityHooksMapWithBin(`/opt/sen dir/sen`)
	if len(hooksMap) != 4 {
		t.Fatalf("expected 4 lifecycle events in RenderAntigravityHooksMapWithBin, got %d", len(hooksMap))
	}
	if runtime.GOOS != "windows" {
		return
	}
	tmpInstall := t.TempDir()
	tmpHooks := filepath.Join(t.TempDir(), "hooks.json")
	if err := os.WriteFile(tmpHooks, []byte(`{"existing-user-hook":{"SessionStart":[{"type":"command","command":"echo hi"}]}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	script, err := filepath.Abs(filepath.Join("scripts", "install-sen.ps1"))
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", script,
		"-BinarySource", compiledSenBin, "-InstallDir", tmpInstall, "-SkipUserPath", "-HooksConfigPath", tmpHooks)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("install-sen.ps1 isolated hook merge failed: %v\n%s", err, string(out))
	}
	mergedRaw, err := os.ReadFile(tmpHooks)
	if err != nil {
		t.Fatal(err)
	}
	var merged map[string]any
	if err := json.Unmarshal(mergedRaw, &merged); err != nil {
		t.Fatalf("invalid merged hooks.json: %v", err)
	}
	if _, ok := merged["existing-user-hook"]; !ok {
		t.Fatalf("installer overwrote pre-existing user hook in hooks.json: %s", string(mergedRaw))
	}
	if _, ok := merged["sen-vn"]; !ok {
		t.Fatalf("installer missing 'sen-vn' entry in hooks.json: %s", string(mergedRaw))
	}
}
