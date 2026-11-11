// Tệp commands.go cài đặt các lệnh khởi tạo, tóm tắt ngữ cảnh, tiếp nhận yêu cầu và mở task (init, brief, intake, begin).
package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/developerfs0110/Sen-VN/internal/bridge"
	"github.com/developerfs0110/Sen-VN/internal/storage"
	"github.com/developerfs0110/Sen-VN/internal/verify"
	"github.com/developerfs0110/Sen-VN/internal/workspace"
)

// cmdInit khởi tạo sổ cái .sen/state.db, cấu hình mặc định, AGENTS.md, hooks.json và chụp mốc nền __BASELINE__.
func cmdInit(cwd string, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	withCompanions := fs.Bool("with-companions", false, "report companion discovery")
	agyHooks := fs.Bool("agy-hooks", false, "write .agents/hooks.json")
	overwriteAgents := fs.Bool("overwrite-agents", false, "overwrite AGENTS.md")
	if err := fs.Parse(args); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Join(cwd, ".sen", "snapshots"), 0o755); err != nil {
		return err
	}
	st, err := storage.Open(filepath.Join(cwd, ".sen", "state.db"))
	if err != nil {
		return err
	}
	defer st.Close()

	cfgPath := filepath.Join(cwd, ".sen", "config.json")
	if !fileExists(cfgPath) {
		defaultCfg := Config{
			Version:          "1.0",
			AllowedBinaries:  verify.DefaultAllowedBinaries(),
			ProjectVerifyCmd: detectDefaultVerifyCmd(cwd),
			TimeoutSeconds:   120,
		}
		if err := saveConfig(cwd, defaultCfg); err != nil {
			return err
		}
	}

	agentsPath := filepath.Join(cwd, "AGENTS.md")
	if *overwriteAgents || !fileExists(agentsPath) {
		if err := os.WriteFile(agentsPath, []byte(bridge.RenderAgentsMD()), 0o644); err != nil {
			return err
		}
	}

	comp := bridge.DetectCompanions(cwd)
	if *agyHooks || comp.AntigravityEnv {
		if err := bridge.WriteAntigravityHooksFile(filepath.Join(cwd, ".agents", "hooks.json")); err != nil {
			return err
		}
	}

	if err := workspace.CaptureBaseline(st, cwd); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "Initialized Sen MVP in %s\n", cwd)
	if *withCompanions || comp.AntigravityEnv {
		fmt.Fprintf(stdout, "Companions: %s\n", comp.FormatCompact())
	}
	return nil
}

