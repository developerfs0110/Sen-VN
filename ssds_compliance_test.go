// Tệp ssds_compliance_test.go kiểm thử đối kháng đầu-cuối (Suites 1–5) cho động cơ SEN-STD-SDLC-001 (SSDS v1.0.0).
package senmvp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/developerfs0110/Sen-VN/internal/storage"
)

// TestSSDSSuite1_ZeroPromptAutoActivationAndMandatoryFirewall kiểm chứng Suite 1: Tự kích hoạt 0 câu nhắc thừa, lọc Q&A và chặn ghi code lén.
func TestSSDSSuite1_ZeroPromptAutoActivationAndMandatoryFirewall(t *testing.T) {
	// 1A. Câu hỏi tra cứu thuần túy ("Giải thích hàm main làm gì") trong thư mục rỗng KHÔNG được tạo .sen/state.db!
	qaDir := t.TempDir()
	invokeHookJSON(t, qaDir, "PreInvocation", map[string]any{
		"hookEventName": "PreInvocation",
		"invocationNum": 1,
		"cwd":           qaDir,
		"lastUserInput": "Giải thích hàm main làm gì",
	})
	if _, err := os.Stat(filepath.Join(qaDir, ".sen", "state.db")); !os.IsNotExist(err) {
		t.Fatalf("pure Q&A prompt must NOT create .sen/state.db in empty folder")
	}

	// 1B. Thư mục chưa khởi tạo bị gọi PreToolUse write_file("main.go") -> chặn cứng theo SSDS §3.1!
	uninitDir := t.TempDir()
	rawDeny, denyResp := invokeHookJSON(t, uninitDir, "PreToolUse", map[string]any{
		"hookEventName": "PreToolUse",
		"cwd":           uninitDir,
		"toolCall": map[string]any{
			"name": "write_file",
			"args": map[string]any{"Path": "main.go"},
		},
	})
	if denyResp["decision"] != "deny" || !strings.Contains(rawDeny, "SSDS Mandatory Activation") {
		t.Fatalf("expected uninitialized software write_file(main.go) denied per §3.1, got: %s", rawDeny)
	}

	// 1C. Thư mục rỗng + yêu cầu tạo phần mềm tự nhiên -> tự động tạo .sen/state.db, ghim v1.0.0 GREENFIELD & bơm banner!
	greenDir := t.TempDir()
	rawAct, _ := invokeHookJSON(t, greenDir, "PreInvocation", map[string]any{
		"hookEventName": "PreInvocation",
		"invocationNum": 1,
		"cwd":           greenDir,
		"lastUserInput": "Viết cho tôi phần mềm quản lý kho hàng",
	})
	if !strings.Contains(rawAct, "[SSDS v1.0.0 — Đã kích hoạt tự động]") || !strings.Contains(rawAct, "GREENFIELD") {
		t.Fatalf("expected auto-activation Workflow Declaration banner in PreInvocation output, got:\n%s", rawAct)
	}
	st, err := storage.Open(filepath.Join(greenDir, ".sen", "state.db"))
	if err != nil {
		t.Fatalf("expected .sen/state.db auto-created: %v", err)
	}
	defer st.Close()
	proj, err := st.GetSSDSProject()
	if err != nil || proj == nil || proj.AppliedSSDSVersion != "1.0.0" || proj.Category != "GREENFIELD" {
		t.Fatalf("unexpected auto-bootstrapped ssds_project: %+v (err=%v)", proj, err)
	}
	if _, err := os.Stat(filepath.Join(greenDir, "PROJECT_STATUS.md")); err != nil {
		t.Fatalf("expected PROJECT_STATUS.md synced on auto-activation: %v", err)
	}
}

