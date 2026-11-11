// Tệp commands.go cài đặt các lệnh khởi tạo dự án, tóm tắt ngữ cảnh đầu phiên và tiếp nhận yêu cầu (init, brief, intake).
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/developerfs0110/Sen-VN/internal/bridge"
	"github.com/developerfs0110/Sen-VN/internal/ssds"
	"github.com/developerfs0110/Sen-VN/internal/storage"
	"github.com/developerfs0110/Sen-VN/internal/verify"
	"github.com/developerfs0110/Sen-VN/internal/workspace"
)

// cmdInit khởi tạo sổ cái .sen/state.db, cấu hình v1.2 (bảo toàn ssds_version đã ghim), cài đặt gói đồng hành và chụp __BASELINE__.
func cmdInit(cwd string, args []string, stdout io.Writer) error {
	reordered := reorderFlagsFirst(args, map[string]bool{"packs": true, "profile": true})
	fs := flag.NewFlagSet("init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	withCompanions := fs.Bool("with-companions", false, "report companion discovery")
	installCompanions := fs.Bool("install-companions", false, "install default companion skills")
	packsFlag := fs.String("packs", "", "list | all | comma-separated pack names")
	profileFlag := fs.String("profile", "auto", "auto|code|web|video|media|docs")
	expressFlag := fs.Bool("express", false, "enable autonomous express mode")
	updateFlag := fs.Bool("update", false, "check Sen-VN updates and sync installed packages")
	agyHooks := fs.Bool("agy-hooks", false, "write .agents/hooks.json")
	overwriteAgents := fs.Bool("overwrite-agents", false, "overwrite AGENTS.md")
	if err := fs.Parse(reordered); err != nil {
		return err
	}

	if strings.EqualFold(strings.TrimSpace(*packsFlag), "list") {
		fmt.Fprint(stdout, bridge.FormatPacksCatalog())
		return nil
	}
	root := resolveProjectRoot(cwd)
	if *updateFlag && !*installCompanions && strings.TrimSpace(*packsFlag) == "" {
		return bridge.RunWorkspaceUpdate(context.Background(), root, stdout)
	}

	cfg, err := ensureInitWorkspace(root, fs, *profileFlag, *expressFlag, *overwriteAgents)
	if err != nil {
		return err
	}
	if *installCompanions || strings.TrimSpace(*packsFlag) != "" {
		if _, err := bridge.InstallCompanionsAndPacks(
			context.Background(), root, *installCompanions, *packsFlag, stdout,
		); err != nil {
			return err
		}
	}
	if *updateFlag {
		if err := bridge.RunWorkspaceUpdate(context.Background(), root, stdout); err != nil {
			return err
		}
	}

	comp := bridge.DetectCompanions(root)
	if *agyHooks || comp.AntigravityEnv {
		if err := bridge.WriteAntigravityHooksFile(filepath.Join(root, ".agents", "hooks.json")); err != nil {
			return err
		}
	}
	resolvedProf := workspace.ResolveDomainProfile(root, cfg.DomainProfile)
	fmt.Fprintf(stdout, "Initialized Sen-VN v%s in %s [profile=%s ssds=%s express=%t]\n",
		workspace.CurrentVersion, root, resolvedProf, cfg.SSDSVersion, cfg.ExpressMode)
	if *withCompanions || *installCompanions || comp.AntigravityEnv {
		fmt.Fprintf(stdout, "Companions: %s\n", comp.FormatCompact())
	}
	return nil
}

// ensureInitWorkspace tạo thư mục .sen, mở SQLite state.db, lưu cấu hình có chữ ký, ghi AGENTS.md và chụp __BASELINE__ một lần duy nhất.
func ensureInitWorkspace(cwd string, fs *flag.FlagSet, profileVal string, expressVal, overwriteAgents bool) (Config, error) {
	prof, err := workspace.ValidateDomainProfile(profileVal)
	if err != nil {
		return Config{}, err
	}
	if err := os.MkdirAll(filepath.Join(cwd, ".sen", "snapshots"), 0o755); err != nil {
		return Config{}, err
	}
	st, err := storage.Open(filepath.Join(cwd, ".sen", "state.db"))
	if err != nil {
		return Config{}, err
	}
	defer st.Close()

	cfg, err := upsertInitConfig(cwd, st, prof, flagVisited(fs, "profile"), expressVal, flagVisited(fs, "express"))
	if err != nil {
		return Config{}, err
	}
	agentsPath := filepath.Join(cwd, "AGENTS.md")
	wroteAgents := false
	if overwriteAgents || !fileExists(agentsPath) {
		if err := os.WriteFile(agentsPath, []byte(bridge.RenderAgentsMD()), 0o644); err != nil {
			return Config{}, err
		}
		wroteAgents = true
	}
	tasks, _ := st.ListTasks()
	hasOpen := false
	for _, t := range tasks {
		if t.Status == "OPEN" {
			hasOpen = true
			break
		}
	}
	existingBase, _ := st.GetSnapshots(workspace.BaselineTaskID)
	if !hasOpen && len(existingBase) == 0 {
		if err := workspace.CaptureBaseline(st, cwd); err != nil {
			return Config{}, err
		}
	} else if !hasOpen && len(existingBase) > 0 && wroteAgents {
		_ = workspace.AdvanceBaseline(st, cwd, []string{"AGENTS.md"})
	}
	return cfg, nil
}

// upsertInitConfig khởi tạo mới hoặc cập nhật .sen/config.json và tuyệt đối bảo toàn ssds_version / project_id đã ghim (Phần II).
func upsertInitConfig(cwd string, st *storage.Store, prof string, profVisited, expressVal, expressVisited bool) (Config, error) {
	cfgPath := filepath.Join(cwd, ".sen", "config.json")
	sumPath := filepath.Join(cwd, ".sen", "config.sha256")
	if fileExists(cfgPath) || fileExists(sumPath) {
		cfg, err := loadConfig(cwd)
		if err != nil {
			return Config{}, err
		}
		if profVisited {
			cfg.DomainProfile = prof
			cfg.ProjectVerifyCmd = detectDefaultVerifyCmd(cwd, workspace.ResolveDomainProfile(cwd, prof))
		}
		if expressVisited {
			cfg.ExpressMode = expressVal
		}
		if proj, _ := st.GetSSDSProject(); proj != nil {
			if strings.TrimSpace(cfg.SSDSVersion) == "" {
				cfg.SSDSVersion = proj.AppliedSSDSVersion
			}
			if strings.TrimSpace(cfg.ProjectID) == "" {
				cfg.ProjectID = proj.ProjectID
			}
		}
		if strings.TrimSpace(cfg.SSDSVersion) == "" {
			cfg.SSDSVersion = ssds.BundledLatestVersion
		}
		cfg.Version = "1.2"
		return cfg, saveConfig(cwd, cfg)
	}
	resolved := workspace.ResolveDomainProfile(cwd, prof)
	pinnedVer := ssds.BundledLatestVersion
	projID := ""
	if proj, _ := st.GetSSDSProject(); proj != nil {
		if strings.TrimSpace(proj.AppliedSSDSVersion) != "" {
			pinnedVer = proj.AppliedSSDSVersion
		}
		projID = proj.ProjectID
	}
	cfg := Config{
		Version:          "1.2",
		SSDSVersion:      pinnedVer,
		ProjectID:        projID,
		AllowedBinaries:  verify.DefaultAllowedBinaries(),
		ProjectVerifyCmd: detectDefaultVerifyCmd(cwd, resolved),
		TimeoutSeconds:   120,
		ExpressMode:      expressVal,
		DomainProfile:    prof,
	}
	return cfg, saveConfig(cwd, cfg)
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

// runHookBrief chuyển tiếp dữ liệu JSON stdin sang bộ định tuyến đa dự án HandleAntigravityHookWithEvent.
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
	effRoot, hasDB := workspace.ResolveNearestProjectRoot("", root)
	var st *storage.Store
	if hasDB {
		st, _ = storage.Open(filepath.Join(effRoot, ".sen", "state.db"))
		if st != nil {
			defer st.Close()
		}
	}
	return bridge.HandleAntigravityHookWithEvent(st, effRoot, fallbackEvent, bytes.NewReader(rawBuf), stdout)
}

// cmdIntake ghi nhận nguyên văn yêu cầu của người dùng (R1..Rn, R_sec, R_law), bật/tắt --express hoặc quản lý <= 20 luật dự án.
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
	express := fs.Bool("express", false, "toggle autonomous express mode")
	ruleText := fs.String("rule", "", "project rule text (<=20 max)")
	glob := fs.String("glob", "*", "rule path glob")
	replaceID := fs.Int64("replace", 0, "replace rule ID")
	deleteID := fs.Int64("delete", 0, "delete rule ID")
	if err := fs.Parse(reordered); err != nil {
		return err
	}
	if err := applyExpressFlagIfVisited(root, fs, *express); err != nil {
		return err
	}

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
	if !didRule && !didReq && !flagVisited(fs, "express") {
		return errors.New("specify --req/--security/--vn-law, --rule/--delete, or --express")
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
	baseSnaps, _ := st.GetSnapshots(workspace.BaselineTaskID)
	if len(baseSnaps) == 0 {
		if err := workspace.CaptureBaseline(st, root); err != nil {
			return false, err
		}
	}
	return true, nil
}
