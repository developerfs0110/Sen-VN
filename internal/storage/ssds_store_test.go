// Tệp ssds_store_test.go kiểm thử nâng cấp schema tương thích ngược, cô lập project_id, vòng đời pha và chuỗi băm ssds_governance.
package storage_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/developerfs0110/Sen-VN/internal/storage"
)

// TestLegacyV11DatabaseMigratesIdempotentlyWithoutDataLoss xác minh cơ sở dữ liệu 5 bảng v1.1 được nâng cấp an toàn lên 8 bảng v1.2.
func TestLegacyV11DatabaseMigratesIdempotentlyWithoutDataLoss(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), ".sen", "state.db")
	stInit, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	_ = stInit.Close()

	// Tạo một tệp DB thứ hai chỉ chứa đúng schema 5 bảng cũ của v1.1 (chưa có cột SSDS).
	legacyPath := filepath.Join(t.TempDir(), "legacy_v11.db")
	rawDB, err := sql.Open("sqlite", legacyPath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = rawDB.Exec(`
		CREATE TABLE requirements (
			id TEXT PRIMARY KEY, user_quote TEXT NOT NULL, summary TEXT NOT NULL,
			is_bugfix INTEGER NOT NULL DEFAULT 0, approved INTEGER NOT NULL DEFAULT 0, created_at TEXT NOT NULL
		);
		CREATE TABLE tasks (
			id TEXT PRIMARY KEY, req_ids_json TEXT NOT NULL, title TEXT NOT NULL, kind TEXT NOT NULL,
			risk_auto TEXT NOT NULL, scope_files_json TEXT NOT NULL, status TEXT NOT NULL,
			created_at TEXT NOT NULL, updated_at TEXT NOT NULL
		);
		INSERT INTO requirements (id, user_quote, summary, is_bugfix, approved, created_at)
		VALUES ('R1', 'quote v1.1', 'legacy requirement', 0, 1, '2026-01-01T00:00:00Z');
		INSERT INTO tasks (id, req_ids_json, title, kind, risk_auto, scope_files_json, status, created_at, updated_at)
		VALUES ('T-OLD', '["R1"]', 'legacy task', 'FEATURE', 'SMALL', '["a.go"]', 'VERIFIED', '2026-01-01T00:00:00Z', '2026-01-01T00:00:00Z');
	`)
	_ = rawDB.Close()
	if err != nil {
		t.Fatalf("seed legacy v1.1 db failed: %v", err)
	}

	// Mở lại bằng storage.Open v1.2 hai lần liên tiếp để kiểm chứng tính idempotent.
	st1, err := storage.Open(legacyPath)
	if err != nil {
		t.Fatalf("first Open on legacy DB failed: %v", err)
	}
	_ = st1.Close()

	st, err := storage.Open(legacyPath)
	if err != nil {
		t.Fatalf("second Open on migrated DB failed: %v", err)
	}
	defer st.Close()

	reqs, err := st.ListRequirements()
	if err != nil || len(reqs) != 1 || reqs[0].ReqType != "FUNC" || !reqs[0].Approved {
		t.Fatalf("unexpected migrated requirements: %+v (err=%v)", reqs, err)
	}
	tk, err := st.GetTask("T-OLD")
	if err != nil || tk.PhaseID != "P5" || tk.ExecState != "IMPLEMENTED" || tk.Verdict != "NOT_EXECUTED" {
		t.Fatalf("unexpected migrated task defaults: %+v (err=%v)", tk, err)
	}
}