// TestSSDSSuite2_PhaseFirewallAntiSkipAndReviewRequiredSeparation kiểm chứng Suite 2: Tường lửa P0..P2, chống tài liệu rỗng và tách REVIEW_REQUIRED != APPROVED.
func TestSSDSSuite2_PhaseFirewallAntiSkipAndReviewRequiredSeparation(t *testing.T) {
	dir := t.TempDir()
	mustSenOK(t, dir, "sdlc", "classify", "--prompt", "Xây dựng ứng dụng web quản lý kho hàng", "--workflow", "WF_STANDARD")
	mustSenOK(t, dir, "intake", "--quote", "Quản lý kho", "--req", "Quản lý tồn kho")

	// Ở P0: cả PreToolUse write_file("src/app.go") lẫn `sen begin` đều bị từ chối!
	_, p0Resp := invokeHookJSON(t, dir, "PreToolUse", map[string]any{
		"hookEventName": "PreToolUse", "cwd": dir,
		"toolCall": map[string]any{"name": "write_file", "args": map[string]any{"Path": "src/app.go"}},
	})
	if p0Resp["decision"] != "deny" {
		t.Fatalf("expected P0 write_file(src/app.go) denied, got: %+v", p0Resp)
	}
	if out, code := runSen(t, dir, "", nil, "begin", "T1", "--reqs", "R1", "--files", "src/app.go"); code == 0 {
		t.Fatalf("expected sen begin in P0 to fail, got 0:\n%s", out)
	}

	// Hoàn thành P0 để sang P1.
	writeRepoFile(t, dir, "docs/00-project-charter.md", validCharterMD("Warehouse App"))
	mustSenOK(t, dir, "sdlc", "gate", "P0")

	// Ở P1: ghi code vẫn bị chặn; tài liệu 01-requirements.md chứa TODO hoặc thiếu Given/When/Then bị gate P1 từ chối!
	_, p1CodeResp := invokeHookJSON(t, dir, "PreToolUse", map[string]any{
		"hookEventName": "PreToolUse", "cwd": dir,
		"toolCall": map[string]any{"name": "write_file", "args": map[string]any{"Path": "src/app.go"}},
	})
	if p1CodeResp["decision"] != "deny" {
		t.Fatalf("expected P1 write_file(src/app.go) denied, got: %+v", p1CodeResp)
	}
	writeRepoFile(t, dir, "docs/01-requirements.md", "# Requirements\n\n## Project Overview\nTODO\n")
	if out, code := runSen(t, dir, "", nil, "sdlc", "gate", "P1"); code == 0 {
		t.Fatalf("expected hollow TODO requirements to fail gate P1, got 0:\n%s", out)
	}

	// Tài liệu đủ 16 mục + biên bản phản biện hợp lệ -> gate P1 PASS và dừng ở REVIEW_REQUIRED (chưa sang P2!).
	writeRepoFile(t, dir, "docs/01-requirements.md", validRequirementsMD("Nhập kho sản phẩm", "Xuất kho an toàn"))
	mustSenOK(t, dir, "sdlc", "review", "--phase", "P1", "--author", "Senior Business Analyst", "--reviewer", "System Analyst", "--ref", "sub-ba-1", "--verdict", "PASS")
	gateP1Out := mustSenOK(t, dir, "sdlc", "gate", "P1")
	if !strings.Contains(gateP1Out, "REVIEW_REQUIRED") {
		t.Fatalf("expected gate P1 to enter REVIEW_REQUIRED awaiting human approval, got:\n%s", gateP1Out)
	}

	// Khi đang REVIEW_REQUIRED (chưa approve), cố tình ghi docs/02-architecture.md của P2 vẫn bị chặn!
	_, preApproveP2 := invokeHookJSON(t, dir, "PreToolUse", map[string]any{
		"hookEventName": "PreToolUse", "cwd": dir,
		"toolCall": map[string]any{"name": "write_file", "args": map[string]any{"Path": "docs/02-architecture.md"}},
	})
	if preApproveP2["decision"] != "deny" {
		t.Fatalf("expected P2 deliverable blocked while P1 is only REVIEW_REQUIRED, got: %+v", preApproveP2)
	}

	// Sau khi User phê duyệt tường minh -> chuyển sang P2, cho phép ghi docs/02-architecture.md nhưng VẪN chặn src/app.go!
	mustSenOK(t, dir, "sdlc", "approve", "--phase", "P1", "--quote", "Đồng ý P1")
	_, allowP2Doc := invokeHookJSON(t, dir, "PreToolUse", map[string]any{
		"hookEventName": "PreToolUse", "cwd": dir,
		"toolCall": map[string]any{"name": "write_file", "args": map[string]any{"Path": "docs/02-architecture.md"}},
	})
	if allowP2Doc["decision"] != "allow" {
		t.Fatalf("expected docs/02-architecture.md allowed in P2 after P1 approval, got: %+v", allowP2Doc)
	}
	_, p2CodeResp := invokeHookJSON(t, dir, "PreToolUse", map[string]any{
		"hookEventName": "PreToolUse", "cwd": dir,
		"toolCall": map[string]any{"name": "write_file", "args": map[string]any{"Path": "src/app.go"}},
	})
	if p2CodeResp["decision"] != "deny" {
		t.Fatalf("expected src/app.go still denied in P2, got: %+v", p2CodeResp)
	}
}

