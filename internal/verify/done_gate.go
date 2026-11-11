// Tệp done_gate.go thực thi cổng nghiệm thu 5 điều kiện của lệnh sen done và xuất báo cáo .sen/delivery-report.md nâng cao.
package verify

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/developerfs0110/Sen-VN/internal/redact"
	"github.com/developerfs0110/Sen-VN/internal/ssds"
	"github.com/developerfs0110/Sen-VN/internal/storage"
	"github.com/developerfs0110/Sen-VN/internal/workspace"
)

// FinalGateTaskID định danh bản ghi bằng chứng cho lần chạy bộ kiểm thử tổng thể cuối cùng tại cổng sen done.
const FinalGateTaskID = "__DONE__"

// DoneOptions truyền siêu dữ liệu phiên bản, chuyên ngành, chế độ tự hành và gói đồng hành vào báo cáo nghiệm thu.
type DoneOptions struct {
	Version       string
	DomainProfile string
	ExpressMode   bool
	Companions    string
}

// DoneRow biểu diễn một dòng đối soát truy vết từ mã yêu cầu R_i tới bậc rủi ro và bằng chứng kiểm chứng đã pass.
type DoneRow struct {
	ReqID     string
	UserQuote string
	Summary   string
	TaskID    string
	RiskTier  string
	Strength  string
	TestsRan  int
	Command   string
}

// DoneReport chứa toàn bộ kết quả nghiệm thu và nội dung bảng Markdown xuất ra cho người dùng kiểm tra.
type DoneReport struct {
	Rows               []DoneRow
	FinalSuiteEvidence *storage.Evidence
	MarkdownPath       string
	TableText          string
}

// EvaluateDoneGate giữ nguyên chữ ký tương thích ngược và ủy quyền cho EvaluateDoneGateWithOpts với cấu hình mặc định.
func EvaluateDoneGate(
	ctx context.Context,
	st *storage.Store,
	root string,
	allowedBins []string,
	finalSuite []string,
	timeout time.Duration,
) (*DoneReport, error) {
	return EvaluateDoneGateWithOpts(ctx, st, root, allowedBins, finalSuite, timeout, DoneOptions{})
}

// EvaluateDoneGateWithOpts kiểm chứng chuỗi băm, toàn vẹn thư mục, độ phủ 100% R_i (hỗ trợ đa chuyên ngành) và xuất báo cáo.
func EvaluateDoneGateWithOpts(
	ctx context.Context,
	st *storage.Store,
	root string,
	allowedBins []string,
	finalSuite []string,
	timeout time.Duration,
	opts DoneOptions,
) (*DoneReport, error) {
	if err := verifyPreDoneState(st, root); err != nil {
		return nil, err
	}
	rows, err := queryRequirementCoverage(st)
	if err != nil {
		return nil, err
	}

	var finalEv *storage.Evidence
	if len(finalSuite) > 0 {
		finalEv, err = runFinalLiveSuite(ctx, st, root, allowedBins, finalSuite, timeout)
		if err != nil {
			return nil, err
		}
	}

	proj, _ := st.GetSSDSProject()
	opts = normalizeDoneOptions(root, opts)
	table := redact.ScrubText(formatCoverageTable(rows, finalEv, opts, proj))
	mdPath := filepath.Join(root, ".sen", "delivery-report.md")
	if err := os.MkdirAll(filepath.Dir(mdPath), 0o755); err != nil {
		return nil, err
	}
	header := fmt.Sprintf("# Sen-VN v%s Delivery Report\n\n", strings.TrimPrefix(opts.Version, "v"))
	if err := os.WriteFile(mdPath, []byte(header+table), 0o644); err != nil {
		return nil, err
	}
	if proj != nil {
		_ = ssds.SyncProjectStateFiles(st, root)
	}

	return &DoneReport{
		Rows:               rows,
		FinalSuiteEvidence: finalEv,
		MarkdownPath:       mdPath,
		TableText:          table,
	}, nil
}

// normalizeDoneOptions điền giá trị mặc định cho phiên bản và chuyên ngành nếu người gọi không truyền tường minh.
func normalizeDoneOptions(root string, opts DoneOptions) DoneOptions {
	if strings.TrimSpace(opts.Version) == "" {
		opts.Version = workspace.CurrentVersion
	}
	opts.DomainProfile = workspace.ResolveDomainProfile(root, opts.DomainProfile)
	if strings.TrimSpace(opts.Companions) == "" {
		opts.Companions = "none"
	}
	return opts
}

