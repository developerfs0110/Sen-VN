// Tệp state_machine.go thực thi máy trạng thái pha P0..P9, tường lửa khóa ghi tệp theo pha và cổng phê duyệt của User (§22).
package ssds

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/developerfs0110/Sen-VN/internal/storage"
	"github.com/developerfs0110/Sen-VN/internal/workspace"
)

// CheckWriteFirewall cưỡng chế quyền ghi tệp theo đúng pha SDLC hiện hành và khóa băm tài liệu baseline đã duyệt (§5–§10, §17).
func CheckWriteFirewall(st *storage.Store, projectRoot, relPath string) error {
	rel, err := workspace.NormalizeRelPath(projectRoot, relPath)
	if err != nil {
		return err
	}
	if err := workspace.CheckNoSubprojectCrossing(projectRoot, rel); err != nil {
		return err
	}
	lower := strings.ToLower(rel)
	if lower == ".sen" || strings.HasPrefix(lower, ".sen/") {
		return fmt.Errorf("cannot write to protected .sen ledger path %q", rel)
	}

	proj, err := st.GetSSDSProject()
	if err != nil {
		return err
	}
	if proj == nil {
		if workspace.IsSoftwareSourceOrManifestPath(rel) {
			return fmt.Errorf("SSDS §3.1 firewall: software source/manifest write %q denied before SSDS activation", rel)
		}
		return nil
	}
	if lower == ".agents" || strings.HasPrefix(lower, ".agents/") {
		return fmt.Errorf("SSDS isolation firewall: project state or artifacts cannot be written inside shared .agents/ (%q)", rel)
	}

	if err := checkBaselineCIALock(st, rel, lower); err != nil {
		return err
	}

	isCode := workspace.IsSoftwareSourceOrManifestPath(rel)
	switch proj.CurrentPhase {
	case "P0":
		if isCode {
			return fmt.Errorf("SSDS §5 P0 firewall: production source/manifest %q forbidden during Project Intake", rel)
		}
		if lower != "docs/00-project-charter.md" && lower != "project_status.md" {
			return fmt.Errorf("SSDS §5 P0 firewall: only docs/00-project-charter.md and PROJECT_STATUS.md are writable in P0 (got %q)", rel)
		}
	case "P1":
		if isCode {
			return fmt.Errorf("SSDS §6 P1 firewall: implementation code/manifest %q is strictly forbidden during Requirements Analysis", rel)
		}
		if lower != "docs/00-project-charter.md" && lower != "docs/01-requirements.md" && lower != "project_status.md" {
			return fmt.Errorf("SSDS §6 P1 firewall: path %q is outside allowed P1 deliverables (docs/01-requirements.md)", rel)
		}
	case "P2":
		if proj.Category == "PROTOTYPE" && (strings.HasPrefix(lower, "prototype/") || strings.HasPrefix(lower, "spike/")) {
			return nil
		}
		if isCode {
			return fmt.Errorf("SSDS §7 P2 firewall: production functionality %q is strictly forbidden during Architecture Design", rel)
		}
		if !isAllowedP2Path(lower) {
			return fmt.Errorf("SSDS §7 P2 firewall: path %q is outside allowed P2 architecture/ADR/diagram deliverables", rel)
		}
	case "P3":
		if isCode {
			return fmt.Errorf("SSDS §8 P3 firewall: production code/manifest %q is forbidden during Project Planning", rel)
		}
		if !isAllowedP3Path(lower, proj.WorkflowTier) {
			return fmt.Errorf("SSDS §8 P3 firewall: path %q is outside allowed P3 planning/traceability deliverables", rel)
		}
	case "P6":
		if isCode && !workspace.IsTestFile(rel) && !strings.HasPrefix(lower, "test/") && !strings.HasPrefix(lower, "tests/") && !strings.HasPrefix(lower, "e2e/") {
			return fmt.Errorf("SSDS §11 P6 firewall: production source %q cannot be modified directly in QA phase (re-enter P5 for fixes)", rel)
		}
	case "P8":
		if isCode {
			return fmt.Errorf("SSDS §13 P8 firewall: production source/manifest %q forbidden during Documentation & Handover", rel)
		}
	}
	return nil
}

