// Tệp ssds_test.go kiểm thử bộ nhận diện song ngữ 15 trường hợp §1.3, khảo sát Brownfield §2.3, phản biện độc lập §4.3 và đa phiên bản SSDS.
package ssds_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/developerfs0110/Sen-VN/internal/ssds"
	"github.com/developerfs0110/Sen-VN/internal/storage"
)

func writeFile(t *testing.T, root, rel, content string) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", rel, err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", rel, err)
	}
}

// TestDetectorAllApplicabilityCasesBilingualAndZeroFalsePositive kiểm chứng nhận diện cả tiếng Việt có dấu/không dấu, tiếng Anh và lọc hỏi đáp/media.
func TestDetectorAllApplicabilityCasesBilingualAndZeroFalsePositive(t *testing.T) {
	emptyDir := t.TempDir()

	positiveCases := []struct {
		prompt   string
		wantCat  string
		wantTier string
	}{
		{"Viết cho tôi ứng dụng quản lý chi tiêu cá nhân trên web", "GREENFIELD", "WF_STANDARD"},
		{"Viet ung dung mobile dat lich kham y te truc tuyen", "GREENFIELD", "WF_CRITICAL"},
		{"Xây dựng hệ thống bot giao dịch tài chính đặt lệnh cắt lỗ tự động", "GREENFIELD", "WF_CRITICAL"},
		{"Create a small utility script to rename local CSV files", "GREENFIELD", "WF_MICRO"},
		{"Develop an AI-powered RAG backend API service", "GREENFIELD", "WF_STANDARD"},
		{"Tich hop cong thanh toan Stripe webhook vao he thong", "INTEGRATION", "WF_CRITICAL"},
		{"Migrate database schema from MySQL to PostgreSQL", "MIGRATION", "WF_CRITICAL"},
		{"Build a quick prototype PoC for websocket chat", "PROTOTYPE", "WF_STANDARD"},
		{"Nghiên cứu khả thi kỹ thuật cho kiến trúc microservice", "RESEARCH", "WF_MICRO"},
	}
	for _, tc := range positiveCases {
		got := ssds.DetectSoftwareIntent(emptyDir, tc.prompt)
		if !got.IsSoftwareRequest || got.IsReadOnlyLookup {
			t.Fatalf("prompt %q: expected IsSoftwareRequest=true, got %+v", tc.prompt, got)
		}
		if got.Category != tc.wantCat || got.SuggestedWorkflow != tc.wantTier {
			t.Errorf("prompt %q: got (%s, %s), want (%s, %s)", tc.prompt, got.Category, got.SuggestedWorkflow, tc.wantCat, tc.wantTier)
		}
		if len(got.RiskDimensions) != 8 {
			t.Errorf("prompt %q: expected 8 risk dimensions, got %d", tc.prompt, len(got.RiskDimensions))
		}
	}

	// Dự án Brownfield có sẵn mã nguồn: phân biệt sửa lỗi nhỏ (MAINTENANCE) và mở rộng tính năng (BROWNFIELD).
	existingDir := t.TempDir()
	writeFile(t, existingDir, "go.mod", "module sample\n\ngo 1.26\n")
	writeFile(t, existingDir, "svc.go", "package sample\n")

	bf := ssds.DetectSoftwareIntent(existingDir, "Thêm tính năng xuất báo cáo CSV cho module hiện tại")
	if !bf.IsSoftwareRequest || !bf.HasExistingCode || bf.Category != "BROWNFIELD" {
		t.Fatalf("expected BROWNFIELD on existing code repo, got %+v", bf)
	}
	maint := ssds.DetectSoftwareIntent(existingDir, "Sua loi nho hien thi tieu de")
	if !maint.IsSoftwareRequest || maint.Category != "MAINTENANCE" || maint.SuggestedWorkflow != "WF_MICRO" {
		t.Fatalf("expected MAINTENANCE / WF_MICRO for minor bugfix, got %+v", maint)
	}

	// Bộ lọc chống kích hoạt nhầm: câu hỏi giải thích thuần túy -> IsSoftwareRequest=false, IsReadOnlyLookup=true.
	for _, q := range []string{
		"Giải thích hàm tính băm SHA-256 này hoạt động thế nào?",
		"Tai sao ham nay tra ve nil?",
		"Explain how the authentication middleware works",
	} {
		got := ssds.DetectSoftwareIntent(existingDir, q)
		if got.IsSoftwareRequest || !got.IsReadOnlyLookup {
			t.Errorf("lookup prompt %q should NOT trigger SSDS creation workflow, got %+v", q, got)
		}
	}

	// Dự án Video thuần túy không chứa yêu cầu lập trình -> IsSoftwareRequest=false.
	videoDir := t.TempDir()
	writeFile(t, videoDir, "raw/clip.mp4", "video-bytes")
	vidDet := ssds.DetectSoftwareIntent(videoDir, "Cắt ghép video thêm phụ đề tiếng Việt SRT chuẩn -14 LUFS")
	if vidDet.IsSoftwareRequest {
		t.Fatalf("expected pure video editing request not to activate SSDS, got %+v", vidDet)
	}
}