// cmdBrief xuất bản tóm tắt ngắn (<250 token) cho đầu phiên làm việc hoặc chạy bộ chuyển đổi JSON hook của Antigravity.
func cmdBrief(cwd string, args []string, stdin io.Reader, stdout io.Writer) error {
	reordered := reorderFlagsFirst(args, map[string]bool{"path": true})
	fs := flag.NewFlagSet("brief", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	targetPath := fs.String("path", "", "filter rules by path")
	hookMode := fs.Bool("hook", false, "run Antigravity JSON hook mode")
	if err := fs.Parse(reordered); err != nil {
		return err
	}

	root := resolveProjectRoot(cwd)
	if *hookMode {
		fallbackEvent := ""
		if len(fs.Args()) > 0 {
			fallbackEvent = fs.Args()[0]
		}
		return runHookBrief(root, fallbackEvent, stdin, stdout)
	}

	var st *storage.Store
	if fileExists(filepath.Join(root, ".sen", "state.db")) {
		var err error
		st, err = storage.Open(filepath.Join(root, ".sen", "state.db"))
		if err != nil {
			return err
		}
		defer st.Close()
	}
	fmt.Fprint(stdout, bridge.FormatBrief(st, root, *targetPath))
	return nil
}

// runHookBrief tách biệt thư mục cài plugin với thư mục workspace của người dùng dựa trên trường cwd từ stdin JSON.
func runHookBrief(root, fallbackEvent string, stdin io.Reader, stdout io.Writer) error {
	var rawBuf []byte
	if stdin != nil {
		rawBuf, _ = io.ReadAll(stdin)
	}
	var probe struct {
		Cwd            string   `json:"cwd"`
		WorkspacePaths []string `json:"workspacePaths"`
	}
	if len(rawBuf) > 0 {
		_ = json.Unmarshal(rawBuf, &probe)
		if strings.TrimSpace(probe.Cwd) != "" {
			root = strings.TrimSpace(probe.Cwd)
		} else if len(probe.WorkspacePaths) > 0 && strings.TrimSpace(probe.WorkspacePaths[0]) != "" {
			root = strings.TrimSpace(probe.WorkspacePaths[0])
		}
	}
	var st *storage.Store
	if fileExists(filepath.Join(root, ".sen", "state.db")) {
		st, _ = storage.Open(filepath.Join(root, ".sen", "state.db"))
		if st != nil {
			defer st.Close()
		}
	}
	return bridge.HandleAntigravityHookWithEvent(st, root, fallbackEvent, bytes.NewReader(rawBuf), stdout)
}

// cmdIntake ghi nhận nguyên văn yêu cầu của người dùng (R1..Rn, R_sec, R_law) hoặc quản lý tối đa 20 luật dự án.
func cmdIntake(cwd string, args []string, stdout io.Writer) error {
	root := resolveProjectRoot(cwd)
	st, err := openExistingStore(root)
	if err != nil {
		return err
	}
	defer st.Close()

	reordered := reorderFlagsFirst(args, map[string]bool{
		"quote": true, "req": true, "rule": true, "glob": true, "replace": true, "delete": true,
	})
	fs := flag.NewFlagSet("intake", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	quote := fs.String("quote", "", "verbatim user quote")
	req := fs.String("req", "", "requirement summary")
	bugfix := fs.Bool("bugfix", false, "mark bugfix requirement")
	sec := fs.Bool("security", false, "anchor R_sec security requirement")
	vnLaw := fs.Bool("vn-law", false, "anchor R_law legal compliance requirement")
	ruleText := fs.String("rule", "", "project rule text (<=20 max)")
	glob := fs.String("glob", "*", "rule path glob")
	replaceID := fs.Int64("replace", 0, "replace rule ID")
	deleteID := fs.Int64("delete", 0, "delete rule ID")
	if err := fs.Parse(reordered); err != nil {
		return err
	}

	// Tự động nạp câu nhắc nguyên văn từ .sen/last_prompt.txt nếu người gọi bỏ trống --quote.
	effectiveQuote := strings.TrimSpace(*quote)
	if effectiveQuote == "" {
		if b, err := os.ReadFile(filepath.Join(root, ".sen", "last_prompt.txt")); err == nil {
			effectiveQuote = strings.TrimSpace(string(b))
		}
	}

	didRule, err := applyRuleIntake(st, *deleteID, *replaceID, *ruleText, effectiveQuote, *glob, stdout)
	if err != nil {
		return err
	}
	didReq, err := applyReqIntake(st, root, *req, effectiveQuote, *bugfix, *sec, *vnLaw, stdout)
	if err != nil {
		return err
	}
	if !didRule && !didReq {
		return errors.New("specify --req/--security/--vn-law or --rule/--delete")
	}
	return nil
}

// applyRuleIntake xử lý thêm mới, thay thế (--replace) hoặc xóa (--delete) luật dự án trong giới hạn <= 20 luật.
func applyRuleIntake(st *storage.Store, deleteID, replaceID int64, ruleText, quote, glob string, stdout io.Writer) (bool, error) {
	did := false
	if deleteID > 0 {
		if err := st.DeleteRule(deleteID); err != nil {
			return false, err
		}
		fmt.Fprintf(stdout, "Deleted rule #%d\n", deleteID)
		did = true
	}
	if strings.TrimSpace(ruleText) != "" {
		if replaceID > 0 {
			if err := st.ReplaceRule(replaceID, ruleText, quote, glob); err != nil {
				return false, err
			}
			fmt.Fprintf(stdout, "Replaced rule #%d\n", replaceID)
		} else {
			r, err := st.AddRule(ruleText, quote, glob)
			if err != nil {
				return false, err
			}
			fmt.Fprintf(stdout, "Added rule #%d [%s]\n", r.ID, r.PathGlob)
		}
		did = true
	}
	return did, nil
}

// applyReqIntake lưu yêu cầu chức năng cùng các neo kiểm chứng bảo mật (R_sec) và pháp lý Việt Nam (R_law).
func applyReqIntake(st *storage.Store, root, req, quote string, bugfix, sec, vnLaw bool, stdout io.Writer) (bool, error) {
	if strings.TrimSpace(req) == "" && !sec && !vnLaw {
		return false, nil
	}
	if quote == "" {
		quote = strings.TrimSpace(req)
		if quote == "" {
			quote = "companion-compliance-anchor"
		}
	}
	if strings.TrimSpace(req) != "" {
		r, err := st.AddRequirement("", quote, req, bugfix)
		if err != nil {
			return false, err
		}
		fmt.Fprintf(stdout, "Recorded %s: %s\n", r.ID, r.Summary)
	}
	if sec {
		r, err := st.AddRequirement("R_sec", quote, "Security audit & secret/input boundary verification", false)
		if err != nil {
			return false, err
		}
		fmt.Fprintf(stdout, "Recorded %s: %s\n", r.ID, r.Summary)
	}
	if vnLaw {
		r, err := st.AddRequirement("R_law", quote, "Vietnam legal compliance (PDPL / Decree 356 / AI label) verification", false)
		if err != nil {
			return false, err
		}
		fmt.Fprintf(stdout, "Recorded %s: %s\n", r.ID, r.Summary)
	}
	tasks, _ := st.ListTasks()
	baseSnaps, _ := st.GetSnapshots(workspace.BaselineTaskID)
	if len(tasks) == 0 || len(baseSnaps) == 0 {
		if err := workspace.CaptureBaseline(st, root); err != nil {
			return false, err
		}
	}
	return true, nil
}

// cmdBegin mở một task mới với phạm vi tệp tường minh, khóa tệp test có sẵn, sao lưu snapshot và tính mức rủi ro tự động.
func cmdBegin(cwd string, args []string, stdout io.Writer) error {
	root := resolveProjectRoot(cwd)
	st, err := openExistingStore(root)
	if err != nil {
		return err
	}
	defer st.Close()

	reordered := reorderFlagsFirst(args, map[string]bool{
		"reqs": true, "files": true, "kind": true, "title": true,
	})
	fs := flag.NewFlagSet("begin", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	reqsCSV := fs.String("reqs", "", "comma-separated requirement IDs")
	filesCSV := fs.String("files", "", "comma-separated scope file paths")
	kindFlag := fs.String("kind", "", "feature|bugfix|ui")
	allowDelete := fs.Bool("allow-delete", false, "permit deleting declared existing scope files (HIGH risk)")
	titleFlag := fs.String("title", "", "task title")
	if err := fs.Parse(reordered); err != nil {
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
	reqIDs, hasBugfixReq, err := resolveBeginReqs(st, *reqsCSV)
	if err != nil {
		return err
	}
	normFiles, err := resolveBeginFiles(root, *filesCSV)
	if err != nil {
		return err
	}
	kind, err := workspace.ValidateKind(*kindFlag)
	if err != nil {
		return err
	}
	// Không cho phép hạ cấp yêu cầu sửa lỗi (--bugfix) xuống UI hoặc FEATURE để né kiểm thử.
	if hasBugfixReq {
		kind = "BUGFIX"
	} else if kind == "" {
		kind = "FEATURE"
	}

	if err := workspace.BeginTaskSnapshotWithOpts(st, root, taskID, normFiles, *allowDelete); err != nil {
		return err
	}
	risk, reasons := workspace.ComputeRiskWithOpts(normFiles, kind, *allowDelete, root)
	title := strings.TrimSpace(*titleFlag)
	if title == "" {
		title = fmt.Sprintf("%s (%s)", taskID, strings.Join(reqIDs, ","))
	}
	if err := st.CreateTask(storage.Task{
		ID: taskID, ReqIDs: reqIDs, Title: title, Kind: kind,
		RiskAuto: risk, ScopeFiles: normFiles, Status: "OPEN",
	}); err != nil {
		return err
	}

	fmt.Fprintf(stdout, "Opened task %s [kind=%s risk=%s reqs=%v files=%v]\n", taskID, kind, risk, reqIDs, normFiles)
	if len(reasons) > 0 {
		fmt.Fprintf(stdout, "Risk factors: %s\n", strings.Join(reasons, "; "))
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

// resolveBeginReqs đối chiếu danh sách mã yêu cầu R_i với sổ cái và phát hiện cờ sửa lỗi (IsBugfix).
func resolveBeginReqs(st *storage.Store, reqsCSV string) ([]string, bool, error) {
	reqIDs := splitCSV(reqsCSV)
	if len(reqIDs) == 0 {
		return nil, false, errors.New("missing --reqs (e.g. --reqs R1)")
	}
	existingReqs, err := st.ListRequirements()
	if err != nil {
		return nil, false, err
	}
	reqMap := make(map[string]storage.Requirement, len(existingReqs))
	for _, r := range existingReqs {
		reqMap[r.ID] = r
	}
	hasBugfix := false
	for _, id := range reqIDs {
		r, ok := reqMap[id]
		if !ok {
			return nil, false, fmt.Errorf("unknown requirement %q (record via `sen intake` first)", id)
		}
		if r.IsBugfix {
			hasBugfix = true
		}
	}
	return reqIDs, hasBugfix, nil
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
