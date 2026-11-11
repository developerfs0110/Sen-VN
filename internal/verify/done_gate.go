// Tệp done_gate.go thực thi cổng nghiệm thu 5 điều kiện của lệnh sen done và xuất báo cáo .sen/delivery-report.md.
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
	"github.com/developerfs0110/Sen-VN/internal/storage"
	"github.com/developerfs0110/Sen-VN/internal/workspace"
)

// FinalGateTaskID định danh bản ghi bằng chứng cho lần chạy bộ kiểm thử tổng thể cuối cùng tại cổng sen done.
const FinalGateTaskID = "__DONE__"

// DoneRow biểu diễn một dòng đối soát truy vết từ mã yêu cầu R_i tới câu gốc của người dùng và lệnh kiểm chứng đã pass.
type DoneRow struct {
	ReqID     string
	UserQuote string
	Summary   string
	TaskID    string
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

// EvaluateDoneGate kiểm chứng chuỗi băm, toàn vẹn thư mục, độ phủ 100% R_i, chạy lại bộ test dự án và ghi báo cáo bàn giao.
func EvaluateDoneGate(
	ctx context.Context,
	st *storage.Store,
	root string,
	allowedBins []string,
	finalSuite []string,
	timeout time.Duration,
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

	table := redact.ScrubText(formatCoverageTable(rows, finalEv))
	mdPath := filepath.Join(root, ".sen", "delivery-report.md")
	if err := os.MkdirAll(filepath.Dir(mdPath), 0o755); err != nil {
		return nil, err
	}
	if err := os.WriteFile(mdPath, []byte("# Sen MVP Delivery Report\n\n"+table), 0o644); err != nil {
		return nil, err
	}

	return &DoneReport{
		Rows:               rows,
		FinalSuiteEvidence: finalEv,
		MarkdownPath:       mdPath,
		TableText:          table,
	}, nil
}

// verifyPreDoneState kiểm tra chuỗi băm bằng chứng từ GENESIS, không còn task OPEN và không có tệp trôi lệch khỏi __BASELINE__.
func verifyPreDoneState(st *storage.Store, root string) error {
	if err := st.VerifyEvidenceChain(); err != nil {
		return fmt.Errorf("evidence hash-chain violation: %w", err)
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
	if err := workspace.CheckIntegrity(st, root, "", nil); err != nil {
		return fmt.Errorf("workspace integrity violation: %w", err)
	}
	return nil
}

// queryRequirementCoverage đối soát SQL (json_each) bắt buộc 100% yêu cầu R_i đều có task VERIFIED đạt exit_code = 0.
func queryRequirementCoverage(st *storage.Store) ([]DoneRow, error) {
	reqs, err := st.ListRequirements()
	if err != nil {
		return nil, err
	}
	if len(reqs) == 0 {
		return nil, errors.New("zero requirements recorded: run `sen intake` before `sen done`")
	}

	var rows []DoneRow
	var uncovered []string
	for _, r := range reqs {
		row := DoneRow{ReqID: r.ID, UserQuote: r.UserQuote, Summary: r.Summary}
		mustTest := 0
		if r.IsBugfix {
			mustTest = 1
		}
		err := st.DB().QueryRow(`
			SELECT t.id, e.strength, e.tests_ran, e.command
			FROM tasks t
			JOIN json_each(t.req_ids_json) j ON j.value = ?
			JOIN evidence e ON e.task_id = t.id
			WHERE t.status = 'VERIFIED'
			  AND e.exit_code = 0
			  AND (e.strength != 'TEST' OR e.tests_ran > 0)
			  AND (? = 0 OR (e.strength = 'TEST' AND e.tests_ran > 0))
			ORDER BY
			  CASE e.strength WHEN 'TEST' THEN 3 WHEN 'BUILD' THEN 2 ELSE 1 END DESC,
			  e.seq DESC
			LIMIT 1
		`, r.ID, mustTest).Scan(&row.TaskID, &row.Strength, &row.TestsRan, &row.Command)

		if errors.Is(err, sql.ErrNoRows) {
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

// formatCoverageTable định dạng bảng đối chiếu R_i <-> Câu gốc người dùng <-> Lệnh kiểm chứng cho báo cáo nghiệm thu.
func formatCoverageTable(rows []DoneRow, finalEv *storage.Evidence) string {
	var b strings.Builder
	b.WriteString("| Req | User Quote | Task | Strength (TestsRan) | Command |\n")
	b.WriteString("|---|---|---|---|---|\n")
	for _, r := range rows {
		b.WriteString(fmt.Sprintf("| %s | %s | %s | %s (%d) | `%s` |\n",
			r.ReqID,
			sanitizeCell(r.UserQuote, 48),
			r.TaskID,
			r.Strength,
			r.TestsRan,
			sanitizeCell(r.Command, 56),
		))
	}
	if finalEv != nil {
		b.WriteString(fmt.Sprintf("\n**Final Live Suite:** `%s` — `PASS` (`%s`, `%d` tests, hash `%s`)\n",
			finalEv.Command, finalEv.Strength, finalEv.TestsRan, finalEv.EntryHash[:12]))
	}
	return b.String()
}
