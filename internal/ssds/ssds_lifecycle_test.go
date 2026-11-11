// Tệp ssds_lifecycle_test.go kiểm thử trọn vòng đời P0..P8, tường lửa pha, khóa CIA (§17), chống báo láo PASS (§11) và ngoại lệ (§27).
package ssds_test

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/developerfs0110/Sen-VN/internal/ssds"
	"github.com/developerfs0110/Sen-VN/internal/storage"
)

func validCharterMD() string {
	return `# Project Charter
## 1. Project Overview
Expense Tracker Web Application.
## 2. Problem Statement
Users lose track of monthly spending.
## 3. Business Objectives
Provide accurate ledger tracking.
## 4. Target Users
Individual consumers.
## 5. Initial Scope
Add expense and monthly summary.
## 6. Constraints
SQLite single-node deployment.
## 7. Initial Risks
Concurrent write contention.
## 8. Success Metrics
100% ledger accuracy.
`
}

func validRequirementsMD() string {
	return `# Software Requirements Specification
## 1. Project Overview
Expense tracker core requirements.
## 2. Business Objectives
Accurate personal ledger.
## 3. Stakeholders
Product Owner and End Users.
## 4. User Personas
Daily mobile/web budgeter.
## 5. Scope
Expense creation and summary API.
## 6. Out of Scope
Bank wire transfers.
## 7. Functional Requirements
- REQ-F-001: Record categorized expense entry with positive amount.
## 8. Non-functional Requirements
- REQ-NF-001: API p95 latency under 100ms for 1,000 rows.
## 9. User Stories
As a budgeter, I want to log daily expenses, so that I control my monthly budget.
## 10. Business Rules
- REQ-BR-001: Expense amount must be strictly greater than zero.
## 11. Acceptance Criteria
Given a valid category and amount 50000 When the user submits an expense Then the ledger stores it and updates total.
## 12. Constraints
- REQ-CON-001: Zero external cloud API keys required.
## 13. Assumptions
Single currency VND in v1.
## 14. Risks
Invalid negative amount inputs.
## 15. Open Questions
None.
## 16. Requirement Traceability
All REQ-F-001 and REQ-NF-001 map to P2 architecture and P6 tests.
`
}