// verifyPreDoneState kiểm tra chuỗi băm bằng chứng/quản trị, các pha SSDS bắt buộc, không còn task OPEN và toàn vẹn __BASELINE__.
func verifyPreDoneState(st *storage.Store, root string) error {
	if err := st.VerifyEvidenceChain(); err != nil {
		return fmt.Errorf("evidence hash-chain violation: %w", err)
	}
	if err := st.VerifyGovernanceChain(); err != nil {
		return fmt.Errorf("governance hash-chain violation: %w", err)
	}
	tasks, err := st.ListTasks()
	if err != nil {
		return err
	}
	for _, t := range tasks {
		if t.Status == "OPEN" {
			return fmt.Errorf("task %q is still OPEN (run `sen verify %s -- ...` or `sen undo %s`)", t.ID, t.ID, t.ID)
		}
	}
	proj, err := st.GetSSDSProject()
	if err != nil {
		return err
	}
	if proj != nil {
		if err := ssds.CheckActiveExceptions(st); err != nil {
			return err
		}
		if violations := ssds.VerifyApprovedBaselineIntegrity(st, root); len(violations) > 0 {
			return fmt.Errorf("SSDS §17 approved baseline violation: %s", strings.Join(violations, "; "))
		}
		for _, reqPhase := range proj.ActivePhases {
			if strings.EqualFold(reqPhase, "P9") {
				continue
			}
			ph, pErr := st.GetSSDSPhase(reqPhase)
			if pErr != nil {
				return fmt.Errorf("SSDS required phase %s missing: %w", reqPhase, pErr)
			}
			if !ph.GatePassed || (ph.Status != "APPROVED" && ph.Status != "COMPLETED") {
				return fmt.Errorf("SSDS phase %s must pass quality gate and reach APPROVED/COMPLETED before sen done (status=%s, gate_passed=%v)",
					reqPhase, ph.Status, ph.GatePassed)
			}
		}
	}
	if err := workspace.CheckIntegrity(st, root, "", nil); err != nil {
		return fmt.Errorf("workspace integrity violation: %w", err)
	}
	return nil
}

// queryRequirementCoverage đối soát SQL (json_each) bắt buộc 100% yêu cầu R_i có task VERIFIED hợp lệ (TEST, MEDIA, BUILD, RUN).
func queryRequirementCoverage(st *storage.Store) ([]DoneRow, error) {
	reqs, err := st.ListRequirements()
	if err != nil {
		return nil, err
	}
	if len(reqs) == 0 {
		return nil, errors.New("zero requirements recorded: run `sen intake` before `sen done`")
	}

	proj, _ := st.GetSSDSProject()
	var rows []DoneRow
	var uncovered []string
	for _, r := range reqs {
		row := DoneRow{ReqID: r.ID, UserQuote: r.UserQuote, Summary: r.Summary}
		mustTest := 0
		if r.IsBugfix {
			mustTest = 1
		}
		err := st.DB().QueryRow(`
			SELECT t.id, t.risk_auto, e.strength, e.tests_ran, e.command
			FROM tasks t
			JOIN json_each(t.req_ids_json) j ON j.value = ?
			JOIN evidence e ON e.task_id = t.id
			WHERE t.status = 'VERIFIED'
			  AND e.exit_code = 0
			  AND (e.strength NOT IN ('TEST', 'MEDIA') OR e.tests_ran > 0)
			  AND (? = 0 OR (e.strength = 'TEST' AND e.tests_ran > 0))
			ORDER BY
			  CASE e.strength WHEN 'TEST' THEN 4 WHEN 'MEDIA' THEN 3 WHEN 'BUILD' THEN 2 ELSE 1 END DESC,
			  e.seq DESC
			LIMIT 1
		`, r.ID, mustTest).Scan(&row.TaskID, &row.RiskTier, &row.Strength, &row.TestsRan, &row.Command)

		if errors.Is(err, sql.ErrNoRows) {
			if proj != nil && (r.ReqType == "BUS_RULE" || r.ReqType == "CONSTRAINT") {
				continue
			}
			uncovered = append(uncovered, r.ID)
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("coverage query for %s: %w", r.ID, err)
		}
		rows = append(rows, row)
	}
	if len(uncovered) > 0 {
		return nil, fmt.Errorf("uncovered requirements missing VERIFIED passing evidence: %s", strings.Join(uncovered, ", "))
	}
	if len(rows) == 0 {
		return nil, errors.New("zero functional/non-functional requirements verified with passing evidence")
	}
	return rows, nil
}