// TestTypedRequirementsCoexistWithSequentialRAndTaskExecutionStates kiểm chứng REQ-F-001 chung sống với R1..Rn và cập nhật 4 trạng thái §20.
func TestTypedRequirementsCoexistWithSequentialRAndTaskExecutionStates(t *testing.T) {
	st, err := storage.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	rf, err := st.UpsertTypedRequirement("REQ-F-001", "", "User wants invoice export", "Export PDF invoice", "Given invoice When export Then PDF saved", "abc123sha", false)
	if err != nil {
		t.Fatalf("UpsertTypedRequirement REQ-F-001 failed: %v", err)
	}
	if rf.ReqType != "FUNC" || rf.DocSHA256 != "abc123sha" {
		t.Fatalf("unexpected REQ-F-001: %+v", rf)
	}
	rnf, err := st.UpsertTypedRequirement("REQ-NF-001", "", "Fast response", "Latency < 200ms", "Given 100 RPS When query Then p95 < 200ms", "abc123sha", false)
	if err != nil || rnf.ReqType != "NON_FUNC" {
		t.Fatalf("expected NON_FUNC inferred for REQ-NF-001, got %+v (err=%v)", rnf, err)
	}
	r1, err := st.AddRequirement("", "Quick intake", "First sequential requirement", false)
	if err != nil || r1.ID != "R1" {
		t.Fatalf("expected R1 to coexist cleanly alongside REQ-F-001, got %+v (err=%v)", r1, err)
	}
	r2, err := st.AddRequirement("", "Second intake", "Second sequential requirement", false)
	if err != nil || r2.ID != "R2" {
		t.Fatalf("expected R2 sequential increment, got %+v (err=%v)", r2, err)
	}

	if err := st.CreateTask(storage.Task{
		ID: "TASK-001", PhaseID: "P5", ReqIDs: []string{"REQ-F-001"}, ArchComponents: []string{"COMP-BILLING"},
		Title: "Implement PDF exporter", Kind: "FEATURE", RiskAuto: "MEDIUM", ScopeFiles: []string{"pdf.go"}, Status: "OPEN",
	}); err != nil {
		t.Fatalf("CreateTask TASK-001: %v", err)
	}
	if err := st.UpdateTaskExecutionState("TASK-001", "TESTED", "NOT VERIFIED", ""); err != nil {
		t.Fatalf("UpdateTaskExecutionState TESTED: %v", err)
	}
	got, err := st.GetTask("TASK-001")
	if err != nil || got.ExecState != "TESTED" || got.Verdict != "NOT_VERIFIED" || got.Status != "OPEN" {
		t.Fatalf("unexpected task state after TESTED: %+v (err=%v)", got, err)
	}
	if err := st.UpdateTaskExecutionState("TASK-001", "VERIFIED", "PASS", "VERIFIED"); err != nil {
		t.Fatalf("UpdateTaskExecutionState VERIFIED: %v", err)
	}
	got2, _ := st.GetTask("TASK-001")
	if got2.ExecState != "VERIFIED" || got2.Verdict != "PASS" || got2.Status != "VERIFIED" {
		t.Fatalf("unexpected task state after VERIFIED: %+v", got2)
	}
}