// TestSSDSSuite3_RoleSeparationAndHonestReviewDisclosure kiểm chứng Suite 3: Cấm tự duyệt trùng vai trò và tự động hạ bậc thiếu --ref.
func TestSSDSSuite3_RoleSeparationAndHonestReviewDisclosure(t *testing.T) {
	dir := t.TempDir()
	mustSenOK(t, dir, "sdlc", "classify", "--prompt", "Tạo hệ thống API quản lý đơn hàng", "--workflow", "WF_STANDARD")

	// Trùng vai trò author == reviewer bị từ chối thẳng!
	outSame, codeSame := runSen(t, dir, "", nil, "sdlc", "review",
		"--phase", "P2",
		"--author", "Principal Software Architect",
		"--reviewer", "Principal Software Architect",
		"--verdict", "PASS")
	if codeSame == 0 || !strings.Contains(outSame, "SSDS §4.3 violation") {
		t.Fatalf("expected author == reviewer to be rejected, got code=%d:\n%s", codeSame, outSame)
	}

	// Khai báo INDEPENDENT_AI_REVIEW nhưng không truyền --ref -> tự động hạ xuống SELF_REVIEW kèm cảnh báo trung thực!
	downgradeOut := mustSenOK(t, dir, "sdlc", "review",
		"--phase", "P2",
		"--author", "Principal Software Architect",
		"--reviewer", "Security Architect",
		"--mode", "INDEPENDENT_AI_REVIEW",
		"--verdict", "PASS")
	if !strings.Contains(downgradeOut, "mode=SELF_REVIEW") || !strings.Contains(downgradeOut, "HONEST REVIEW DISCLOSURE") {
		t.Fatalf("expected downgrade to SELF_REVIEW with honest disclosure, got:\n%s", downgradeOut)
	}

	// Có --ref từ phiên subagent độc lập -> giữ nguyên mode=INDEPENDENT_AI_REVIEW.
	indepOut := mustSenOK(t, dir, "sdlc", "review",
		"--phase", "P2",
		"--author", "Principal Software Architect",
		"--reviewer", "Security Architect",
		"--mode", "INDEPENDENT_AI_REVIEW",
		"--ref", "subagent-sec-001",
		"--verdict", "PASS")
	if !strings.Contains(indepOut, "mode=INDEPENDENT_AI_REVIEW") {
		t.Fatalf("expected INDEPENDENT_AI_REVIEW preserved with --ref, got:\n%s", indepOut)
	}
}

