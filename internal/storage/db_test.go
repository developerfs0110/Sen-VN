package storage_test

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/developerfs0110/Sen-VN/internal/storage"
)

func TestOpenCreatesExactFiveTablesInWALMode(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), ".sen", "state.db")
	st, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open failed: %v", err)
	}
	defer st.Close()

	tables, err := st.UserTableNames()
	if err != nil {
		t.Fatalf("UserTableNames failed: %v", err)
	}
	if len(tables) != 5 {
		t.Fatalf("expected exact 5 SQLite tables, got %d: %v", len(tables), tables)
	}
	want := map[string]bool{
		"rules":        true,
		"requirements": true,
		"tasks":        true,
		"snapshots":    true,
		"evidence":     true,
	}
	for _, name := range tables {
		if !want[name] {
			t.Errorf("unexpected table %q", name)
		}
	}
}

func TestRulesHardCapTwentyWithReplaceAndDelete(t *testing.T) {
	st, err := storage.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()

	for i := 1; i <= 20; i++ {
		_, err := st.AddRule(fmt.Sprintf("rule %d", i), fmt.Sprintf("quote %d", i), "*")
		if err != nil {
			t.Fatalf("AddRule %d failed unexpectedly: %v", i, err)
		}
	}

	_, err = st.AddRule("rule 21", "quote 21", "*")
	if err == nil {
		t.Fatalf("expected 21st rule to fail hard cap of 20")
	}
	if !strings.Contains(err.Error(), "--replace") || !strings.Contains(err.Error(), "--delete") {
		t.Errorf("expected cap error to suggest --replace and --delete, got: %v", err)
	}

	if err := st.ReplaceRule(5, "updated rule 5", "updated quote 5", "*.go"); err != nil {
		t.Fatalf("ReplaceRule failed: %v", err)
	}

	if err := st.DeleteRule(20); err != nil {
		t.Fatalf("DeleteRule failed: %v", err)
	}

	// Now 19 rules remain; adding 1 more succeeds.
	if _, err := st.AddRule("rule 20b", "quote 20b", "cmd/*"); err != nil {
		t.Fatalf("AddRule after delete failed: %v", err)
	}
	rules, err := st.ListRules()
	if err != nil {
		t.Fatalf("ListRules: %v", err)
	}
	if len(rules) != 20 {
		t.Fatalf("expected 20 rules, got %d", len(rules))
	}
}

func TestRequirementsAndApproval(t *testing.T) {
	st, err := storage.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()

	r1, err := st.AddRequirement("", "Fix login crash", "Null pointer fix in auth", true)
	if err != nil {
		t.Fatalf("AddRequirement R1: %v", err)
	}
	if r1.ID != "R1" || !r1.IsBugfix || r1.Approved {
		t.Fatalf("unexpected R1: %+v", r1)
	}

	rSec, err := st.AddRequirement("R_sec", "Fix login crash", "Security audit on auth path", false)
	if err != nil {
		t.Fatalf("AddRequirement R_sec: %v", err)
	}
	if rSec.ID != "R_sec" {
		t.Fatalf("expected R_sec, got %s", rSec.ID)
	}

	if err := st.ApproveAllRequirements(); err != nil {
		t.Fatalf("ApproveAllRequirements: %v", err)
	}
	reqs, err := st.ListRequirements()
	if err != nil {
		t.Fatalf("ListRequirements: %v", err)
	}
	for _, r := range reqs {
		if !r.Approved {
			t.Errorf("expected requirement %s to be approved", r.ID)
		}
	}
}

func TestEvidenceHashChainDetectsTampering(t *testing.T) {
	st, err := storage.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer st.Close()

	ev1, err := st.AppendEvidence("T1", "go test ./...", "TEST", 4, 0, "ok pkg 0.01s")
	if err != nil {
		t.Fatalf("AppendEvidence 1: %v", err)
	}
	if ev1.PrevHash != "GENESIS" || ev1.EntryHash == "" {
		t.Fatalf("unexpected genesis evidence: %+v", ev1)
	}

	ev2, err := st.AppendEvidence("T2", "go build ./...", "BUILD", 0, 1, "compile error")
	if err != nil {
		t.Fatalf("AppendEvidence 2: %v", err)
	}
	if ev2.PrevHash != ev1.EntryHash {
		t.Fatalf("broken prev_hash link: got %s, want %s", ev2.PrevHash, ev1.EntryHash)
	}

	if err := st.VerifyEvidenceChain(); err != nil {
		t.Fatalf("expected valid chain, got error: %v", err)
	}

	// Simulate SQLite tampering: flip exit_code from 1 to 0 directly via raw SQL.
	if _, err := st.DB().Exec(`UPDATE evidence SET exit_code = 0 WHERE seq = ?`, ev2.Seq); err != nil {
		t.Fatalf("raw tamper update failed: %v", err)
	}

	if err := st.VerifyEvidenceChain(); err == nil {
		t.Fatalf("expected VerifyEvidenceChain to detect tampered exit_code, got nil")
	}
}