// isAllowedP2Path kiểm tra đường dẫn thuộc tập tài liệu kiến trúc, ADR, sơ đồ và đặc tả dữ liệu/API/bảo mật của Pha 2.
func isAllowedP2Path(lower string) bool {
	return lower == "docs/02-architecture.md" ||
		strings.HasPrefix(lower, "docs/adr/") ||
		strings.HasPrefix(lower, "docs/diagrams/") ||
		strings.HasPrefix(lower, "docs/03-") ||
		strings.HasPrefix(lower, "docs/04-") ||
		strings.HasPrefix(lower, "docs/05-") ||
		lower == "docs/decisions.md" ||
		lower == "project_status.md"
}

// isAllowedP3Path kiểm tra đường dẫn thuộc tập tài liệu kế hoạch, đặc tả kỹ thuật, chia task và ma trận truy vết của Pha 3.
func isAllowedP3Path(lower, workflowTier string) bool {
	if lower == "roadmap.md" || lower == "tasks.md" || lower == "project_status.md" ||
		lower == "docs/traceability-matrix.md" || lower == "docs/decisions.md" {
		return true
	}
	for _, p := range []string{"docs/06-", "docs/07-", "docs/08-", "docs/09-", "docs/10-"} {
		if strings.HasPrefix(lower, p) {
			return true
		}
	}
	if strings.EqualFold(workflowTier, "WF_MICRO") && (lower == "docs/02-architecture.md" || strings.HasPrefix(lower, "docs/adr/")) {
		return true
	}
	return false
}

// CanBeginImplementationTask xác minh toàn bộ các pha tiền đề (P1, P2, P3, P4) đã hoàn tất và không có ngoại lệ quá hạn trước khi code.
func CanBeginImplementationTask(st *storage.Store, projectRoot string) error {
	proj, err := st.GetSSDSProject()
	if err != nil {
		return err
	}
	if proj == nil {
		return errors.New("SSDS project is not initialized: run classification before beginning implementation tasks")
	}
	if err := CheckActiveExceptions(st); err != nil {
		return err
	}

	activeSet := make(map[string]bool, len(proj.ActivePhases))
	for _, pid := range proj.ActivePhases {
		activeSet[pid] = true
	}
	for _, pid := range []string{"P0", "P1", "P2", "P3", "P4"} {
		if !activeSet[pid] {
			continue
		}
		ph, err := st.GetSSDSPhase(pid)
		if err != nil {
			return fmt.Errorf("SSDS prerequisite phase %s missing: %w", pid, err)
		}
		if pid == "P3" || pid == "P4" {
			if ph.Status != "COMPLETED" {
				return fmt.Errorf("SSDS prerequisite phase %s must be COMPLETED before implementation (current=%s)", pid, ph.Status)
			}
		} else if ph.Status != "APPROVED" && ph.Status != "COMPLETED" {
			return fmt.Errorf("SSDS prerequisite phase %s must be APPROVED/COMPLETED before implementation (current=%s)", pid, ph.Status)
		}
	}

	if strings.TrimSpace(projectRoot) != "" {
		if drifts := VerifyApprovedBaselineIntegrity(st, projectRoot); len(drifts) > 0 {
			return fmt.Errorf("SSDS §17 baseline drift blocks implementation: %s", strings.Join(drifts, "; "))
		}
	}
	return nil
}

