// Tệp renderer.go đồng bộ trạng thái từ SQLite sang các tệp chuẩn trên repo (§24) và xuất Báo cáo Tiến độ 8 phần (§25).
package ssds

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/developerfs0110/Sen-VN/internal/storage"
	"github.com/developerfs0110/Sen-VN/internal/workspace"
)

// SyncProjectStateFiles kết xuất xác định PROJECT_STATUS.md, ROADMAP.md (nếu thiếu), TASKS.md và docs/DECISIONS.md từ SQLite (§24).
func SyncProjectStateFiles(st *storage.Store, projectRoot string) error {
	if strings.TrimSpace(projectRoot) == "" {
		return fmt.Errorf("projectRoot cannot be empty")
	}
	proj, err := st.GetSSDSProject()
	if err != nil {
		return err
	}
	if proj == nil {
		return fmt.Errorf("SSDS project is not initialized")
	}
	phases, err := st.ListSSDSPhases()
	if err != nil {
		return err
	}
	tasks, err := st.ListTasks()
	if err != nil {
		return err
	}
	evs, err := st.ListEvidence()
	if err != nil {
		return err
	}
	govs, err := st.ListGovernanceRecords("", "")
	if err != nil {
		return err
	}

	if err := os.WriteFile(filepath.Join(projectRoot, "PROJECT_STATUS.md"), []byte(renderProjectStatusMD(proj, phases, tasks, evs, govs)), 0o644); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(projectRoot, "TASKS.md"), []byte(renderTasksMD(tasks)), 0o644); err != nil {
		return err
	}

	roadmapPath := filepath.Join(projectRoot, "ROADMAP.md")
	if _, sErr := os.Stat(roadmapPath); os.IsNotExist(sErr) {
		if err := os.WriteFile(roadmapPath, []byte(renderRoadmapMD(proj, phases)), 0o644); err != nil {
			return err
		}
	}

	docsDir := filepath.Join(projectRoot, "docs")
	if err := os.MkdirAll(docsDir, 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(docsDir, "DECISIONS.md"), []byte(renderDecisionsMD(govs)), 0o644); err != nil {
		return err
	}
	if baseSnaps, sErr := st.GetSnapshots(workspace.BaselineTaskID); sErr == nil && len(baseSnaps) > 0 {
		toSync := []string{"PROJECT_STATUS.md", "TASKS.md", "ROADMAP.md", "docs/DECISIONS.md"}
		hasOpen := false
		for _, t := range tasks {
			if t.Status == "OPEN" {
				hasOpen = true
				break
			}
		}
		if !hasOpen {
			toSync = append(toSync, collectNonCodeDocDeliverables(st, projectRoot)...)
		}
		_ = workspace.AdvanceBaseline(st, projectRoot, toSync)
	}
	return nil
}

// collectNonCodeDocDeliverables thu thập các tệp tài liệu phi mã nguồn hợp lệ (không vi phạm khóa CIA hay ranh giới dự án con) để đồng bộ __BASELINE__.
func collectNonCodeDocDeliverables(st *storage.Store, projectRoot string) []string {
	var out []string
	for _, rootDoc := range []string{"README.md", "CHANGELOG.md"} {
		if info, err := os.Stat(filepath.Join(projectRoot, rootDoc)); err == nil && info.Mode().IsRegular() {
			out = append(out, rootDoc)
		}
	}
	docsRoot := filepath.Join(projectRoot, "docs")
	_ = filepath.WalkDir(docsRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		rel, rErr := workspace.NormalizeRelPath(projectRoot, path)
		if rErr != nil || workspace.IsSoftwareSourceOrManifestPath(rel) || workspace.IsLockedAsset(rel) {
			return nil
		}
		if workspace.CheckNoSubprojectCrossing(projectRoot, rel) != nil {
			return nil
		}
		if st != nil && checkBaselineCIALock(st, rel, strings.ToLower(rel)) != nil {
			return nil
		}
		out = append(out, rel)
		return nil
	})
	return out
}