func TestEvidenceHashChainDetectsStrengthAndTestsRanTampering(t *testing.T) {
	st, err := storage.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	ev, err := st.AppendEvidence("T1", "go build ./...", "BUILD", 0, 0, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().Exec(`UPDATE evidence SET strength = 'TEST', tests_ran = 5 WHERE seq = ?`, ev.Seq); err != nil {
		t.Fatal(err)
	}
	if err := st.VerifyEvidenceChain(); err == nil {
		t.Fatalf("expected VerifyEvidenceChain to detect tampered strength/tests_ran")
	}
}

func TestImmutableCreateTaskAndPreserveApprovalOnReIntake(t *testing.T) {
	st, err := storage.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	if err := st.CreateTask(storage.Task{
		ID: "T1", ReqIDs: []string{"R1"}, Title: "first", Kind: "FEATURE",
		RiskAuto: "LOW", ScopeFiles: []string{"a.go"}, Status: "VERIFIED",
	}); err != nil {
		t.Fatalf("CreateTask T1 failed: %v", err)
	}
	if err := st.CreateTask(storage.Task{
		ID: "T1", ReqIDs: []string{"R2"}, Title: "reuse", Kind: "FEATURE",
		RiskAuto: "LOW", ScopeFiles: []string{"b.go"}, Status: "OPEN",
	}); err == nil {
		t.Fatalf("expected duplicate task ID T1 to be rejected by CreateTask")
	}

	if _, err := st.AddRequirement("R_sec", "Original user security quote", "Security check", false); err != nil {
		t.Fatal(err)
	}
	if err := st.ApproveAllRequirements(); err != nil {
		t.Fatal(err)
	}
	// Re-anchoring R_sec with fallback quote must preserve Approved=true and original quote.
	if _, err := st.AddRequirement("R_sec", "", "Security check updated", false); err != nil {
		t.Fatal(err)
	}
	reqs, err := st.ListRequirements()
	if err != nil || len(reqs) != 1 {
		t.Fatalf("expected 1 req, got %d (err=%v)", len(reqs), err)
	}
	if !reqs[0].Approved || reqs[0].UserQuote != "Original user security quote" {
		t.Fatalf("expected Approved=true and original quote preserved, got %+v", reqs[0])
	}
}

// TestCountConsecutiveFailedEvidence kiểm chứng đếm số lần verify lỗi liên tiếp từ lần pass gần nhất mà không tạo bảng mới.
func TestCountConsecutiveFailedEvidence(t *testing.T) {
	st, err := storage.Open(filepath.Join(t.TempDir(), "state.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	_, _ = st.AppendEvidence("T1", "go test", "TEST", 0, 1, "fail 1")
	_, _ = st.AppendEvidence("T1", "go test", "TEST", 2, 0, "pass resets streak")
	ev3, _ := st.AppendEvidence("T1", "go test", "TEST", 0, 1, "fail A")
	_, _ = st.AppendEvidence("T-OTHER", "go test", "TEST", 0, 1, "other task fail")
	_, _ = st.AppendEvidence("T1", "go test", "TEST", 0, 2, "fail B")

	got, err := st.CountConsecutiveFailedEvidence("T1")
	if err != nil || got != 2 {
		t.Fatalf("expected 2 consecutive failed evidence rows for T1, got %d (err=%v)", got, err)
	}

	gotAfter, err := st.CountConsecutiveFailedEvidenceSince("T1", ev3.Seq)
	if err != nil || gotAfter != 1 {
		t.Fatalf("expected 1 failed evidence row after seq %d, got %d (err=%v)", ev3.Seq, gotAfter, err)
	}
}