// AdvancePhaseAfterGate chuyển trạng thái pha sang REVIEW_REQUIRED (nếu cần User duyệt) hoặc COMPLETED sau khi vượt qua cổng chất lượng.
func AdvancePhaseAfterGate(st *storage.Store, projectRoot, phaseID string, gate *GateResult) (*storage.SSDSPhase, error) {
	if gate == nil || !gate.Passed {
		return nil, fmt.Errorf("SSDS §22 violation: failed quality gate on phase %s cannot advance phase state", phaseID)
	}
	proj, err := st.GetSSDSProject()
	if err != nil || proj == nil {
		return nil, errors.New("SSDS project is not initialized")
	}
	ph, err := st.GetSSDSPhase(phaseID)
	if err != nil {
		return nil, err
	}
	ph.GatePassed = true
	newHashes := ph.VerifiedDeliverables
	if len(gate.DeliverableHashes) > 0 {
		newHashes = gate.DeliverableHashes
		if strings.TrimSpace(projectRoot) != "" {
			newHashes = collectPhaseBaselineHashes(projectRoot, phaseID, gate.DeliverableHashes)
		}
	}
	hashesChanged := deliverableHashesChanged(ph.VerifiedDeliverables, newHashes)
	ph.VerifiedDeliverables = newHashes

	if ph.RequiresUserApproval {
		if (ph.Status != "APPROVED" && ph.Status != "COMPLETED") || hashesChanged {
			ph.Status = "REVIEW_REQUIRED"
		}
		if err := st.UpsertSSDSPhase(*ph); err != nil {
			return nil, err
		}
		advanceBaselineForDeliverables(st, projectRoot, ph.VerifiedDeliverables)
		return st.GetSSDSPhase(phaseID)
	}

	ph.Status = "COMPLETED"
	if err := st.UpsertSSDSPhase(*ph); err != nil {
		return nil, err
	}
	advanceBaselineForDeliverables(st, projectRoot, ph.VerifiedDeliverables)
	if err := activateNextWorkflowPhase(st, proj, phaseID); err != nil {
		return nil, err
	}
	return st.GetSSDSPhase(phaseID)
}

// ApprovePhase thực thi phê duyệt tường minh của User (REVIEW_REQUIRED -> APPROVED -> COMPLETED) và kích hoạt pha kế tiếp.
func ApprovePhase(st *storage.Store, projectRoot, phaseID, approver, userQuote string) error {
	quote := strings.TrimSpace(userQuote)
	if quote == "" {
		return errors.New("SSDS §21/§22 violation: silence is never approval — non-empty userQuote is required")
	}
	proj, err := st.GetSSDSProject()
	if err != nil || proj == nil {
		return errors.New("SSDS project is not initialized")
	}
	ph, err := st.GetSSDSPhase(phaseID)
	if err != nil {
		return err
	}
	if !ph.GatePassed || ph.Status != "REVIEW_REQUIRED" {
		return fmt.Errorf("SSDS §22 violation: phase %s cannot be approved in status %s (gate_passed=%v; must reach REVIEW_REQUIRED first)",
			phaseID, ph.Status, ph.GatePassed)
	}

	if strings.TrimSpace(projectRoot) != "" {
		ph.VerifiedDeliverables = collectPhaseBaselineHashes(projectRoot, phaseID, ph.VerifiedDeliverables)
	}
	if strings.EqualFold(phaseID, "P1") {
		if err := st.ApproveAllRequirements(); err != nil {
			return err
		}
	}

	who := strings.TrimSpace(approver)
	if who == "" {
		who = "User"
	}
	ph.ApprovedBy = who
	ph.ApprovedQuote = quote
	ph.ApprovedAt = time.Now().UTC().Format(time.RFC3339)
	ph.Status = "APPROVED"
	if err := st.UpsertSSDSPhase(*ph); err != nil {
		return err
	}

	payloadBytes, _ := json.Marshal(map[string]string{
		"phase_id":   phaseID,
		"approver":   who,
		"user_quote": quote,
	})
	if _, err := st.AppendGovernanceRecord(storage.GovernanceRecord{
		RecordKind: "DECISION", PhaseID: phaseID, AuthorityLevel: "CRITICAL",
		AuthorRole: ph.PrimaryRole, ReviewerRole: who, EvidenceMode: "HUMAN_APPROVAL",
		Verdict: "PASS", PayloadJSON: string(payloadBytes), Approved: true,
	}); err != nil {
		return err
	}

	ph.Status = "COMPLETED"
	if err := st.UpsertSSDSPhase(*ph); err != nil {
		return err
	}
	advanceBaselineForDeliverables(st, projectRoot, ph.VerifiedDeliverables)
	return activateNextWorkflowPhase(st, proj, phaseID)
}

