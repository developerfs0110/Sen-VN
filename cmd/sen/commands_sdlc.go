// Tệp commands_sdlc.go cài đặt nhóm lệnh điều phối vòng đời SSDS cốt lõi: classify, status, gate và sync.
package main

import (
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
	"github.com/developerfs0110/Sen-VN/internal/workspace"
)

// validSSDSCategories liệt kê 7 nhóm phân loại dự án chuẩn theo điều khoản SSDS §3.2.
var validSSDSCategories = map[string]bool{
	"GREENFIELD":  true,
	"BROWNFIELD":  true,
	"MAINTENANCE": true,
	"MIGRATION":   true,
	"INTEGRATION": true,
	"PROTOTYPE":   true,
	"RESEARCH":    true,
}

// validSSDSWorkflows liệt kê 3 bậc quy trình thích ứng của SSDS trong Sen-VN.
var validSSDSWorkflows = map[string]bool{
	"WF_MICRO":    true,
	"WF_STANDARD": true,
	"WF_CRITICAL": true,
}

// cmdSDLC định tuyến 9 lệnh con quản trị vòng đời phần mềm chuẩn SEN-STD-SDLC-001 (SSDS).
func cmdSDLC(cwd string, args []string, stdout io.Writer) error {
	if len(args) == 0 {
		return errors.New("usage: sen sdlc <classify|status|gate|review|approve|impact|exception|sync|version> [flags]")
	}
	sub := strings.ToLower(strings.TrimSpace(args[0]))
	rest := args[1:]
	switch sub {
	case "classify":
		return sdlcClassify(cwd, rest, stdout)
	case "status":
		return sdlcStatus(cwd, rest, stdout)
	case "gate":
		return sdlcGate(cwd, rest, stdout)
	case "review":
		return sdlcReview(cwd, rest, stdout)
	case "approve":
		return sdlcApprove(cwd, rest, stdout)
	case "impact":
		return sdlcImpact(cwd, rest, stdout)
	case "exception":
		return sdlcException(cwd, rest, stdout)
	case "sync":
		return sdlcSync(cwd, rest, stdout)
	case "version":
		return sdlcVersion(cwd, rest, stdout)
	default:
		return fmt.Errorf("unknown `sen sdlc` subcommand %q (expected classify|status|gate|review|approve|impact|exception|sync|version)", sub)
	}
}