// TestSSDSSuite4_FullStandardLifecycleTraceabilityCIAAndAntiFakePass kiểm chứng Suite 4: P0->P8->sen done, khóa CIA §17, chặn PASS giả và truy vết 6 mắt xích.
func TestSSDSSuite4_FullStandardLifecycleTraceabilityCIAAndAntiFakePass(t *testing.T) {
	dir := t.TempDir()
	mustSenOK(t, dir, "sdlc", "classify", "--prompt", "Xây dựng phần mềm quản lý tồn kho chuẩn", "--workflow", "WF_STANDARD")

	// P0 -> P1 -> P2 -> P3 -> P4
	writeRepoFile(t, dir, "docs/00-project-charter.md", validCharterMD("StockCore"))
	mustSenOK(t, dir, "sdlc", "gate", "P0")

	writeRepoFile(t, dir, "docs/01-requirements.md", validRequirementsMD("Cộng tồn kho mặt hàng", "Trừ tồn kho có kiểm tra số dư"))
	mustSenOK(t, dir, "sdlc", "review", "--phase", "P1", "--author", "Senior Business Analyst", "--reviewer", "System Analyst", "--ref", "rev-p1", "--verdict", "PASS")
	mustSenOK(t, dir, "sdlc", "gate", "P1")
	mustSenOK(t, dir, "sdlc", "approve", "--phase", "P1", "--quote", "Duyệt đặc tả yêu cầu P1")

	archV1 := "# Architecture\n## Architecture\nModular Go service.\n## Technology Stack\nGo 1.26.\n## System Boundaries\nIn-process ledger.\n## Components\nINV-001 implements REQ-F-001 and REQ-NF-001; INV-002 implements REQ-F-002.\n## Security\nStrict non-negative validation.\n"
	writeRepoFile(t, dir, "docs/02-architecture.md", archV1)
	writeRepoFile(t, dir, "docs/adr/ADR-001-ledger.md", "# ADR-001: In-Memory Ledger\nAccepted deterministic map store.\n")
	mustSenOK(t, dir, "sdlc", "review", "--phase", "P2", "--author", "Principal Software Architect", "--reviewer", "Security Architect", "--ref", "rev-p2", "--verdict", "PASS")
	mustSenOK(t, dir, "sdlc", "gate", "P2")
	mustSenOK(t, dir, "sdlc", "approve", "--phase", "P2", "--quote", "Duyệt thiết kế kiến trúc P2")

	breakdown := "# Task Breakdown\n## Milestones\nM1 Core Stock.\n## Tasks\n- TASK-001 | REQ-F-001, REQ-F-002, REQ-NF-001 | Component: INV-001, INV-002 | Dependencies: none | Output: stock.go | Acceptance: Given/When/Then | Test: stock_test.go | Complexity: SMALL | Status: Planned\n"
	writeRepoFile(t, dir, "docs/08-task-breakdown.md", breakdown)
	fullMatrix := "| Req | Component | Task | Source | Test | Release |\n|---|---|---|---|---|---|\n| REQ-F-001 | INV-001 | TASK-001 | stock.go | TestStockOps | v1.0.0 |\n| REQ-F-002 | INV-002 | TASK-001 | stock.go | TestStockOps | v1.0.0 |\n| REQ-NF-001 | INV-001 | TASK-001 | stock.go | TestStockOps | v1.0.0 |\n"
	writeRepoFile(t, dir, "docs/traceability-matrix.md", fullMatrix)
	mustSenOK(t, dir, "sdlc", "gate", "P3")

	// P4 Foundation: tạo README.md + go.mod qua task nền tảng và chạy `go version` để ghi nhận bằng chứng nền.
	writeRepoFile(t, dir, "README.md", "# StockCore\n\n## Installation\n`go build ./...`\n\n## Usage\nImport package `stockcore`.\n")
	mustSenOK(t, dir, "sdlc", "sync")
	mustSenOK(t, dir, "begin", "TASK-FOUNDATION", "--phase", "P4", "--reqs", "REQ-F-001", "--files", "go.mod")
	writeRepoFile(t, dir, "go.mod", "module stockcore\n\ngo 1.22\n")
	mustSenOK(t, dir, "verify", "TASK-FOUNDATION", "--", "go", "version")
	mustSenOK(t, dir, "sdlc", "gate", "P4")

	// Đang ở P5: cố tình sửa docs/02-architecture.md đã APPROVED -> bị PreToolUse khóa cứng cho tới khi lập CIA-*!
	rawCIABlock, ciaDeny := invokeHookJSON(t, dir, "PreToolUse", map[string]any{
		"hookEventName": "PreToolUse", "cwd": dir,
		"toolCall": map[string]any{"name": "edit_file", "args": map[string]any{"Path": "docs/02-architecture.md"}},
	})
	if ciaDeny["decision"] != "deny" || !strings.Contains(rawCIABlock, "Change Impact Analysis lock") {
		t.Fatalf("expected approved docs/02-architecture.md locked in P5 before CIA, got: %s", rawCIABlock)
	}
	mustSenOK(t, dir, "sdlc", "impact", "--phase", "P5", "--level", "MAJOR",
		"--summary", "Document thread-safe stock map note in INV-002",
		"--doc", "docs/02-architecture.md", "--reqs", "REQ-F-002", "--adrs", "ADR-001",
		"--components", "INV-002", "--modules", "stock.go", "--db", "none", "--apis", "none",
		"--tests", "stock_test.go", "--docs", "docs/02-architecture.md",
		"--compat", "backward-compatible", "--rollback", "git/sen undo")
	_, ciaAllow := invokeHookJSON(t, dir, "PreToolUse", map[string]any{
		"hookEventName": "PreToolUse", "cwd": dir,
		"toolCall": map[string]any{"name": "edit_file", "args": map[string]any{"Path": "docs/02-architecture.md"}},
	})
	if ciaAllow["decision"] != "allow" {
		t.Fatalf("expected docs/02-architecture.md unlocked after CIA-001, got: %+v", ciaAllow)
	}
	writeRepoFile(t, dir, "docs/02-architecture.md", archV1+"\nUpdated note under CIA-001.\n")
	// Sau khi sửa tài liệu baseline dưới CIA-001, bắt buộc niêm phong lại mã băm P2 qua gate P2 + approve P2 trước khi code P5!
	if out, code := runSen(t, dir, "", nil, "begin", "TASK-001", "--phase", "P5", "--reqs", "REQ-F-001", "--files", "stock.go"); code == 0 {
		t.Fatalf("expected un-re-sealed CIA edit on docs/02-architecture.md to block sen begin, got 0:\n%s", out)
	}
	mustSenOK(t, dir, "sdlc", "gate", "P2")
	mustSenOK(t, dir, "sdlc", "approve", "--phase", "P2", "--quote", "Duyệt cập nhật kiến trúc CIA-001")
	mustSenOK(t, dir, "sdlc", "sync")

	// Kiểm chứng chống báo cáo láo PASS tại P6 khi chưa chạy bất kỳ bài kiểm thử TEST thực nào!
	writeRepoFile(t, dir, "docs/testing/test-plan.md", "# Test Plan\n## Scope\nUnit tests.\n## Strategy\nDeterministic Go tests.\n")
	writeRepoFile(t, dir, "docs/testing/test-cases.md", "# Cases\n## Test Cases\nTC-01 add & deduct stock.\n")
	writeRepoFile(t, dir, "docs/testing/test-results.md", "# Results\n## Test Results\nTC-01: PASS\n## Summary\nAll green.\n")
	writeRepoFile(t, dir, "docs/testing/security-report.md", "# Security\n## Security\nNo P0/P1 findings.\n")
	mustSenOK(t, dir, "sdlc", "review", "--phase", "P6", "--author", "Senior Software Engineer", "--reviewer", "Security Engineer", "--ref", "rev-p6", "--verdict", "PASS")
	mustSenOK(t, dir, "sdlc", "sync")
	fakeOut, fakeCode := runSen(t, dir, "", nil, "sdlc", "gate", "P6")
	if fakeCode == 0 || !strings.Contains(fakeOut, "unexecuted tests reported as PASS") {
		t.Fatalf("expected fake PASS in test-results.md before real TEST run to FAIL gate P6, got code=%d:\n%s", fakeCode, fakeOut)
	}

	// Triển khai mã nguồn thật trong P5 và chạy `sen verify` với `go test -v ./...`.
	mustSenOK(t, dir, "begin", "TASK-001", "--phase", "P5", "--components", "INV-001,INV-002", "--reqs", "REQ-F-001,REQ-F-002,REQ-NF-001", "--files", "stock.go,stock_test.go")
	writeRepoFile(t, dir, "stock.go", "package stockcore\n\nimport \"errors\"\n\nfunc Adjust(cur, delta int) (int, error) {\n\tif cur+delta < 0 { return cur, errors.New(\"insufficient stock\") }\n\treturn cur + delta, nil\n}\n")
	writeRepoFile(t, dir, "stock_test.go", "package stockcore\n\nimport \"testing\"\n\nfunc TestStockOps(t *testing.T) {\n\tv, err := Adjust(0, 15)\n\tif err != nil || v != 15 { t.Fatal(err) }\n\tif _, err := Adjust(v, -20); err == nil { t.Fatal(\"expected insufficient stock\") }\n}\n")
	mustSenOK(t, dir, "verify", "TASK-001", "--", "go", "test", "-v", "./...")
	mustSenOK(t, dir, "sdlc", "review", "--phase", "P5", "--author", "Senior Software Engineer", "--reviewer", "QA Engineer", "--ref", "rev-p5", "--verdict", "PASS")
	mustSenOK(t, dir, "sdlc", "gate", "P5")

	// Bỏ sót REQ-F-002 khỏi docs/traceability-matrix.md -> gate P6 phải BẮT LỖI!
	incompleteMatrix := "| Req | Component | Task | Source | Test | Release |\n|---|---|---|---|---|---|\n| REQ-F-001 | INV-001 | TASK-001 | stock.go | TestStockOps | v1.0.0 |\n| REQ-NF-001 | INV-001 | TASK-001 | stock.go | TestStockOps | v1.0.0 |\n"
	writeRepoFile(t, dir, "docs/traceability-matrix.md", incompleteMatrix)
	mustSenOK(t, dir, "sdlc", "sync")
	omitOut, omitCode := runSen(t, dir, "", nil, "sdlc", "gate", "P6")
	if omitCode == 0 || !strings.Contains(omitOut, "REQ-F-002") {
		t.Fatalf("expected omitting REQ-F-002 from traceability-matrix.md to FAIL gate P6, got code=%d:\n%s", omitCode, omitOut)
	}

	// Khôi phục đủ 6 mắt xích cho REQ-F-002 -> vượt qua P6, P7, P8 và hoàn tất `sen done`!
	writeRepoFile(t, dir, "docs/traceability-matrix.md", fullMatrix)
	mustSenOK(t, dir, "sdlc", "gate", "P6")
	mustSenOK(t, dir, "sdlc", "approve", "--phase", "P6", "--quote", "QA và User nghiệm thu P6")

	writeRepoFile(t, dir, "docs/release-plan.md", "# Release v1.0.0\n## Release Notes\nInitial verified release.\n## Deployment\nSingle binary rollout.\n## Rollback Plan\nRestore previous binary artifact.\n")
	mustSenOK(t, dir, "sdlc", "gate", "P7")
	mustSenOK(t, dir, "sdlc", "approve", "--phase", "P7", "--quote", "Phê duyệt phát hành v1.0.0")

	writeRepoFile(t, dir, "CHANGELOG.md", "# Changelog\n\n## v1.0.0\n- Verified warehouse stock engine.\n")
	mustSenOK(t, dir, "sdlc", "gate", "P8")

	doneOut := mustSenOK(t, dir, "done")
	if !strings.Contains(doneOut, "Standard: SEN-STD-SDLC-001 v1.0.0") {
		t.Fatalf("expected SSDS metadata in sen done output, got:\n%s", doneOut)
	}
}

