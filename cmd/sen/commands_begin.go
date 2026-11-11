// Tệp commands_begin.go cài đặt lệnh sen begin gắn kèm tường lửa pha SSDS, liên kết thành phần kiến trúc và sao lưu snapshot.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"

	"github.com/developerfs0110/Sen-VN/internal/ssds"
	"github.com/developerfs0110/Sen-VN/internal/storage"
	"github.com/developerfs0110/Sen-VN/internal/workspace"
)

// beginOptions gom nhóm toàn bộ tham số mở task của lệnh sen begin để giữ chữ ký hàm tinh gọn.
type beginOptions struct {
	taskID      string
	reqsCSV     string
	filesCSV    string
	kindFlag    string
	titleFlag   string
	phaseID     string
	components  []string
	allowDelete bool
}

// cmdBegin mở một task mới với phạm vi tệp tường minh, kiểm tra tường lửa pha SSDS, khóa test/media gốc và sao lưu snapshot.
func cmdBegin(cwd string, args []string, stdout io.Writer) error {
	root := resolveProjectRoot(cwd)
	st, err := openExistingStore(root)
	if err != nil {
		return err
	}
	defer st.Close()

	reordered := reorderFlagsFirst(args, map[string]bool{
		"reqs": true, "files": true, "kind": true, "title": true, "phase": true, "components": true,
	})
	fs := flag.NewFlagSet("begin", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	reqsCSV := fs.String("reqs", "", "comma-separated requirement IDs")
	filesCSV := fs.String("files", "", "comma-separated scope file paths")
	kindFlag := fs.String("kind", "", "feature|bugfix|ui")
	phaseFlag := fs.String("phase", "P5", "SDLC phase ID (default P5)")
	componentsCSV := fs.String("components", "", "comma-separated architecture component IDs (e.g. AUTH-001,DB-001)")
	allowDelete := fs.Bool("allow-delete", false, "permit deleting declared existing scope files (LARGE risk)")
	express := fs.Bool("express", false, "toggle autonomous express mode")
	titleFlag := fs.String("title", "", "task title")
	if err := fs.Parse(reordered); err != nil {
		return err
	}
	if err := applyExpressFlagIfVisited(root, fs, *express); err != nil {
		return err
	}
	if len(fs.Args()) == 0 {
		return errors.New("missing <task_id>")
	}
	taskID := strings.TrimSpace(fs.Args()[0])
	if err := workspace.ValidateTaskID(taskID); err != nil {
		return err
	}
	if err := ensureCanOpenNewTask(st, taskID); err != nil {
		return err
	}

	phaseID := strings.ToUpper(strings.TrimSpace(*phaseFlag))
	if proj, _ := st.GetSSDSProject(); proj != nil {
		if !flagVisited(fs, "phase") || phaseID == "" {
			phaseID = proj.CurrentPhase
		} else if !strings.EqualFold(phaseID, proj.CurrentPhase) {
			return fmt.Errorf("SSDS phase mismatch: `--phase %s` does not match active project phase %s", phaseID, proj.CurrentPhase)
		}
	} else if phaseID == "" {
		phaseID = "P5"
	}
	opts := beginOptions{
		taskID:      taskID,
		reqsCSV:     *reqsCSV,
		filesCSV:    *filesCSV,
		kindFlag:    *kindFlag,
		titleFlag:   *titleFlag,
		phaseID:     phaseID,
		components:  splitCSV(*componentsCSV),
		allowDelete: *allowDelete,
	}
	return createAndSnapshotTask(st, root, opts, stdout)
}

// createAndSnapshotTask thực thi kiểm tra tường lửa pha SSDS, đối chiếu yêu cầu, chụp snapshot và lưu task mới vào SQLite.
func createAndSnapshotTask(st *storage.Store, root string, opts beginOptions, stdout io.Writer) error {
	normFiles, err := resolveBeginFiles(root, opts.filesCSV)
	if err != nil {
		return err
	}
	if err := enforceSSDSBeginGuard(st, root, normFiles); err != nil {
		return err
	}
	reqIDs, hasBugfixReq, err := resolveBeginReqs(st, opts.reqsCSV)
	if err != nil {
		return err
	}
	kind, err := workspace.ValidateKind(opts.kindFlag)
	if err != nil {
		return err
	}
	if hasBugfixReq {
		kind = "BUGFIX"
	} else if kind == "" {
		kind = "FEATURE"
	}
	if err := workspace.BeginTaskSnapshotWithOpts(st, root, opts.taskID, normFiles, opts.allowDelete); err != nil {
		return err
	}
	risk, reasons := workspace.ComputeRiskWithOpts(normFiles, kind, opts.allowDelete, root)
	title := strings.TrimSpace(opts.titleFlag)
	if title == "" {
		title = fmt.Sprintf("%s (%s)", opts.taskID, strings.Join(reqIDs, ","))
	}
	if err := st.CreateTask(storage.Task{
		ID:             opts.taskID,
		ReqIDs:         reqIDs,
		Title:          title,
		Kind:           kind,
		RiskAuto:       risk,
		ScopeFiles:     normFiles,
		Status:         "OPEN",
		PhaseID:        opts.phaseID,
		ArchComponents: opts.components,
		ExecState:      "IMPLEMENTED",
		Verdict:        "NOT_EXECUTED",
	}); err != nil {
		return err
	}
	if proj, _ := st.GetSSDSProject(); proj != nil {
		_ = ssds.SyncProjectStateFiles(st, root)
	}
	fmt.Fprintf(stdout, "Opened task %s [phase=%s kind=%s risk=%s reqs=%v files=%v]\n",
		opts.taskID, opts.phaseID, kind, risk, reqIDs, normFiles)
	if len(reasons) > 0 {
		fmt.Fprintf(stdout, "Risk factors: %s\n", strings.Join(reasons, "; "))
	}
	return nil
}

// enforceSSDSBeginGuard chặn vật lý lệnh sen begin khi tệp vi phạm ranh giới dự án con, tường lửa pha hoặc chưa qua P1..P4.
func enforceSSDSBeginGuard(st *storage.Store, root string, normFiles []string) error {
	for _, f := range normFiles {
		if err := workspace.CheckNoSubprojectCrossing(root, f); err != nil {
			return err
		}
	}
	proj, err := st.GetSSDSProject()
	if err != nil || proj == nil {
		return err
	}
	allowedCodePhases := map[string]bool{"P4": true, "P5": true, "P6": true, "P7": true, "P9": true}
	for _, f := range normFiles {
		if fwErr := ssds.CheckWriteFirewall(st, root, f); fwErr != nil {
			return fwErr
		}
		if workspace.IsSoftwareSourceOrManifestPath(f) && proj.Category != "PROTOTYPE" {
			if !allowedCodePhases[proj.CurrentPhase] {
				return fmt.Errorf("SSDS phase guard: cannot open coding task for %q during phase %s (complete and approve upstream phases first)",
					f, proj.CurrentPhase)
			}
			if proj.CurrentPhase == "P5" {
				if preErr := ssds.CanBeginImplementationTask(st, root); preErr != nil {
					return preErr
				}
			}
		}
	}
	return nil
}

// ensureCanOpenNewTask đảm bảo mã task chưa từng được dùng và không có task nào khác đang mở (OPEN).
func ensureCanOpenNewTask(st *storage.Store, taskID string) error {
	tasks, err := st.ListTasks()
	if err != nil {
		return err
	}
	for _, t := range tasks {
		if t.ID == taskID {
			return fmt.Errorf("task %q already exists (status=%s); choose a new task ID", taskID, t.Status)
		}
		if t.Status == "OPEN" {
			return fmt.Errorf("cannot begin %q while task %q is still OPEN", taskID, t.ID)
		}
	}
	return nil
}

// resolveBeginReqs đối chiếu danh sách mã yêu cầu R_i / REQ-* với sổ cái và phát hiện cờ sửa lỗi (IsBugfix).
func resolveBeginReqs(st *storage.Store, reqsCSV string) ([]string, bool, error) {
	reqIDs := splitCSV(reqsCSV)
	if len(reqIDs) == 0 {
		return nil, false, errors.New("missing --reqs (e.g. --reqs R1 or --reqs REQ-F-001)")
	}
	existingReqs, err := st.ListRequirements()
	if err != nil {
		return nil, false, err
	}
	reqMap := make(map[string]storage.Requirement, len(existingReqs))
	for _, r := range existingReqs {
		reqMap[r.ID] = r
		reqMap[strings.ToUpper(r.ID)] = r
	}
	hasBugfix := false
	var resolved []string
	for _, id := range reqIDs {
		r, ok := reqMap[id]
		if !ok {
			r, ok = reqMap[strings.ToUpper(id)]
		}
		if !ok {
			return nil, false, fmt.Errorf("unknown requirement %q (record via `sen intake` or `sen sdlc gate P1` first)", id)
		}
		resolved = append(resolved, r.ID)
		if r.IsBugfix {
			hasBugfix = true
		}
	}
	return resolved, hasBugfix, nil
}

// resolveBeginFiles chuẩn hóa và kiểm tra an toàn đường dẫn tương đối cho toàn bộ tệp thuộc phạm vi task.
func resolveBeginFiles(root, filesCSV string) ([]string, error) {
	rawFiles := splitCSV(filesCSV)
	if len(rawFiles) == 0 {
		return nil, errors.New("missing --files (declare exact scope files)")
	}
	var normFiles []string
	for _, f := range rawFiles {
		nf, err := workspace.NormalizeRelPath(root, f)
		if err != nil {
			return nil, err
		}
		normFiles = append(normFiles, nf)
	}
	return normFiles, nil
}

// splitCSV tách chuỗi phân cách bằng dấu phẩy thành danh sách các phần tử đã cắt khoảng trắng.
func splitCSV(s string) []string {
	var out []string
	for _, part := range strings.Split(s, ",") {
		p := strings.TrimSpace(part)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