// runFinalLiveSuite chạy lại bộ kiểm thử toàn dự án ngay tại thời điểm nghiệm thu để phát hiện hồi quy chéo giữa các task.
func runFinalLiveSuite(
	ctx context.Context,
	st *storage.Store,
	root string,
	allowedBins []string,
	finalSuite []string,
	timeout time.Duration,
) (*storage.Evidence, error) {
	res, err := executeFilteredSubprocess(ctx, root, allowedBins, finalSuite, timeout)
	if err != nil {
		return nil, fmt.Errorf("final verification suite error: %w", err)
	}
	storedExit := res.rawExitCode
	storedOut := res.redactedOutput
	if storedExit == 0 && res.zeroTestErr != nil {
		storedExit = 1
		storedOut += "\n[SEN REJECTED] " + res.zeroTestErr.Error()
	}
	ev, err := st.AppendEvidence(FinalGateTaskID, res.commandLine, res.strength, res.testsRan, storedExit, storedOut)
	if err != nil {
		return nil, err
	}
	if res.zeroTestErr != nil {
		return nil, fmt.Errorf("final verification suite failed empty-test check: %w", res.zeroTestErr)
	}
	if res.rawExitCode != 0 || res.execErr != nil {
		return nil, fmt.Errorf("final verification suite exited with code %d: %s", res.rawExitCode, res.commandLine)
	}
	if err := workspace.CheckIntegrity(st, root, "", nil); err != nil {
		return nil, fmt.Errorf("post-suite workspace integrity violation: %w", err)
	}
	return ev, nil
}

// sanitizeCell làm sạch ký tự xuống dòng và dấu gạch đứng để không làm vỡ bảng Markdown.
func sanitizeCell(s string, max int) string {
	s = strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(s), "\r", " "), "\n", " ")
	s = strings.ReplaceAll(s, "|", "\\|")
	if len(s) > max && max > 3 {
		return s[:max-3] + "..."
	}
	return s
}

// formatCoverageTable định dạng siêu dữ liệu Sen-VN v1.2.0 / SSDS và bảng đối chiếu R_i <-> Risk Tier <-> Strength/Streams.
func formatCoverageTable(rows []DoneRow, finalEv *storage.Evidence, opts DoneOptions, proj *storage.SSDSProject) string {
	var b strings.Builder
	expressLabel := "off"
	if opts.ExpressMode {
		expressLabel = "on"
	}
	b.WriteString(fmt.Sprintf("- **Sen-VN Version:** `v%s`\n", strings.TrimPrefix(opts.Version, "v")))
	if proj != nil {
		b.WriteString(fmt.Sprintf("- **SSDS Metadata:** Standard: SEN-STD-SDLC-001 v%s | Category: %s | Workflow: %s\n",
			proj.AppliedSSDSVersion, proj.Category, proj.WorkflowTier))
	}
	b.WriteString(fmt.Sprintf("- **Domain Profile:** `%s` | **Express Mode:** `%s`\n", opts.DomainProfile, expressLabel))
	b.WriteString(fmt.Sprintf("- **Companions:** `%s`\n\n", opts.Companions))

	b.WriteString("| Req | User Quote | Task | Risk Tier | Strength/Streams | Command |\n")
	b.WriteString("|---|---|---|---|---|---|\n")
	for _, r := range rows {
		tier := r.RiskTier
		if strings.TrimSpace(tier) == "" {
			tier = "SMALL"
		}
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s | %s (%d) | `%s` |\n",
			r.ReqID,
			sanitizeCell(r.UserQuote, 48),
			r.TaskID,
			tier,
			r.Strength,
			r.TestsRan,
			sanitizeCell(r.Command, 56),
		))
	}
	if finalEv != nil {
		b.WriteString(fmt.Sprintf("\n**Final Live Suite:** `%s` — `PASS` (`%s`, `%d` tests/streams, hash `%s`)\n",
			finalEv.Command, finalEv.Strength, finalEv.TestsRan, finalEv.EntryHash[:12]))
	}
	return b.String()
}
