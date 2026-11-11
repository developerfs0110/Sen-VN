// Tệp workflows.go khởi tạo dự án SSDS (UUIDv4, chuỗi pha thích ứng) và kết xuất bản tuyên bố kích hoạt Workflow Declaration (§3.4).
package ssds

import (
	"crypto/rand"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/developerfs0110/Sen-VN/internal/storage"
)

// NewProjectUUID sinh mã định danh dự án chuẩn RFC 4122 UUIDv4 bằng nguồn ngẫu nhiên mật mã crypto/rand.
func NewProjectUUID() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	buf[6] = (buf[6] & 0x0f) | 0x40
	buf[8] = (buf[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		buf[0:4], buf[4:6], buf[6:8], buf[8:10], buf[10:16]), nil
}

// resolveBootstrapParams chuẩn hóa tên dự án, phân loại, bậc quy trình và danh sách pha hoạt động khi khởi tạo dự án.
func resolveBootstrapParams(projectRoot, projectName string, rp *Rulepack, det DetectionResult) (string, string, string, []string) {
	name := strings.TrimSpace(projectName)
	if name == "" && strings.TrimSpace(projectRoot) != "" {
		name = filepath.Base(filepath.Clean(projectRoot))
	}
	category := strings.ToUpper(strings.TrimSpace(det.Category))
	if category == "" {
		if det.HasExistingCode {
			category = "BROWNFIELD"
		} else {
			category = "GREENFIELD"
		}
	}
	tier := strings.ToUpper(strings.TrimSpace(det.SuggestedWorkflow))
	if tier != "WF_MICRO" && tier != "WF_STANDARD" && tier != "WF_CRITICAL" {
		tier = "WF_STANDARD"
	}
	activeIDs := rp.Workflows[tier]
	if len(activeIDs) == 0 {
		if tier == "WF_MICRO" {
			activeIDs = []string{"P1", "P3", "P5", "P6", "P8"}
		} else {
			activeIDs = []string{"P0", "P1", "P2", "P3", "P4", "P5", "P6", "P7", "P8", "P9"}
		}
	}
	return name, category, tier, activeIDs
}

// seedInitialPhaseRows khởi tạo các dòng trạng thái pha ban đầu trong bảng ssds_phases theo Rulepack đang áp dụng.
func seedInitialPhaseRows(st *storage.Store, rp *Rulepack, tier string, activeIDs []string) error {
	for idx, pid := range activeIDs {
		spec, ok := rp.PhaseByID(pid)
		if !ok {
			continue
		}
		status := "NOT_STARTED"
		if idx == 0 {
			status = "IN_PROGRESS"
		}
		ph := storage.SSDSPhase{
			PhaseID:              spec.PhaseID,
			Status:               status,
			PrimaryRole:          spec.PrimaryRole,
			SupportingRoles:      spec.SupportingRoles,
			RequiredDeliverables: spec.EffectiveDeliverables(tier),
			VerifiedDeliverables: map[string]string{},
			RequiresUserApproval: spec.RequiresUserApproval,
			GatePassed:           false,
		}
		if err := st.UpsertSSDSPhase(ph); err != nil {
			return err
		}
	}
	return nil
}

// BootstrapProject khởi tạo bản ghi ssds_project và các dòng pha tương ứng trong ssds_phases cho dự án phần mềm.
func BootstrapProject(
	st *storage.Store,
	projectRoot, projectName, ssdsVersion string,
	det DetectionResult,
) (*storage.SSDSProject, []storage.SSDSPhase, error) {
	if existing, err := st.GetSSDSProject(); err != nil {
		return nil, nil, err
	} else if existing != nil {
		phases, pErr := st.ListSSDSPhases()
		return existing, phases, pErr
	}
	ver := cleanVersion(ssdsVersion)
	if ver == "" {
		ver = BundledLatestVersion
	}
	rp, err := LoadRulepack(ver)
	if err != nil {
		return nil, nil, err
	}
	name, category, tier, activeIDs := resolveBootstrapParams(projectRoot, projectName, rp, det)
	projID, err := NewProjectUUID()
	if err != nil {
		return nil, nil, err
	}
	risks := det.RiskDimensions
	if len(risks) == 0 {
		risks = defaultLowRiskMap()
	}
	audited := false
	if strings.TrimSpace(projectRoot) != "" {
		if _, sErr := os.Stat(filepath.Join(projectRoot, ".sen", "brownfield_audit.json")); sErr == nil {
			audited = true
		}
	}
	proj := storage.SSDSProject{
		ProjectID: projID, ProjectName: name, AppliedSSDSVersion: rp.Version,
		Category: category, WorkflowTier: tier, ActivePhases: activeIDs,
		RiskAssessment: risks, CurrentPhase: activeIDs[0], BrownfieldAudited: audited,
	}
	if err := st.SaveSSDSProject(proj); err != nil {
		return nil, nil, err
	}
	if err := seedInitialPhaseRows(st, rp, tier, activeIDs); err != nil {
		return nil, nil, err
	}
	savedProj, err := st.GetSSDSProject()
	if err != nil {
		return nil, nil, err
	}
	savedPhases, err := st.ListSSDSPhases()
	return savedProj, savedPhases, err
}