// TestBrownfieldAuditAndRoleSeparationHonestReview kiểm chứng khảo sát kho mã §2.3, danh mục 27 vai trò và chống giả mạo Review §4.3.
func TestBrownfieldAuditAndRoleSeparationHonestReview(t *testing.T) {
	repo := t.TempDir()
	st, err := storage.Open(filepath.Join(repo, ".sen", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	writeFile(t, repo, "go.mod", "module brownfield\n\ngo 1.26\n")
	writeFile(t, repo, "internal/app.go", "package internal\n")
	writeFile(t, repo, "internal/app_test.go", "package internal\n")
	writeFile(t, repo, "docs/adr/ADR-001-sqlite.md", "# ADR 001\nUse SQLite WAL.\n")

	det := ssds.DetectSoftwareIntent(repo, "Refactor internal service layer")
	proj, phases, err := ssds.BootstrapProject(st, repo, "BrownfieldApp", "1.0.0", det)
	if err != nil {
		t.Fatalf("BootstrapProject: %v", err)
	}
	if proj.BrownfieldAudited {
		t.Fatalf("expected BrownfieldAudited=false before InspectBrownfieldRepo")
	}
	decl := ssds.FormatActivationDeclaration(proj, phases, "1.0.0")
	if !strings.Contains(decl, "[SSDS v1.0.0 — Đã kích hoạt tự động") || !strings.Contains(decl, "P1, P2, P6, P7") {
		t.Fatalf("unexpected Workflow Declaration:\n%s", decl)
	}

	audit, err := ssds.InspectBrownfieldRepo(repo)
	if err != nil {
		t.Fatalf("InspectBrownfieldRepo failed: %v", err)
	}
	if audit.SourceFiles < 2 || audit.TestFiles != 1 || len(audit.ExistingADRs) != 1 {
		t.Fatalf("unexpected BrownfieldAudit summary: %+v", audit)
	}
	reloaded, _ := st.GetSSDSProject()
	if !reloaded.BrownfieldAudited {
		t.Fatalf("expected InspectBrownfieldRepo to mark SetBrownfieldAudited(true)")
	}

	if gotRoles := ssds.AllEngineeringRoles(); len(gotRoles) != 27 {
		t.Fatalf("expected 27 specialized SSDS roles (§4.1), got %d", len(gotRoles))
	}

	// 1. Trùng vai trò người viết và người phản biện (kể cả khác tiền tố Principal) -> bị từ chối!
	if _, _, err := ssds.ValidateIndependentReview("P2", "Principal Software Architect", "Software Architect", "INDEPENDENT_AI_REVIEW", true); err == nil {
		t.Fatalf("expected same-perspective reviewer_role to be rejected under SSDS §4.3")
	}

	// 2. Khai báo INDEPENDENT_AI_REVIEW khi không có phiên/subagent độc lập -> hạ xuống SELF_REVIEW kèm cảnh báo bắt buộc!
	normMode, warn, err := ssds.ValidateIndependentReview("P2", "Principal Software Architect", "Security Architect", "INDEPENDENT_AI_REVIEW", false)
	if err != nil || normMode != "SELF_REVIEW" || !strings.Contains(warn, "Self-review must not be represented as independent verification") {
		t.Fatalf("expected downgrade to SELF_REVIEW with §4.3 disclosure, got mode=%q warn=%q err=%v", normMode, warn, err)
	}
}

// TestMultiVersionRegistryDryRunCheckControlledUpgradeAndRollback kiểm chứng ghim phiên bản, kiểm tra tương thích, nâng cấp CRITICAL và hoàn tác.
func TestMultiVersionRegistryDryRunCheckControlledUpgradeAndRollback(t *testing.T) {
	baseRP, err := ssds.LoadRulepack("1.0.0")
	if err != nil {
		t.Fatalf("LoadRulepack 1.0.0 failed: %v", err)
	}
	if !strings.Contains(ssds.ChangelogMarkdown(), "SEN-STD-SDLC-001") {
		t.Fatalf("expected embedded SSDS_CHANGELOG.md content")
	}

	// Đăng ký bản giả lập 1.1.0 (MINOR) và 2.0.0 (MAJOR thêm mục bắt buộc mới ở P0).
	rp11 := *baseRP
	rp11.Version = "1.1.0"
	rp20 := *baseRP
	rp20.Version = "2.0.0"
	rp20.Phases = make([]ssds.PhaseSpec, len(baseRP.Phases))
	copy(rp20.Phases, baseRP.Phases)
	rp20.Phases[0].RequiredHeadings = map[string][]string{
		"docs/00-project-charter.md": {"Project Overview", "Quantum Safety Assessment"},
	}
	ssds.RegisterRulepackForTest(rp11)
	ssds.RegisterRulepackForTest(rp20)
	defer ssds.UnregisterRulepackForTest("1.1.0")
	defer ssds.UnregisterRulepackForTest("2.0.0")

	versions := ssds.ListBundledVersions()
	if strings.Join(versions, ",") != "1.0.0,1.1.0,2.0.0" {
		t.Fatalf("expected sorted bundled versions 1.0.0,1.1.0,2.0.0, got %v", versions)
	}

	root := t.TempDir()
	st, err := storage.Open(filepath.Join(root, ".sen", "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	det := ssds.DetectSoftwareIntent(root, "Create web invoicing application")
	proj, phases, err := ssds.BootstrapProject(st, root, "InvoiceApp", "1.0.0", det)
	if err != nil {
		t.Fatal(err)
	}
	declWithHint := ssds.FormatActivationDeclaration(proj, phases, "2.0.0")
	if !strings.Contains(declWithHint, "v2.0.0") {
		t.Fatalf("expected declaration banner to notify newer bundled SSDS v2.0.0, got:\n%s", declWithHint)
	}

	writeFile(t, root, "docs/00-project-charter.md", `# Charter
## Project Overview
Invoicing web app.
## Problem Statement
Manual billing is slow.
## Business Objectives
Cut billing time by 80%.
## Target Users
Accountants.
## Initial Scope
Invoice CRUD and PDF export.
## Constraints
Pure Go + SQLite.
## Initial Risks
Tax rounding edge cases.
## Success Metrics
Zero calculation errors.
`)
	// Dự án đang ghim 1.0.0 vẫn vượt cổng P0 xanh 100% dù hệ thống đã có 2.0.0!
	gateP0, err := ssds.EvaluatePhaseGate(st, root, "P0")
	if err != nil || !gateP0.Passed {
		t.Fatalf("expected pinned v1.0.0 project to pass P0 gate cleanly, got %+v (err=%v)", gateP0, err)
	}

	// Dry-run --check 2.0.0 phát hiện đúng nhảy MAJOR và thiếu mục Quantum Safety Assessment mà không sửa DB!
	chk, err := ssds.CheckUpgradeCompatibility(st, root, "2.0.0")
	if err != nil {
		t.Fatalf("CheckUpgradeCompatibility failed: %v", err)
	}
	if chk.Compatible || len(chk.BreakingDiffs) == 0 || len(chk.MissingSections) == 0 {
		t.Fatalf("expected incompatible dry-run report for 2.0.0, got %+v", chk)
	}
	afterCheck, _ := st.GetSSDSProject()
	if afterCheck.AppliedSSDSVersion != "1.0.0" {
		t.Fatalf("CheckUpgradeCompatibility must be strictly read-only, got version %s", afterCheck.AppliedSSDSVersion)
	}

	// Nâng cấp không có trích dẫn phê duyệt của User -> bị từ chối (§21 CRITICAL).
	if err := ssds.PerformControlledUpgrade(st, root, "2.0.0", ""); err == nil {
		t.Fatalf("expected upgrade without user approval quote to be rejected")
	}
	if err := ssds.PerformControlledUpgrade(st, root, "2.0.0", "Anh Gấu duyệt nâng cấp lên SSDS 2.0.0"); err != nil {
		t.Fatalf("PerformControlledUpgrade failed: %v", err)
	}
	upgraded, _ := st.GetSSDSProject()
	if upgraded.AppliedSSDSVersion != "2.0.0" {
		t.Fatalf("expected version 2.0.0 after upgrade, got %s", upgraded.AppliedSSDSVersion)
	}

	// Hoàn tác tức thời về 1.0.0 từ bản sao lưu .sen/ssds_upgrade_backup.json.
	restoredVer, err := ssds.RollbackSSDSUpgrade(st, root)
	if err != nil || restoredVer != "1.0.0" {
		t.Fatalf("RollbackSSDSUpgrade failed: ver=%q err=%v", restoredVer, err)
	}
	afterRollback, _ := st.GetSSDSProject()
	if afterRollback.AppliedSSDSVersion != "1.0.0" {
		t.Fatalf("expected version restored to 1.0.0, got %s", afterRollback.AppliedSSDSVersion)
	}
}