// TestEndToEndPhaseFirewallGatesCIALockFakePassAndProgressReport kiểm chứng thực thi cứng toàn bộ P0..P8 của SSDS v1.0.0.
func TestEndToEndPhaseFirewallGatesCIALockFakePassAndProgressReport(t *testing.T) {
	root := t.TempDir()
	st, err := storage.Open(filepath.Join(root, ".sen", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// Chưa kích hoạt SSDS mà cố ghi tệp mã nguồn -> bị chặn ngay tại §3.1 firewall!
	if err := ssds.CheckWriteFirewall(st, root, "src/ledger.go"); err == nil {
		t.Fatalf("expected uninitialized SSDS repo to block software source writes")
	}

	det := ssds.DetectSoftwareIntent(root, "Xây dựng ứng dụng web quản lý chi tiêu cá nhân")
	if _, _, err := ssds.BootstrapProject(st, root, "ExpenseWeb", "1.0.0", det); err != nil {
		t.Fatal(err)
	}

	// --- PHA 0: Chặn ghi code, từ chối mục TBD, vượt cổng sang P1 ---
	if err := ssds.CheckWriteFirewall(st, root, "src/ledger.go"); err == nil {
		t.Fatalf("expected P0 firewall to block src/ledger.go")
	}
	writeFile(t, root, "docs/00-project-charter.md", "# Charter\n## Project Overview\nTBD\n")
	if g0Bad, _ := ssds.EvaluatePhaseGate(st, root, "P0"); g0Bad.Passed {
		t.Fatalf("expected placeholder TBD charter to fail P0 gate")
	}
	writeFile(t, root, "docs/00-project-charter.md", validCharterMD())
	if g0, ph0, err := ssds.RunPhaseGate(st, root, "P0"); err != nil || !g0.Passed || ph0.Status != "COMPLETED" {
		t.Fatalf("expected P0 to complete automatically (requires_user_approval=false), got gate=%+v ph=%+v err=%v", g0, ph0, err)
	}

	// --- PHA 1: Chặn code (§6), bắt buộc REVIEW + User Approval (REVIEW_REQUIRED != APPROVED) ---
	if err := ssds.CheckWriteFirewall(st, root, "src/ledger.go"); err == nil {
		t.Fatalf("expected P1 firewall to forbid implementation code")
	}
	if err := ssds.CanBeginImplementationTask(st, root); err == nil {
		t.Fatalf("expected CanBeginImplementationTask blocked during P1")
	}
	writeFile(t, root, "docs/01-requirements.md", validRequirementsMD())
	if g1NoRev, _ := ssds.EvaluatePhaseGate(st, root, "P1"); g1NoRev.Passed {
		t.Fatalf("expected P1 in WF_STANDARD to fail before REVIEW record is logged")
	}
	_, _ = st.AppendGovernanceRecord(storage.GovernanceRecord{
		RecordKind: "REVIEW", PhaseID: "P1", AuthorityLevel: "MAJOR",
		AuthorRole: "Senior Business Analyst", ReviewerRole: "System Analyst",
		EvidenceMode: "INDEPENDENT_AI_REVIEW", Verdict: "PASS", Approved: true,
	})
	g1, ph1, err := ssds.RunPhaseGate(st, root, "P1")
	if err != nil || !g1.Passed || ph1.Status != "REVIEW_REQUIRED" || g1.SyncedReqs < 4 {
		t.Fatalf("expected P1 gate PASS -> REVIEW_REQUIRED (not APPROVED!) with >=4 synced reqs, got gate=%+v ph=%+v err=%v", g1, ph1, err)
	}
	if err := ssds.ApprovePhase(st, root, "P1", "anh Gấu", ""); err == nil {
		t.Fatalf("expected empty userQuote in ApprovePhase to be rejected (§21)")
	}
	if err := ssds.ApprovePhase(st, root, "P1", "anh Gấu", "Duyệt tài liệu yêu cầu P1"); err != nil {
		t.Fatalf("ApprovePhase P1 failed: %v", err)
	}

	// --- PHA 2: Chặn code production (§7), kiểm tra khóa CIA (§17) lên P1 đã duyệt, yêu cầu ADR + User Approval ---
	if err := ssds.CheckWriteFirewall(st, root, "docs/01-requirements.md"); err == nil || !strings.Contains(err.Error(), "Change Impact Analysis") {
		t.Fatalf("expected editing approved docs/01-requirements.md without CIA-* to be blocked, got: %v", err)
	}
	if err := ssds.CheckWriteFirewall(st, root, "src/ledger.go"); err == nil {
		t.Fatalf("expected P2 firewall to block production code")
	}
	writeFile(t, root, "docs/02-architecture.md", `# System Architecture
## Architecture Overview
Modular layered Go service satisfying REQ-F-001 and REQ-NF-001.
## Technology Stack
Go 1.26 + SQLite WAL.
## System Boundaries
HTTP JSON API + local SQLite ledger.
## Components
COMP-LEDGER-001 handles REQ-F-001 and REQ-NF-001.
## Security Boundaries
Input validation and parameterized SQL.
`)
	_, _ = st.AppendGovernanceRecord(storage.GovernanceRecord{
		RecordKind: "REVIEW", PhaseID: "P2", AuthorityLevel: "MAJOR",
		AuthorRole: "Principal Software Architect", ReviewerRole: "Security Architect",
		EvidenceMode: "INDEPENDENT_AI_REVIEW", Verdict: "PASS", Approved: true,
	})
	if g2NoADR, _ := ssds.EvaluatePhaseGate(st, root, "P2"); g2NoADR.Passed {
		t.Fatalf("expected P2 gate to fail when docs/adr/ADR-*.md is missing")
	}
	writeFile(t, root, "docs/adr/ADR-001-sqlite-wal.md", "# ADR-001: SQLite WAL\nUse pure-Go SQLite WAL for REQ-NF-001.\n")
	if g2, ph2, err := ssds.RunPhaseGate(st, root, "P2"); err != nil || !g2.Passed || ph2.Status != "REVIEW_REQUIRED" {
		t.Fatalf("expected P2 gate PASS -> REVIEW_REQUIRED, got gate=%+v ph=%+v err=%v", g2, ph2, err)
	}
	if err := ssds.ApprovePhase(st, root, "P2", "anh Gấu", "Duyệt kiến trúc P2"); err != nil {
		t.Fatal(err)
	}

	// --- PHA 3 & PHA 4: Lập kế hoạch 10 thuộc tính task + ma trận truy vết + build nền ---
	writeFile(t, root, "docs/08-task-breakdown.md", `# Task Breakdown
## Milestones
M1 Core Ledger.
## Tasks
- Task ID: TASK-001 | Description: Implement expense service | Requirements: REQ-F-001, REQ-NF-001 | Component: COMP-LEDGER-001 | Dependencies: None | Expected Outputs: src/ledger.go | Acceptance Criteria: Positive amount saved | Testing: Unit test | Complexity: S | Status: OPEN
`)
	writeFile(t, root, "docs/traceability-matrix.md", `# Traceability Matrix
| Requirement | Architecture | Task | Source Code | Test Case | Release |
|---|---|---|---|---|---|
| REQ-F-001 | COMP-LEDGER-001 | TASK-001 | src/ledger.go | TEST-LEDGER-001 | v1.0.0 |
| REQ-NF-001 | COMP-LEDGER-001 | TASK-001 | src/ledger.go | TEST-PERF-001 | v1.0.0 |
`)
	if g3, _, err := ssds.RunPhaseGate(st, root, "P3"); err != nil || !g3.Passed {
		t.Fatalf("P3 gate failed: %+v (err=%v)", g3, err)
	}

	writeFile(t, root, "README.md", "# ExpenseWeb\n## Installation\ngo build ./...\n## Usage\n./expenseweb\n")
	_, _ = st.AppendEvidence("FOUNDATION", "go build ./...", "BUILD", 0, 0, "ok")
	if g4, _, err := ssds.RunPhaseGate(st, root, "P4"); err != nil || !g4.Passed {
		t.Fatalf("P4 gate failed: %+v (err=%v)", g4, err)
	}

	// --- PHA 5: Kiểm tra CanBeginImplementationTask, thực thi task đủ 4 nấc tới VERIFIED ---
	if err := ssds.CanBeginImplementationTask(st, root); err != nil {
		t.Fatalf("expected CanBeginImplementationTask allowed in P5, got: %v", err)
	}
	if err := ssds.CheckWriteFirewall(st, root, "src/ledger.go"); err != nil {
		t.Fatalf("expected src/ledger.go writable in P5, got: %v", err)
	}
	writeFile(t, root, "src/ledger.go", "package ledger\n")
	_ = st.CreateTask(storage.Task{
		ID: "TASK-001", PhaseID: "P5", ReqIDs: []string{"REQ-F-001", "REQ-NF-001"},
		ArchComponents: []string{"COMP-LEDGER-001"}, Title: "Ledger service", Kind: "FEATURE",
		RiskAuto: "SMALL", ScopeFiles: []string{"src/ledger.go"}, Status: "OPEN",
	})
	_, _ = st.AppendGovernanceRecord(storage.GovernanceRecord{
		RecordKind: "REVIEW", PhaseID: "P5", AuthorityLevel: "MAJOR",
		AuthorRole: "Senior Software Engineer", ReviewerRole: "QA Engineer",
		EvidenceMode: "INDEPENDENT_AI_REVIEW", Verdict: "PASS", Approved: true,
	})
	if g5Early, _ := ssds.EvaluatePhaseGate(st, root, "P5"); g5Early.Passed {
		t.Fatalf("expected P5 gate to fail while TASK-001 is still IMPLEMENTED / NOT_EXECUTED")
	}
	_ = st.UpdateTaskExecutionState("TASK-001", "VERIFIED", "PASS", "VERIFIED")
	if g5NoEv, _ := ssds.EvaluatePhaseGate(st, root, "P5"); g5NoEv.Passed {
		t.Fatalf("expected P5 gate to fail when TASK-001 has no passing evidence row")
	}
	// Ghi bằng chứng BUILD cho TASK-001 (chưa chạy TEST để kiểm thử bẫy báo láo PASS ở P6 ngay sau đây!).
	_, _ = st.AppendEvidence("TASK-001", "go build ./...", "BUILD", 0, 0, "compiled")
	if g5, _, err := ssds.RunPhaseGate(st, root, "P5"); err != nil || !g5.Passed {
		t.Fatalf("P5 gate failed: %+v (err=%v)", g5, err)
	}

	// --- PHA 6: Chặn báo cáo láo PASS (§11), lỗi P0 treo và kiểm tra ngoại lệ quá hạn (§27) ---
	writeFile(t, root, "docs/testing/test-plan.md", "# Test Plan\n## Scope\nCore ledger.\n## Strategy\nAutomated unit & perf.\n")
	writeFile(t, root, "docs/testing/test-cases.md", "# Test Cases\n## Test Cases\nTEST-LEDGER-001 and TEST-PERF-001.\n")
	writeFile(t, root, "docs/testing/test-results.md", "# Test Results\n## Test Results\nTEST-LEDGER-001: PASS\n## Summary\nAll passed.\n")
	writeFile(t, root, "docs/testing/security-report.md", "# Security Report\n## Security\nNo SQLi or secrets.\n")
	writeFile(t, root, "docs/testing/bug-reports.md", "# Bugs\n- BUG-01 [P0] OPEN: Ledger panic on zero\n")
	_, _ = st.AppendGovernanceRecord(storage.GovernanceRecord{
		RecordKind: "REVIEW", PhaseID: "P6", AuthorityLevel: "MAJOR",
		AuthorRole: "QA Lead", ReviewerRole: "Security Engineer",
		EvidenceMode: "INDEPENDENT_AI_REVIEW", Verdict: "PASS", Approved: true,
	})

	g6Fake, _ := ssds.EvaluatePhaseGate(st, root, "P6")
	if g6Fake.Passed || !strings.Contains(strings.Join(g6Fake.Violations, " | "), "unexecuted tests reported as PASS") {
		t.Fatalf("expected P6 anti-fake-PASS check to catch PASS claim without TEST evidence, got %+v", g6Fake)
	}

	// Chạy TEST thật + đóng lỗi P0 -> P6 đạt REVIEW_REQUIRED -> User duyệt!
	_, _ = st.AppendEvidence("TASK-001", "go test -v ./...", "TEST", 2, 0, "--- PASS: TestLedger")
	writeFile(t, root, "docs/testing/bug-reports.md", "# Bugs\n- BUG-01 [P0] RESOLVED: Fixed validation.\n")
	if g6, ph6, err := ssds.RunPhaseGate(st, root, "P6"); err != nil || !g6.Passed || ph6.Status != "REVIEW_REQUIRED" {
		t.Fatalf("expected P6 gate PASS -> REVIEW_REQUIRED, got gate=%+v ph=%+v err=%v", g6, ph6, err)
	}
	if err := ssds.ApprovePhase(st, root, "P6", "anh Gấu", "Duyệt kết quả kiểm thử QA P6"); err != nil {
		t.Fatal(err)
	}

	// Kiểm chứng mở khóa sửa tài liệu đã duyệt bằng CIA-* đủ 10 chiều có phê duyệt CRITICAL (§17).
	if _, err := ssds.RecordChangeImpact(st, "P7", "CRITICAL", "Principal Software Architect", ssds.ChangeImpactSpec{
		TargetDocument: "docs/02-architecture.md", AffectedRequirements: []string{"REQ-F-001"},
		AffectedADRs: []string{"ADR-001"}, AffectedComponents: []string{"COMP-LEDGER-001"},
		AffectedModules: []string{"src/ledger.go"}, AffectedDatabase: "None", AffectedAPIs: "None",
		AffectedTests: []string{"TEST-LEDGER-001"}, AffectedDocs: []string{"docs/02-architecture.md"},
		BackwardCompatibilityRisks: "None", MigrationAndRollback: "No DB change",
	}, true, "Duyệt cập nhật sơ đồ kiến trúc"); err != nil {
		t.Fatalf("RecordChangeImpact failed: %v", err)
	}
	if err := ssds.CheckWriteFirewall(st, root, "docs/02-architecture.md"); err != nil {
		t.Fatalf("expected approved CIA-* to unlock docs/02-architecture.md, got: %v", err)
	}

	// Kiểm chứng đồng bộ tệp trạng thái (§24) và Báo cáo Tiến độ chuẩn 8 mục (§25).
	if err := ssds.SyncProjectStateFiles(st, root); err != nil {
		t.Fatalf("SyncProjectStateFiles failed: %v", err)
	}
	report, err := ssds.RenderStandardProgressReport(st, root)
	if err != nil {
		t.Fatalf("RenderStandardProgressReport failed: %v", err)
	}
	for _, sec := range []string{
		"## 1. Project Status", "## 2. Completed Work", "## 3. Files Changed", "## 4. Testing",
		"## 5. Issues and Risks", "## 6. Decisions", "## 7. Next Actions", "## 8. User Action Required",
	} {
		if !strings.Contains(report, sec) {
			t.Errorf("missing mandatory §25 section %q in report:\n%s", sec, report)
		}
	}

	// Kiểm chứng ngoại lệ quá hạn (§27) tự động khóa cổng chất lượng!
	_, err = ssds.RecordException(st, "P7", "CRITICAL", "Release Manager", ssds.ExceptionSpec{
		ExceptionID: "EXC-001", Reason: "Temporary staging skip", AffectedRequirement: "REQ-NF-001",
		RiskAssessment: "Low", Mitigation: "Local benchmark", Approval: "User approved", Expiration: "2020-01-01",
	})
	if err != nil {
		t.Fatalf("RecordException failed: %v", err)
	}
	if err := ssds.CheckActiveExceptions(st); err == nil || !strings.Contains(err.Error(), "expired") {
		t.Fatalf("expected expired EXC-001 to block CheckActiveExceptions, got: %v", err)
	}
}