// sdlcClassify phân loại tự động dự án, khảo sát Brownfield (nếu có), khởi tạo ssds_project/ssds_phases và xuất Workflow Declaration.
func sdlcClassify(cwd string, args []string, stdout io.Writer) error {
	reordered := reorderFlagsFirst(args, map[string]bool{
		"prompt": true, "name": true, "category": true, "workflow": true, "ssds-version": true,
	})
	fs := flag.NewFlagSet("sdlc classify", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	promptFlag := fs.String("prompt", "", "user software request prompt")
	nameFlag := fs.String("name", "", "project name")
	catFlag := fs.String("category", "", "explicit SSDS category override")
	wfFlag := fs.String("workflow", "", "explicit workflow tier override (WF_MICRO|WF_STANDARD|WF_CRITICAL)")
	verFlag := fs.String("ssds-version", "", "SSDS rulepack version to pin (default bundled latest)")
	if err := fs.Parse(reordered); err != nil {
		return err
	}

	root := resolveProjectRoot(cwd)
	st, cfg, err := openOrBootstrapSDLCStore(root)
	if err != nil {
		return err
	}
	defer st.Close()

	promptText := strings.TrimSpace(*promptFlag)
	if promptText == "" {
		if b, rErr := os.ReadFile(filepath.Join(root, ".sen", "last_prompt.txt")); rErr == nil {
			promptText = strings.TrimSpace(string(b))
		}
	}

	det, err := buildClassificationResult(root, promptText, *catFlag, *wfFlag)
	if err != nil {
		return err
	}
	if det.HasExistingCode || det.Category == "BROWNFIELD" || det.Category == "MAINTENANCE" || det.Category == "MIGRATION" || det.Category == "INTEGRATION" {
		if _, err := ssds.AuditBrownfieldProject(st, root); err != nil {
			return err
		}
	}

	targetVer := strings.TrimSpace(*verFlag)
	if targetVer == "" {
		if existingProj, _ := st.GetSSDSProject(); existingProj != nil && strings.TrimSpace(existingProj.AppliedSSDSVersion) != "" {
			targetVer = existingProj.AppliedSSDSVersion
		} else {
			targetVer = ssds.BundledLatestVersion
		}
	}
	projName := strings.TrimSpace(*nameFlag)
	if projName == "" {
		projName = filepath.Base(root)
	}

	proj, phases, err := ssds.BootstrapProject(st, root, projName, targetVer, det)
	if err != nil {
		return err
	}
	cfg.Version = "1.2"
	cfg.SSDSVersion = proj.AppliedSSDSVersion
	cfg.ProjectID = proj.ProjectID
	if err := saveConfig(root, cfg); err != nil {
		return err
	}
	if err := ssds.SyncProjectStateFiles(st, root); err != nil {
		return err
	}
	fmt.Fprint(stdout, ssds.FormatActivationDeclaration(proj, phases, ssds.LatestAvailableVersion()))
	return nil
}

// openOrBootstrapSDLCStore đảm bảo .sen/state.db, .sen/config.json và ảnh chụp __BASELINE__ (ghi một lần) tồn tại trước khi phân loại SSDS.
func openOrBootstrapSDLCStore(root string) (*storage.Store, Config, error) {
	if err := os.MkdirAll(filepath.Join(root, ".sen", "snapshots"), 0o755); err != nil {
		return nil, Config{}, err
	}
	st, err := storage.Open(filepath.Join(root, ".sen", "state.db"))
	if err != nil {
		return nil, Config{}, err
	}
	cfg, err := upsertInitConfig(root, st, "auto", false, false, false)
	if err != nil {
		_ = st.Close()
		return nil, Config{}, err
	}
	agentsPath := filepath.Join(root, "AGENTS.md")
	wroteAgents := false
	if !fileExists(agentsPath) {
		_ = os.WriteFile(agentsPath, []byte(bridge.RenderAgentsMD()), 0o644)
		wroteAgents = true
	}
	if baseSnaps, sErr := st.GetSnapshots(workspace.BaselineTaskID); sErr == nil && len(baseSnaps) == 0 {
		_ = workspace.CaptureBaseline(st, root)
	} else if sErr == nil && len(baseSnaps) > 0 && wroteAgents {
		_ = workspace.AdvanceBaseline(st, root, []string{"AGENTS.md"})
	}
	return st, cfg, nil
}

// buildClassificationResult chạy bộ dò tự động và áp dụng ghi đè tường minh cho Category hoặc SuggestedWorkflow nếu người dùng truyền cờ.
func buildClassificationResult(root, promptText, catOverride, wfOverride string) (ssds.DetectionResult, error) {
	det := ssds.DetectSoftwareIntent(root, promptText)
	if !det.IsSoftwareRequest {
		det.IsSoftwareRequest = true
		if det.Category == "" {
			det.Category = "GREENFIELD"
		}
		if det.SuggestedWorkflow == "" {
			det.SuggestedWorkflow = "WF_STANDARD"
		}
	}
	if strings.TrimSpace(catOverride) != "" {
		c := strings.ToUpper(strings.TrimSpace(catOverride))
		if !validSSDSCategories[c] {
			return det, fmt.Errorf("invalid --category %q (expected GREENFIELD|BROWNFIELD|MAINTENANCE|MIGRATION|INTEGRATION|PROTOTYPE|RESEARCH)", catOverride)
		}
		det.Category = c
	}
	if strings.TrimSpace(wfOverride) != "" {
		w := strings.ToUpper(strings.TrimSpace(wfOverride))
		if !validSSDSWorkflows[w] {
			return det, fmt.Errorf("invalid --workflow %q (expected WF_MICRO|WF_STANDARD|WF_CRITICAL)", wfOverride)
		}
		det.SuggestedWorkflow = w
	}
	return det, nil
}

// sdlcStatus hiển thị phiên bản SSDS đã ghim, loại dự án, pha hiện hành, vai trò chuyên môn, bảng pha và báo cáo §25 tùy chọn.
func sdlcStatus(cwd string, args []string, stdout io.Writer) error {
	fs := flag.NewFlagSet("sdlc status", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	reportFlag := fs.Bool("report", false, "append full SSDS §25 Standard Progress Report")
	if err := fs.Parse(args); err != nil {
		return err
	}

	root := resolveProjectRoot(cwd)
	st, err := openExistingStore(root)
	if err != nil {
		return err
	}
	defer st.Close()

	proj, err := st.GetSSDSProject()
	if err != nil {
		return err
	}
	if proj == nil {
		return errors.New("SSDS lifecycle is not initialized for this project; run `sen sdlc classify` first")
	}
	phases, _ := st.ListSSDSPhases()
	govs, _ := st.ListGovernanceRecords("", "")
	latestVer := ssds.LatestAvailableVersion()

	fmt.Fprintf(stdout, "=== SSDS LIFECYCLE STATUS (%s) ===\n", proj.ProjectName)
	fmt.Fprintf(stdout, "Project ID: %s\n", proj.ProjectID)
	fmt.Fprintf(stdout, "SSDS Version: pinned v%s (bundled/available latest: v%s)\n", proj.AppliedSSDSVersion, latestVer)
	if ssds.CompareSemver(latestVer, proj.AppliedSSDSVersion) > 0 {
		fmt.Fprintf(stdout, "Notice: Newer SSDS v%s is available (this project remains safely pinned to v%s; run `sen sdlc version --check %s`).\n",
			latestVer, proj.AppliedSSDSVersion, latestVer)
	}
	fmt.Fprintf(stdout, "Category: %s | Workflow: %s | Brownfield Audited: %v | Governance Records: %d\n",
		proj.Category, proj.WorkflowTier, proj.BrownfieldAudited, len(govs))

	for _, ph := range phases {
		if ph.PhaseID == proj.CurrentPhase {
			fmt.Fprintf(stdout, "Current Phase: %s [%s] | Primary Role: %s | Supporting: %s\n\n",
				ph.PhaseID, ph.Status, ph.PrimaryRole, strings.Join(ph.SupportingRoles, ", "))
			break
		}
	}
	fmt.Fprintln(stdout, "Phases:")
	for _, ph := range phases {
		fmt.Fprintf(stdout, "  %-4s | %-15s | gate=%-5v | approval_req=%-5v | role=%s\n",
			ph.PhaseID, ph.Status, ph.GatePassed, ph.RequiresUserApproval, ph.PrimaryRole)
	}
	if *reportFlag {
		rep, rErr := ssds.RenderStandardProgressReport(st, root)
		if rErr != nil {
			return rErr
		}
		fmt.Fprintln(stdout, "\n"+rep)
	}
	return nil
}

// sdlcGate thực thi cổng kiểm định chất lượng của một pha SDLC, đồng bộ tệp trạng thái và thông báo rõ nếu chờ User phê duyệt.
func sdlcGate(cwd string, args []string, stdout io.Writer) error {
	reordered := reorderFlagsFirst(args, map[string]bool{"phase": true})
	fs := flag.NewFlagSet("sdlc gate", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	phaseFlag := fs.String("phase", "", "phase ID (defaults to current phase)")
	if err := fs.Parse(reordered); err != nil {
		return err
	}

	root := resolveProjectRoot(cwd)
	st, err := openExistingStore(root)
	if err != nil {
		return err
	}
	defer st.Close()

	proj, err := st.GetSSDSProject()
	if err != nil {
		return err
	}
	if proj == nil {
		return errors.New("SSDS project is not initialized; run `sen sdlc classify` first")
	}
	phaseID := strings.ToUpper(strings.TrimSpace(*phaseFlag))
	if phaseID == "" && len(fs.Args()) > 0 {
		phaseID = strings.ToUpper(strings.TrimSpace(fs.Args()[0]))
	}
	if phaseID == "" {
		phaseID = proj.CurrentPhase
	}

	gate, ph, err := ssds.RunPhaseGate(st, root, phaseID)
	if err != nil {
		return err
	}
	_ = ssds.SyncProjectStateFiles(st, root)

	fmt.Fprintf(stdout, "[SSDS Gate %s | v%s] Verdict: %s\n", gate.PhaseID, gate.AppliedVersion, gate.Verdict)
	if gate.SyncedReqs > 0 {
		fmt.Fprintf(stdout, "  Synced Requirements: %d\n", gate.SyncedReqs)
	}
	for rel, sum := range gate.DeliverableHashes {
		fmt.Fprintf(stdout, "  Verified Deliverable: %s (sha256:%s)\n", rel, sum[:12])
	}
	for _, w := range gate.Warnings {
		fmt.Fprintf(stdout, "  WARNING: %s\n", w)
	}
	if !gate.Passed {
		for _, v := range gate.Violations {
			fmt.Fprintf(stdout, "  VIOLATION: %s\n", v)
		}
		return fmt.Errorf("SSDS gate %s FAILED (%d violations): %s",
			gate.PhaseID, len(gate.Violations), strings.Join(gate.Violations, "; "))
	}

	updatedProj, _ := st.GetSSDSProject()
	if ph != nil && ph.Status == "REVIEW_REQUIRED" {
		fmt.Fprintf(stdout, "\n[USER NOTIFICATION — HUMAN APPROVAL REQUIRED (SSDS §21/§22)]\n")
		fmt.Fprintf(stdout, "Phase %s (%s) passed automated checks and is now in REVIEW_REQUIRED (REVIEW_REQUIRED != APPROVED).\n",
			ph.PhaseID, ph.PrimaryRole)
		fmt.Fprintf(stdout, "Downstream phase transitions remain locked until explicit user approval:\n")
		fmt.Fprintf(stdout, "  sen sdlc approve --phase %s --quote \"<exact user approval>\"\n", ph.PhaseID)
	} else if updatedProj != nil {
		fmt.Fprintf(stdout, "Next Active Phase: %s\n", updatedProj.CurrentPhase)
	}
	return nil
}

// sdlcSync kết xuất lại toàn bộ tệp trạng thái trên repo từ SQLite và in Báo cáo Tiến độ chuẩn 8 mục (§24–§25).
func sdlcSync(cwd string, args []string, stdout io.Writer) error {
	_ = args
	root := resolveProjectRoot(cwd)
	st, err := openExistingStore(root)
	if err != nil {
		return err
	}
	defer st.Close()

	if err := ssds.SyncProjectStateFiles(st, root); err != nil {
		return err
	}
	rep, err := ssds.RenderStandardProgressReport(st, root)
	if err != nil {
		return err
	}
	fmt.Fprint(stdout, rep)
	return nil
}