// progressReportData gom toàn bộ số liệu tổng hợp phục vụ kết xuất báo cáo tiến độ 8 mục chuẩn SSDS §25.
type progressReportData struct {
	curPhaseStatus   string
	blockers         []string
	userActions      []string
	nextDeliverables []string
	doneTasks        []string
	nextTasks        []string
	scopedFiles      []string
	decLines         []string
	adrs             []string
	totalEv          int
	passEv           int
	failEv           int
}

// collectProgressReportData tổng hợp trạng thái pha, task, bằng chứng và bản ghi quản trị cho báo cáo tiến độ cuối phiên.
func collectProgressReportData(
	projectRoot, currentPhase string,
	phases []storage.SSDSPhase,
	tasks []storage.Task,
	evs []storage.Evidence,
	govs []storage.GovernanceRecord,
) progressReportData {
	d := progressReportData{curPhaseStatus: "IN_PROGRESS", totalEv: len(evs), adrs: listADRFiles(projectRoot)}
	for _, ph := range phases {
		if ph.PhaseID == currentPhase {
			d.curPhaseStatus = ph.Status
			d.nextDeliverables = ph.RequiredDeliverables
		}
		if ph.Status == "BLOCKED" {
			d.blockers = append(d.blockers, fmt.Sprintf("Phase %s is BLOCKED", ph.PhaseID))
		}
		if ph.Status == "REVIEW_REQUIRED" && ph.RequiresUserApproval {
			d.userActions = append(d.userActions, fmt.Sprintf("Approve phase %s (%s) deliverables to proceed", ph.PhaseID, ph.PrimaryRole))
		}
	}
	seenFile := make(map[string]bool)
	for _, t := range tasks {
		if t.ExecState == "VERIFIED" && t.Verdict == "PASS" {
			d.doneTasks = append(d.doneTasks, fmt.Sprintf("%s: %s (%s/%s)", t.ID, t.Title, t.ExecState, t.Verdict))
		} else if t.Status != "UNDONE" {
			d.nextTasks = append(d.nextTasks, fmt.Sprintf("%s: %s [%s/%s]", t.ID, t.Title, t.ExecState, t.Verdict))
		}
		if t.ExecState == "BLOCKED" {
			d.blockers = append(d.blockers, fmt.Sprintf("Task %s is BLOCKED", t.ID))
		}
		for _, f := range t.ScopeFiles {
			if !seenFile[f] {
				seenFile[f] = true
				d.scopedFiles = append(d.scopedFiles, f)
			}
		}
	}
	sort.Strings(d.scopedFiles)
	for _, ev := range evs {
		if ev.ExitCode == 0 {
			d.passEv++
		} else {
			d.failEv++
		}
	}
	for _, g := range govs {
		d.decLines = append(d.decLines, fmt.Sprintf("%s (%s/%s, %s, approved=%v)", g.RecordID, g.RecordKind, g.AuthorityLevel, g.Verdict, g.Approved))
		if g.AuthorityLevel == "CRITICAL" && !g.Approved {
			d.userActions = append(d.userActions, fmt.Sprintf("Explicit approval required for %s (%s in %s)", g.RecordID, g.RecordKind, g.PhaseID))
		}
	}
	return d
}