// TestSSDSProjectSingleIDLockPhasesAndGovernanceHashChain kiểm chứng khóa chống nhiễm chéo project_id, pha P0..P9 và chuỗi băm governance.
func TestSSDSProjectSingleIDLockPhasesAndGovernanceHashChain(t *testing.T) {
	st, err := storage.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	// Khi bảng trống, GetSSDSProject bắt buộc trả về (nil, nil).
	emptyProj, err := st.GetSSDSProject()
	if err != nil || emptyProj != nil {
		t.Fatalf("expected (nil, nil) on empty ssds_project, got (%+v, %v)", emptyProj, err)
	}

	projA := storage.SSDSProject{
		ProjectID:          "11111111-2222-4333-8444-555555555555",
		ProjectName:        "ERP-Core",
		AppliedSSDSVersion: "1.0.0",
		Category:           "GREENFIELD",
		WorkflowTier:       "WF_STANDARD",
		ActivePhases:       []string{"P0", "P1", "P2"},
		RiskAssessment:     map[string]string{"complexity": "MEDIUM"},
		CurrentPhase:       "P0",
	}
	if err := st.SaveSSDSProject(projA); err != nil {
		t.Fatalf("SaveSSDSProject A failed: %v", err)
	}

	// Cố tình ghi đè bằng project_id khác phải bị từ chối cứng!
	projB := projA
	projB.ProjectID = "99999999-8888-4777-8666-555555555555"
	if err := st.SaveSSDSProject(projB); err == nil || !strings.Contains(err.Error(), "cross-project contamination") {
		t.Fatalf("expected cross-project project_id overwrite to fail, got: %v", err)
	}

	if err := st.UpdateCurrentPhase("P1"); err != nil {
		t.Fatal(err)
	}
	if err := st.SetBrownfieldAudited(true); err != nil {
		t.Fatal(err)
	}
	if err := st.UpdateAppliedSSDSVersion("1.1.0"); err != nil {
		t.Fatal(err)
	}
	loaded, err := st.GetSSDSProject()
	if err != nil || loaded.CurrentPhase != "P1" || !loaded.BrownfieldAudited || loaded.AppliedSSDSVersion != "1.1.0" {
		t.Fatalf("unexpected updated SSDSProject: %+v (err=%v)", loaded, err)
	}

	if err := st.UpsertSSDSPhase(storage.SSDSPhase{
		PhaseID: "P1", Status: "REVIEW_REQUIRED", PrimaryRole: "Senior Business Analyst",
		SupportingRoles: []string{"System Analyst"}, RequiredDeliverables: []string{"docs/01-requirements.md"},
		VerifiedDeliverables: map[string]string{"docs/01-requirements.md": "sha256hex"},
		RequiresUserApproval: true, GatePassed: true,
	}); err != nil {
		t.Fatalf("UpsertSSDSPhase P1: %v", err)
	}
	ph1, err := st.GetSSDSPhase("P1")
	if err != nil || !ph1.RequiresUserApproval || !ph1.GatePassed || ph1.VerifiedDeliverables["docs/01-requirements.md"] != "sha256hex" {
		t.Fatalf("unexpected P1 row: %+v (err=%v)", ph1, err)
	}

	// Kiểm tra chuỗi băm ssds_governance từ GENESIS -> phê duyệt hợp lệ -> phát hiện sửa trộm SQL.
	cia, err := st.AppendGovernanceRecord(storage.GovernanceRecord{
		RecordKind: "CHANGE_IMPACT", PhaseID: "P2", AuthorityLevel: "CRITICAL",
		AuthorRole: "Principal Software Architect", EvidenceMode: "SELF_REVIEW", Verdict: "PASS",
		PayloadJSON: `{"reason":"Add multi-currency column"}`, Approved: false,
	})
	if err != nil || cia.RecordID != "CIA-001" || cia.PrevHash != "GENESIS" {
		t.Fatalf("unexpected CIA-001: %+v (err=%v)", cia, err)
	}
	rev, err := st.AppendGovernanceRecord(storage.GovernanceRecord{
		RecordKind: "REVIEW", PhaseID: "P2", AuthorityLevel: "MAJOR",
		AuthorRole: "Principal Software Architect", ReviewerRole: "Security Architect",
		EvidenceMode: "INDEPENDENT_AI_REVIEW", Verdict: "PASS", Approved: true,
	})
	if err != nil || rev.RecordID != "REV-001" || rev.PrevHash != cia.EntryHash {
		t.Fatalf("unexpected REV-001 chain link: %+v (err=%v)", rev, err)
	}

	// Từ chối phê duyệt rỗng (im lặng không phải là đồng ý).
	if err := st.ApproveGovernanceRecord("CIA-001", "   "); err == nil {
		t.Fatalf("expected empty approverQuote to fail")
	}
	if err := st.ApproveGovernanceRecord("CIA-001", "OK anh duyệt CIA-001"); err != nil {
		t.Fatalf("ApproveGovernanceRecord CIA-001 failed: %v", err)
	}
	if err := st.VerifyGovernanceChain(); err != nil {
		t.Fatalf("expected valid governance chain after authorized approval, got: %v", err)
	}
	ciaList, _ := st.ListGovernanceRecords("CHANGE_IMPACT", "P2")
	if len(ciaList) != 1 || !ciaList[0].Approved || !strings.Contains(ciaList[0].PayloadJSON, "OK anh duyệt CIA-001") {
		t.Fatalf("expected CIA-001 approved with quote in payload, got %+v", ciaList)
	}

	// Sửa trộm cột verdict bằng raw SQL phải bị VerifyGovernanceChain bắt tại trận!
	if _, err := st.DB().Exec(`UPDATE ssds_governance SET verdict = 'FAIL' WHERE record_id = 'REV-001'`); err != nil {
		t.Fatal(err)
	}
	if err := st.VerifyGovernanceChain(); err == nil {
		t.Fatalf("expected VerifyGovernanceChain to catch tampered verdict on REV-001")
	}
}

