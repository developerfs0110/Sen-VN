// Tệp commands_exec.go cài đặt các lệnh kiểm chứng, hoàn tác, cổng nghiệm thu cuối và tra cứu trạng thái (verify, undo, done, status).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/developerfs0110/Sen-VN/internal/bridge"
	"github.com/developerfs0110/Sen-VN/internal/storage"
	"github.com/developerfs0110/Sen-VN/internal/verify"
	"github.com/developerfs0110/Sen-VN/internal/workspace"
)

// cmdVerify kiểm tra tính toàn vẹn phạm vi tệp, chạy trực tiếp binary kiểm thử và ghi bằng chứng băm nối chuỗi.
func cmdVerify(cwd string, args []string, stdout io.Writer) error {
	root := resolveProjectRoot(cwd)
	st, err := openExistingStore(root)
	if err != nil {
		return err
	}
	defer st.Close()

	before, cmdArgs := splitDoubleDash(args)
	if len(before) == 0 || strings.TrimSpace(before[0]) == "" {
		return errors.New("usage: sen verify <task_id> -- <bin> [args...]")
	}
	taskID := strings.TrimSpace(before[0])
	if err := workspace.ValidateTaskID(taskID); err != nil {
		return err
	}
	cfg, err := loadConfig(root)
	if err != nil {
		return err
	}
	ev, err := verify.RunTaskVerification(
		context.Background(), st, root, taskID, cfg.AllowedBinaries, cmdArgs, time.Duration(cfg.TimeoutSeconds)*time.Second,
	)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "VERIFIED %s: strength=%s tests_ran=%d hash=%s\n",
		taskID, ev.Strength, ev.TestsRan, ev.EntryHash[:12])
	return nil
}

// cmdUndo khôi phục chính xác từng byte cho task đang OPEN từ bản sao lưu snapshot và dọn tệp mới tạo.
func cmdUndo(cwd string, args []string, stdout io.Writer) error {
	root := resolveProjectRoot(cwd)
	st, err := openExistingStore(root)
	if err != nil {
		return err
	}
	defer st.Close()

	taskID := ""
	if len(args) > 0 {
		taskID = strings.TrimSpace(args[0])
	}
	// Nếu không chỉ định mã task, tự động tìm task đang ở trạng thái OPEN để hoàn tác.
	if taskID == "" {
		tasks, err := st.ListTasks()
		if err != nil {
			return err
		}
		for _, t := range tasks {
			if t.Status == "OPEN" {
				taskID = t.ID
				break
			}
		}
		if taskID == "" {
			return errors.New("no OPEN task found to undo")
		}
	} else if err := workspace.ValidateTaskID(taskID); err != nil {
		return err
	}
	restored, err := workspace.UndoTask(st, root, taskID)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "Undid task %s (restored/cleaned %d paths: %v)\n", taskID, len(restored), restored)
	return nil
}

// cmdDone thực thi cổng nghiệm thu 5 bước và xuất báo cáo bàn giao .sen/delivery-report.md khi đạt 100% yêu cầu.
func cmdDone(cwd string, args []string, stdout io.Writer) error {
	root := resolveProjectRoot(cwd)
	st, err := openExistingStore(root)
	if err != nil {
		return err
	}
	defer st.Close()

	_, suiteArgs := splitDoubleDash(args)
	cfg, err := loadConfig(root)
	if err != nil {
		return err
	}
	if len(suiteArgs) == 0 {
		suiteArgs = cfg.ProjectVerifyCmd
	}
	report, err := verify.EvaluateDoneGate(
		context.Background(), st, root, cfg.AllowedBinaries, suiteArgs, time.Duration(cfg.TimeoutSeconds)*time.Second,
	)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "SEN DONE — 100%% Requirements Verified\n\n%s", report.TableText)
	return nil
}

// cmdStatus hiển thị tổng quan luật dự án, kỹ năng đồng hành, yêu cầu, các task và trạng thái chuỗi băm bằng chứng.
func cmdStatus(cwd string, args []string, stdout io.Writer) error {
	root := resolveProjectRoot(cwd)
	st, err := openExistingStore(root)
	if err != nil {
		return err
	}
	defer st.Close()

	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	approve := fs.Bool("approve", false, "approve all pending requirements")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *approve {
		if err := st.ApproveAllRequirements(); err != nil {
			return err
		}
	}

	rules, _ := st.ListRules()
	reqs, _ := st.ListRequirements()
	tasks, _ := st.ListTasks()
	evs, _ := st.ListEvidence()
	chainStatus := "VALID"
	if err := st.VerifyEvidenceChain(); err != nil {
		chainStatus = "TAMPERED (" + err.Error() + ")"
	}
	comp := bridge.DetectCompanions(root)

	fmt.Fprintf(stdout, "=== SEN STATUS ===\n")
	fmt.Fprintf(stdout, "Rules: %d/%d | Companions: %s | Evidence: %d (%s)\n",
		len(rules), storage.MaxRules, comp.FormatCompact(), len(evs), chainStatus)
	fmt.Fprintln(stdout, "\nRequirements:")
	for _, r := range reqs {
		state := "PENDING"
		if r.Approved {
			state = "APPROVED"
		}
		fmt.Fprintf(stdout, "  [%s] %s — %s (quote: %q)\n", state, r.ID, r.Summary, r.UserQuote)
	}
	fmt.Fprintln(stdout, "\nTasks:")
	for _, t := range tasks {
		fmt.Fprintf(stdout, "  [%s] %s (kind=%s risk=%s reqs=%v files=%v)\n",
			t.Status, t.ID, t.Kind, t.RiskAuto, t.ReqIDs, t.ScopeFiles)
	}
	return nil
}