// advanceBaselineForDeliverables cập nhật ảnh chụp __BASELINE__ cho các tài liệu bàn giao phi mã nguồn đã qua cổng kiểm định.
func advanceBaselineForDeliverables(st *storage.Store, projectRoot string, verified map[string]string) {
	if strings.TrimSpace(projectRoot) == "" {
		return
	}
	baseSnaps, err := st.GetSnapshots(workspace.BaselineTaskID)
	if err != nil || len(baseSnaps) == 0 {
		return
	}
	tasks, _ := st.ListTasks()
	for _, t := range tasks {
		if t.Status == "OPEN" {
			return
		}
	}
	seen := make(map[string]bool)
	var paths []string
	addCandidate := func(rel string) {
		norm, nErr := workspace.NormalizeRelPath(projectRoot, rel)
		if nErr != nil || seen[norm] || workspace.IsSoftwareSourceOrManifestPath(norm) || workspace.IsLockedAsset(norm) {
			return
		}
		if stInfo, sErr := os.Stat(filepath.Join(projectRoot, filepath.FromSlash(norm))); sErr == nil && stInfo.Mode().IsRegular() {
			seen[norm] = true
			paths = append(paths, norm)
		}
	}
	for rel := range verified {
		addCandidate(rel)
	}
	for _, extra := range []string{"docs/traceability-matrix.md", "docs/testing/bug-reports.md", "README.md", "CHANGELOG.md"} {
		addCandidate(extra)
	}
	if len(paths) > 0 {
		_ = workspace.AdvanceBaseline(st, projectRoot, paths)
	}
}

// activateNextWorkflowPhase mở khóa pha kế tiếp sang IN_PROGRESS (chỉ đổi CurrentPhase khi pha vừa xong đang là CurrentPhase).
func activateNextWorkflowPhase(st *storage.Store, proj *storage.SSDSProject, completedPhaseID string) error {
	for i, pid := range proj.ActivePhases {
		if strings.EqualFold(pid, completedPhaseID) && i+1 < len(proj.ActivePhases) {
			nextID := proj.ActivePhases[i+1]
			nextPh, err := st.GetSSDSPhase(nextID)
			if err != nil {
				return err
			}
			if nextPh.Status == "NOT_STARTED" {
				nextPh.Status = "IN_PROGRESS"
				if err := st.UpsertSSDSPhase(*nextPh); err != nil {
					return err
				}
			}
			if strings.EqualFold(proj.CurrentPhase, completedPhaseID) {
				return st.UpdateCurrentPhase(nextID)
			}
			return nil
		}
	}
	return nil
}

// deliverableHashesChanged kiểm tra tập mã băm tài liệu chuẩn của pha đã phê duyệt trước đó có thay đổi hay không.
func deliverableHashesChanged(prev, cur map[string]string) bool {
	if len(prev) == 0 {
		return false
	}
	if len(prev) != len(cur) {
		return true
	}
	for k, v := range prev {
		if cur[k] != v {
			return true
		}
	}
	return false
}

// collectPhaseBaselineHashes chụp lại mã băm SHA-256 của toàn bộ tài liệu chuẩn đã duyệt trong P1 hoặc P2 (kể cả ADR-*.md).
func collectPhaseBaselineHashes(projectRoot, phaseID string, existing map[string]string) map[string]string {
	out := make(map[string]string)
	for k, v := range existing {
		out[k] = v
	}
	var candidates []string
	if strings.EqualFold(phaseID, "P1") {
		candidates = []string{"docs/01-requirements.md"}
	} else if strings.EqualFold(phaseID, "P2") {
		candidates = []string{
			"docs/02-architecture.md", "docs/03-database-design.md",
			"docs/04-api-specification.md", "docs/05-security-design.md",
		}
		adrDir := filepath.Join(projectRoot, "docs", "adr")
		if entries, err := os.ReadDir(adrDir); err == nil {
			for _, e := range entries {
				if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
					candidates = append(candidates, "docs/adr/"+e.Name())
				}
			}
		}
	}
	for _, rel := range candidates {
		if sum, err := hashDeliverableFile(filepath.Join(projectRoot, filepath.FromSlash(rel))); err == nil {
			out[rel] = sum
		}
	}
	return out
}
