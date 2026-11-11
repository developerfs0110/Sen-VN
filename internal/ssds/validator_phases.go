// Tệp validator_phases.go thực thi các quy tắc kiểm định chuyên biệt cho các pha P3, P4, P5, P6 và P7 của SSDS.
package ssds

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/developerfs0110/Sen-VN/internal/storage"
)

// validatePhase3Planning kiểm tra đủ các trường task của SSDS §8 và sự hiện diện của khung docs/traceability-matrix.md.
func validatePhase3Planning(st *storage.Store, proj *storage.SSDSProject, root string, res *GateResult) {
	taskDoc := filepath.Join(root, "docs", "08-task-breakdown.md")
	relDoc := "docs/08-task-breakdown.md"
	if proj.WorkflowTier == "WF_MICRO" {
		if _, err := os.Stat(taskDoc); os.IsNotExist(err) {
			taskDoc = filepath.Join(root, "TASKS.md")
			relDoc = "TASKS.md"
		}
	}
	raw, err := os.ReadFile(taskDoc)
	if err != nil {
		res.Violations = append(res.Violations, fmt.Sprintf("P3 missing task breakdown deliverable (%s)", relDoc))
		return
	}
	text := string(raw)
	if !reTaskIDToken.MatchString(text) {
		res.Violations = append(res.Violations, fmt.Sprintf("SSDS §8 violation: %s must define explicit Task IDs (e.g., TASK-001)", relDoc))
	}
	if !reReqToken.MatchString(text) {
		res.Violations = append(res.Violations, fmt.Sprintf("SSDS §8 violation: tasks in %s must reference REQ-* IDs", relDoc))
	}
	if proj.WorkflowTier != "WF_MICRO" {
		lower := strings.ToLower(text)
		for _, field := range []string{"component", "dependenc", "output", "acceptance", "test", "complexity", "status"} {
			if !strings.Contains(lower, field) {
				res.Violations = append(res.Violations, fmt.Sprintf("SSDS §8 violation: %s missing mandatory task attribute %q", relDoc, field))
			}
		}
	}
	matBytes, mErr := os.ReadFile(filepath.Join(root, "docs", "traceability-matrix.md"))
	if mErr == nil {
		matUpper := strings.ToUpper(string(matBytes))
		reqs, _ := st.ListRequirements()
		for _, r := range reqs {
			u := strings.ToUpper(r.ID)
			if (strings.HasPrefix(u, "REQ-F-") || strings.HasPrefix(u, "REQ-NF-")) && !strings.Contains(matUpper, u) {
				res.Violations = append(res.Violations, fmt.Sprintf("SSDS §16 violation: docs/traceability-matrix.md omits requirement %s", r.ID))
			}
		}
	}
}

// validatePhase4Foundation xác minh dự án đã chạy thành công ít nhất một lệnh build/test nền trong bảng evidence (§9).
func validatePhase4Foundation(st *storage.Store, res *GateResult) {
	evs, err := st.ListEvidence()
	if err != nil {
		res.Violations = append(res.Violations, err.Error())
		return
	}
	for _, ev := range evs {
		if ev.ExitCode == 0 {
			return
		}
	}
	res.Violations = append(res.Violations, "SSDS §9 violation: P4 requires passing build/test foundation execution evidence (exit_code == 0)")
}

// validatePhase5Implementation kiểm tra toàn bộ task đã thỏa mãn Definition of Done (§19) và có bằng chứng thực thi PASS (§20).
func validatePhase5Implementation(st *storage.Store, res *GateResult) {
	tasks, err := st.ListTasks()
	if err != nil {
		res.Violations = append(res.Violations, err.Error())
		return
	}
	evs, _ := st.ListEvidence()
	passByTask := make(map[string]bool)
	for _, ev := range evs {
		if ev.ExitCode == 0 {
			passByTask[ev.TaskID] = true
		}
	}
	activeCount := 0
	for _, t := range tasks {
		if t.Status == "UNDONE" {
			continue
		}
		if t.PhaseID != "" && !strings.EqualFold(t.PhaseID, "P5") {
			continue
		}
		activeCount++
		if t.ExecState != "VERIFIED" || t.Verdict != "PASS" || !passByTask[t.ID] {
			res.Violations = append(res.Violations, fmt.Sprintf(
				"SSDS §19/§20 DoD violation: task %s has exec_state=%s verdict=%s has_passing_evidence=%v",
				t.ID, t.ExecState, t.Verdict, passByTask[t.ID],
			))
		}
	}
	if activeCount == 0 {
		res.Violations = append(res.Violations, "SSDS §10 violation: P5 cannot pass with 0 completed implementation tasks")
	}
}

// validatePhase6QAAndSecurity chặn tuyệt đối báo cáo láo PASS (§11), kiểm tra lỗi P0/P1 tồn đọng và đối soát truy vết 6 mắt xích.
func validatePhase6QAAndSecurity(st *storage.Store, root string, res *GateResult) {
	if err := st.VerifyEvidenceChain(); err != nil {
		res.Violations = append(res.Violations, fmt.Sprintf("SSDS §20 evidence chain tampered: %v", err))
	}
	evs, _ := st.ListEvidence()
	passingTestRuns := 0
	for _, ev := range evs {
		if ev.ExitCode == 0 && strings.EqualFold(ev.Strength, "TEST") && ev.TestsRan > 0 {
			passingTestRuns++
		}
	}

	resPath := filepath.Join(root, "docs", "testing", "test-results.md")
	if rawRes, err := os.ReadFile(resPath); err == nil {
		if reClaimedPass.Match(rawRes) && passingTestRuns == 0 {
			res.Violations = append(res.Violations, "SSDS §11 violation: unexecuted tests reported as PASS (0 passing TEST runs in evidence ledger)")
		}
	}

	bugPath := filepath.Join(root, "docs", "testing", "bug-reports.md")
	if rawBugs, err := os.ReadFile(bugPath); err == nil {
		for _, line := range strings.Split(string(rawBugs), "\n") {
			if reOpenSeverity.MatchString(line) {
				upper := strings.ToUpper(line)
				if strings.Contains(upper, "P0") || !strings.Contains(upper, "ACCEPTED") {
					res.Violations = append(res.Violations, fmt.Sprintf("SSDS §11 defect gate: unresolved P0/unaccepted P1 defect in bug-reports.md (%s)", strings.TrimSpace(line)))
				}
			}
		}
	}

	_, traceViolations := ValidateTraceabilityMatrix(st, root, false)
	res.Violations = append(res.Violations, traceViolations...)
	res.Violations = append(res.Violations, VerifyApprovedBaselineIntegrity(st, root)...)
}

// validatePhase7Release đối soát ma trận truy vết gắn nhãn Release cụ thể và bằng chứng đóng gói/triển khai thành công.
func validatePhase7Release(st *storage.Store, root string, res *GateResult) {
	_, traceViolations := ValidateTraceabilityMatrix(st, root, true)
	res.Violations = append(res.Violations, traceViolations...)
	validatePhase4Foundation(st, res)
}