// RenderStandardProgressReport định dạng Báo cáo Tiến độ chuẩn cuối phiên làm việc gồm đúng 8 mục bắt buộc của SSDS §25.
func RenderStandardProgressReport(st *storage.Store, projectRoot string) (string, error) {
	proj, err := st.GetSSDSProject()
	if err != nil {
		return "", err
	}
	if proj == nil {
		return "", fmt.Errorf("SSDS project is not initialized")
	}
	phases, _ := st.ListSSDSPhases()
	tasks, _ := st.ListTasks()
	evs, _ := st.ListEvidence()
	govs, _ := st.ListGovernanceRecords("", "")
	d := collectProgressReportData(projectRoot, proj.CurrentPhase, phases, tasks, evs, govs)

	var b strings.Builder
	fmt.Fprintf(&b, "# SSDS Standard Progress Report (%s v%s)\n\n", StandardID, proj.AppliedSSDSVersion)
	b.WriteString("## 1. Project Status\n")
	fmt.Fprintf(&b, "- Current Phase: `%s` (`%s`)\n", proj.CurrentPhase, d.curPhaseStatus)
	fmt.Fprintf(&b, "- Current Milestone: `%s` workflow (`%s`)\n", proj.WorkflowTier, proj.Category)
	fmt.Fprintf(&b, "- Overall Status: `%s`\n", d.curPhaseStatus)
	fmt.Fprintf(&b, "- Current Blockers: %s\n\n", joinOrNone(d.blockers))

	b.WriteString("## 2. Completed Work\n")
	fmt.Fprintf(&b, "- Tasks completed: %s\n\n", joinOrNone(d.doneTasks))

	b.WriteString("## 3. Files Changed\n")
	fmt.Fprintf(&b, "- Tracked scope files: %s\n\n", joinOrNone(d.scopedFiles))

	b.WriteString("## 4. Testing\n")
	fmt.Fprintf(&b, "- Verification runs executed: `%d` (`PASS=%d`, `FAIL=%d`)\n", d.totalEv, d.passEv, d.failEv)
	if d.totalEv == 0 {
		b.WriteString("- Status: `NOT_EXECUTED` (no test execution claimed)\n\n")
	} else {
		b.WriteString("- Verification limitations: Bound to recorded command outputs in `.sen/state.db`.\n\n")
	}

	b.WriteString("## 5. Issues and Risks\n")
	if excErr := CheckActiveExceptions(st); excErr != nil {
		fmt.Fprintf(&b, "- Governance / Exception Alert: %s\n\n", excErr.Error())
	} else {
		b.WriteString("- Active blocking defects / expired exceptions: None.\n\n")
	}

	b.WriteString("## 6. Decisions\n")
	fmt.Fprintf(&b, "- Governance records: %s\n", joinOrNone(d.decLines))
	fmt.Fprintf(&b, "- ADRs in repository: %s\n\n", joinOrNone(d.adrs))

	b.WriteString("## 7. Next Actions\n")
	fmt.Fprintf(&b, "- Next authorized tasks: %s\n", joinOrNone(d.nextTasks))
	fmt.Fprintf(&b, "- Expected phase deliverables: %s\n\n", joinOrNone(d.nextDeliverables))

	b.WriteString("## 8. User Action Required\n")
	if len(d.userActions) == 0 {
		b.WriteString("- None — authorized to continue execution within approved SSDS scope (§23).\n")
	} else {
		for _, ua := range d.userActions {
			fmt.Fprintf(&b, "- %s\n", ua)
		}
	}
	return b.String(), nil
}

// renderProjectStatusMD sinh nội dung tệp PROJECT_STATUS.md làm nguồn sự thật trạng thái dự án trên repo (§24).
func renderProjectStatusMD(
	proj *storage.SSDSProject,
	phases []storage.SSDSPhase,
	tasks []storage.Task,
	evs []storage.Evidence,
	govs []storage.GovernanceRecord,
) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# PROJECT STATUS — %s\n\n", proj.ProjectName)
	fmt.Fprintf(&b, "- **Project ID:** `%s`\n", proj.ProjectID)
	fmt.Fprintf(&b, "- **Pinned SSDS Standard:** `%s v%s`\n", StandardID, proj.AppliedSSDSVersion)
	fmt.Fprintf(&b, "- **Category / Workflow:** `%s` / `%s`\n", proj.Category, proj.WorkflowTier)
	fmt.Fprintf(&b, "- **Current Phase:** `%s`\n", proj.CurrentPhase)
	fmt.Fprintf(&b, "- **Tasks / Evidence / Governance:** `%d` tasks | `%d` evidence rows | `%d` governance records\n\n", len(tasks), len(evs), len(govs))

	b.WriteString("## SDLC Phases\n\n")
	b.WriteString("| Phase | Primary Role | Status | Gate Passed | User Approval |\n")
	b.WriteString("|---|---|---|---|---|\n")
	for _, ph := range phases {
		app := "Not Required"
		if ph.RequiresUserApproval {
			if ph.ApprovedBy != "" {
				app = "Approved by " + ph.ApprovedBy
			} else {
				app = "Required"
			}
		}
		fmt.Fprintf(&b, "| `%s` | %s | `%s` | `%v` | %s |\n", ph.PhaseID, ph.PrimaryRole, ph.Status, ph.GatePassed, app)
	}
	return b.String()
}

