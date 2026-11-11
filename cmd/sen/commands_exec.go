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

// cmdVerify kiểm tra tính toàn vẹn phạm vi tệp, giới hạn <= 3 vòng sửa lỗi, chạy binary kiểm thử và ghi bằng chứng băm nối chuỗi.
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

// cmdUndo khôi phục chính xác từng byte cho task đang OPEN từ bản sao lưu snapshot và xóa cờ cách ly Quarantine.
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

// cmdDone thực thi cổng nghiệm thu 5 bước đa chuyên ngành và xuất báo cáo bàn giao .sen/delivery-report.md nâng cao.
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
	opts := verify.DoneOptions{
		Version:       workspace.CurrentVersion,
		DomainProfile: workspace.ResolveDomainProfile(root, cfg.DomainProfile),
		ExpressMode:   cfg.ExpressMode,
		Companions:    bridge.DetectCompanions(root).FormatCompact(),
	}
	report, err := verify.EvaluateDoneGateWithOpts(
		context.Background(), st, root, cfg.AllowedBinaries, suiteArgs, time.Duration(cfg.TimeoutSeconds)*time.Second, opts,
	)
	if err != nil {
		return err
	}
	fmt.Fprintf(stdout, "SEN DONE — 100%% Requirements Verified\n\n%s", report.TableText)
	return nil
}

// cmdStatus hiển thị tổng quan chuyên ngành, chế độ tự hành, luật, yêu cầu, task, Quarantine và mở khóa khi có --approve.
func cmdStatus(cwd string, args []string, stdout io.Writer) error {
	root := resolveProjectRoot(cwd)
	st, err := openExistingStore(root)
	if err != nil {
		return err
	}
	defer st.Close()

	fs := flag.NewFlagSet("status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	approve := fs.Bool("approve", false, "approve all pending requirements and unlock quarantined OPEN tasks")
	express := fs.Bool("express", false, "toggle autonomous express mode")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if err := applyExpressFlagIfVisited(root, fs, *express); err != nil {
		return err
	}
	cfg, err := loadConfig(root)
	if err != nil {
		return err
	}
	if *approve {
		if err := approveAndUnlockQuarantine(st, root, stdout); err != nil {
			return err
		}
	}
	renderStatusOverview(st, root, cfg, stdout)
	return nil
}

// approveAndUnlockQuarantine phê duyệt toàn bộ yêu cầu đang chờ và đặt lại bộ đếm vòng lặp sửa lỗi cho các task OPEN bị kẹt.
func approveAndUnlockQuarantine(st *storage.Store, root string, stdout io.Writer) error {
	if err := st.ApproveAllRequirements(); err != nil {
		return err
	}
	tasks, err := st.ListTasks()
	if err != nil {
		return err
	}
	for _, t := range tasks {
		if t.Status != "OPEN" {
			continue
		}
		failed, _ := workspace.ActiveFailedVerifyCount(st, root, t.ID)
		if failed > 0 {
			if err := workspace.WriteQuarantineReset(st, root, t.ID); err != nil {
				return err
			}
			fmt.Fprintf(stdout, "Unlocked repair loop for task %s (previous failed verifies=%d)\n", t.ID, failed)
		}
	}
	return nil
}

// renderStatusOverview in bảng trạng thái chi tiết của dự án ra luồng stdout.
func renderStatusOverview(st *storage.Store, root string, cfg Config, stdout io.Writer) {
	rules, _ := st.ListRules()
	reqs, _ := st.ListRequirements()
	tasks, _ := st.ListTasks()
	evs, _ := st.ListEvidence()
	chainStatus := "VALID"
	if err := st.VerifyEvidenceChain(); err != nil {
		chainStatus = "TAMPERED (" + err.Error() + ")"
	}
	comp := bridge.DetectCompanions(root)
	prof := workspace.ResolveDomainProfile(root, cfg.DomainProfile)
	expLabel := "off"
	if cfg.ExpressMode {
		expLabel = "on"
	}

	fmt.Fprintf(stdout, "=== SEN STATUS ===\n")
	fmt.Fprintf(stdout, "Profile: %s | Express: %s | Rules: %d/%d | Companions: %s | Evidence: %d (%s)\n",
		prof, expLabel, len(rules), storage.MaxRules, comp.FormatCompact(), len(evs), chainStatus)
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
		qTag := ""
		if t.Status == "OPEN" {
			if failed, _ := workspace.ActiveFailedVerifyCount(st, root, t.ID); failed >= workspace.MaxRepairRounds {
				qTag = " [QUARANTINE 3/3]"
			}
		}
		fmt.Fprintf(stdout, "  [%s]%s %s (kind=%s risk=%s reqs=%v files=%v)\n",
			t.Status, qTag, t.ID, t.Kind, t.RiskAuto, t.ReqIDs, t.ScopeFiles)
	}
}