// FormatActivationDeclaration kết xuất thông báo kích hoạt tự động và bản tuyên bố Workflow Declaration chuẩn SSDS §3.4.
func FormatActivationDeclaration(proj *storage.SSDSProject, phases []storage.SSDSPhase, latestBundled string) string {
	if proj == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "[SSDS v%s — Đã kích hoạt tự động | %s]\n", proj.AppliedSSDSVersion, StandardID)
	if latestBundled != "" && CompareSemver(latestBundled, proj.AppliedSSDSVersion) > 0 {
		fmt.Fprintf(&b, "Thông báo phiên bản SSDS: Dự án đang ghim v%s | Bản mới nhất trong Sen: v%s (chạy `sen sdlc version --check %s` để kiểm tra nâng cấp an toàn).\n",
			proj.AppliedSSDSVersion, cleanVersion(latestBundled), cleanVersion(latestBundled))
	}

	fmt.Fprintf(&b, "- Project ID / Name: %s (%s)\n", proj.ProjectID, proj.ProjectName)
	fmt.Fprintf(&b, "- Classification: %s | Workflow Tier: %s | Current Phase: %s\n", proj.Category, proj.WorkflowTier, proj.CurrentPhase)
	fmt.Fprintf(&b, "- Applicable SDLC Phases: %s\n", strings.Join(proj.ActivePhases, " -> "))

	var roleLines, delivSet, approvalPhases []string
	seenDeliv := make(map[string]bool)
	for _, ph := range phases {
		sup := ""
		if len(ph.SupportingRoles) > 0 {
			sup = " (Supporting: " + strings.Join(ph.SupportingRoles, ", ") + ")"
		}
		roleLines = append(roleLines, fmt.Sprintf("  * %s [%s]: %s%s", ph.PhaseID, ph.Status, ph.PrimaryRole, sup))
		for _, d := range ph.RequiredDeliverables {
			if !seenDeliv[d] {
				seenDeliv[d] = true
				delivSet = append(delivSet, d)
			}
		}
		if ph.RequiresUserApproval {
			approvalPhases = append(approvalPhases, ph.PhaseID)
		}
	}
	if len(roleLines) > 0 {
		b.WriteString("- Primary & Supporting Roles:\n" + strings.Join(roleLines, "\n") + "\n")
	}
	if len(delivSet) > 0 {
		fmt.Fprintf(&b, "- Expected Deliverables: %s\n", strings.Join(delivSet, ", "))
	}
	fmt.Fprintf(&b, "- Quality Gates: Phase-scoped write firewall, non-placeholder Markdown deliverable validation, 6-link traceability (§16), honest execution evidence (§20).\n")
	if len(approvalPhases) > 0 {
		fmt.Fprintf(&b, "- User Approval Checkpoints (REVIEW_REQUIRED != APPROVED): %s\n", strings.Join(approvalPhases, ", "))
	}

	if len(proj.RiskAssessment) > 0 {
		keys := make([]string, 0, len(proj.RiskAssessment))
		for k := range proj.RiskAssessment {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var rParts []string
		for _, k := range keys {
			rParts = append(rParts, fmt.Sprintf("%s=%s", k, proj.RiskAssessment[k]))
		}
		fmt.Fprintf(&b, "- Initial Risks & Constraints (§3.2): %s\n", strings.Join(rParts, ", "))
	}
	return b.String()
}