// renderRoadmapMD sinh khung ROADMAP.md theo danh sách pha hoạt động của dự án khi tệp chưa tồn tại.
func renderRoadmapMD(proj *storage.SSDSProject, phases []storage.SSDSPhase) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# PROJECT ROADMAP — %s (`%s`)\n\n", proj.ProjectName, proj.WorkflowTier)
	for _, ph := range phases {
		deliv := "Source & verification evidence"
		if len(ph.RequiredDeliverables) > 0 {
			deliv = strings.Join(ph.RequiredDeliverables, ", ")
		}
		fmt.Fprintf(&b, "- **%s (%s)** — Status: `%s` | Deliverables: `%s`\n", ph.PhaseID, ph.PrimaryRole, ph.Status, deliv)
	}
	return b.String()
}

// renderTasksMD kết xuất bảng TASKS.md thể hiện rõ 4 trạng thái thực thi và 6 nhãn kết luận của SSDS §20.
func renderTasksMD(tasks []storage.Task) string {
	var b strings.Builder
	b.WriteString("# TASKS LEDGER\n\n")
	b.WriteString("| Task ID | Phase | Requirements | Components | Status | Exec State (§20) | Verdict (§20) | Scope Files |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|\n")
	for _, t := range tasks {
		fmt.Fprintf(&b, "| `%s` | `%s` | %s | %s | `%s` | `%s` | `%s` | %s |\n",
			t.ID, t.PhaseID, strings.Join(t.ReqIDs, ", "), strings.Join(t.ArchComponents, ", "),
			t.Status, t.ExecState, t.Verdict, strings.Join(t.ScopeFiles, ", "))
	}
	return b.String()
}

// renderDecisionsMD kết xuất sổ cái quyết định kiến trúc, phản biện, CIA và ngoại lệ ra docs/DECISIONS.md.
func renderDecisionsMD(govs []storage.GovernanceRecord) string {
	var b strings.Builder
	b.WriteString("# GOVERNANCE & DECISIONS LOG\n\n")
	b.WriteString("| ID | Kind | Phase | Authority | Evidence Mode | Verdict | Approved | Hash Prefix |\n")
	b.WriteString("|---|---|---|---|---|---|---|---|")
	b.WriteString("\n")
	for _, g := range govs {
		prefix := g.EntryHash
		if len(prefix) > 12 {
			prefix = prefix[:12]
		}
		fmt.Fprintf(&b, "| `%s` | `%s` | `%s` | `%s` | `%s` | `%s` | `%v` | `%s` |\n",
			g.RecordID, g.RecordKind, g.PhaseID, g.AuthorityLevel, g.EvidenceMode, g.Verdict, g.Approved, prefix)
	}
	return b.String()
}

// listADRFiles liệt kê danh sách các tệp quyết định kiến trúc ADR hiện có trong thư mục docs/adr/.
func listADRFiles(projectRoot string) []string {
	if strings.TrimSpace(projectRoot) == "" {
		return nil
	}
	entries, err := os.ReadDir(filepath.Join(projectRoot, "docs", "adr"))
	if err != nil {
		return nil
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(strings.ToLower(e.Name()), ".md") {
			out = append(out, "docs/adr/"+e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// joinOrNone nối mảng chuỗi bằng dấu chấm phẩy hoặc trả về 'None' nếu danh sách rỗng.
func joinOrNone(items []string) string {
	if len(items) == 0 {
		return "None"
	}
	return strings.Join(items, "; ")
}