// TestSSDSSuite5_DecisionAuthorityExpressBrakeAndExpiredExceptions kiểm chứng Suite 5: Chặn quote rỗng, phanh WF_CRITICAL dưới --express và chặn EXC quá hạn.
func TestSSDSSuite5_DecisionAuthorityExpressBrakeAndExpiredExceptions(t *testing.T) {
	dir := t.TempDir()
	writeRepoFile(t, dir, "go.mod", "module tradingcrit\n\ngo 1.22\n")
	writeRepoFile(t, dir, "auth/ledger.go", "package auth\n\nfunc Verify() bool { return true }\n")
	mustSenOK(t, dir, "sdlc", "classify", "--prompt", "Xây dựng hệ thống thanh toán giao dịch chứng khoán", "--workflow", "WF_CRITICAL")

	// 5A. Silence != Approval (§21): truyền --quote "" bị từ chối!
	if out, code := runSen(t, dir, "", nil, "sdlc", "approve", "--phase", "P1", "--quote", ""); code == 0 || !strings.Contains(out, "silence is never approval") {
		t.Fatalf("expected empty approval quote rejected, got code=%d:\n%s", code, out)
	}

	// 5B. Bật --express trong WF_CRITICAL vẫn phải phanh cứng ("force_ask") trên đường dẫn trọng yếu auth/schema!
	st, err := storage.Open(filepath.Join(dir, ".sen", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	for _, pid := range []string{"P0", "P1", "P2", "P3", "P4"} {
		ph, _ := st.GetSSDSPhase(pid)
		ph.GatePassed = true
		ph.Status = "COMPLETED"
		_ = st.UpsertSSDSPhase(*ph)
	}
	_ = st.UpdateCurrentPhase("P5")
	_, _ = st.AddRequirement("REQ-F-001", "Đặt lệnh an toàn", "Xác thực chữ ký lệnh", false)
	_ = st.Close()

	mustSenOK(t, dir, "begin", "T-CRIT", "--express", "--reqs", "REQ-F-001", "--files", "auth/ledger.go,auth/ledger_test.go")
	rawBrake, brakeResp := invokeHookJSON(t, dir, "PreToolUse", map[string]any{
		"hookEventName": "PreToolUse", "cwd": dir,
		"toolCall": map[string]any{"name": "edit_file", "args": map[string]any{"Path": "auth/ledger.go"}},
	})
	if brakeResp["decision"] != "force_ask" || !strings.Contains(rawBrake, "WF_CRITICAL") {
		t.Fatalf("expected WF_CRITICAL under --express to return force_ask on auth/ledger.go, got: %s", rawBrake)
	}
	writeRepoFile(t, dir, "auth/ledger_test.go", "package auth\n\nimport \"testing\"\nfunc TestVerify(t *testing.T) { if !Verify() { t.Fatal() } }\n")
	mustSenOK(t, dir, "verify", "T-CRIT", "--", "go", "test", "-v", "./...")

	// 5C. Ngoại lệ EXC-001 đã hết hạn (2020-01-01T00:00:00Z) bắt buộc làm trượt cả `sen sdlc gate` lẫn `sen done`!
	mustSenOK(t, dir, "sdlc", "exception",
		"--id", "EXC-001", "--phase", "P5", "--req", "REQ-F-001",
		"--reason", "Temporary hotfix bypass", "--risk", "Medium", "--mitigation", "Manual audit",
		"--approval", "User temporary sign-off", "--expires", "2020-01-01T00:00:00Z")
	if out, code := runSen(t, dir, "", nil, "sdlc", "gate", "P5"); code == 0 || !strings.Contains(out, "EXC-001 expired") {
		t.Fatalf("expected expired EXC-001 to FAIL gate P5, got code=%d:\n%s", code, out)
	}
	if out, code := runSen(t, dir, "", nil, "done"); code == 0 || !strings.Contains(out, "EXC-001 expired") {
		t.Fatalf("expected expired EXC-001 to FAIL sen done, got code=%d:\n%s", code, out)
	}
}
