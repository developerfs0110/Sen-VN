// Tệp registry_upgrade.go thực thi kiểm toán tương thích chỉ-đọc, nâng cấp có kiểm soát và hoàn tác phiên bản chuẩn SSDS.
package ssds

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/developerfs0110/Sen-VN/internal/storage"
)

// UpgradeCheckReport báo cáo kết quả chạy thử (Dry-run Read-Only) khi đối chiếu dự án với phiên bản SSDS mới.
type UpgradeCheckReport struct {
	CurrentVersion      string   `json:"current_version"`
	TargetVersion       string   `json:"target_version"`
	Compatible          bool     `json:"compatible"`
	BreakingDiffs       []string `json:"breaking_diffs"`
	MissingDeliverables []string `json:"missing_deliverables"`
	MissingSections     []string `json:"missing_sections"`
}

// upgradeBackup lưu ảnh chụp phiên bản, danh sách pha hoạt động và trạng thái pha trước khi nâng cấp để hoàn tác tức thời.
type upgradeBackup struct {
	PreviousVersion      string              `json:"previous_version"`
	PreviousActivePhases []string            `json:"previous_active_phases"`
	TargetVersion        string              `json:"target_version"`
	ApprovalQuote        string              `json:"approval_quote"`
	PhasesSnapshot       []storage.SSDSPhase `json:"phases_snapshot"`
	SavedAt              string              `json:"saved_at"`
}

// CheckUpgradeCompatibility thực hiện kiểm toán chỉ-đọc (0 ghi đè) so sánh dự án hiện tại với phiên bản SSDS mục tiêu.
func CheckUpgradeCompatibility(st *storage.Store, root, targetVersion string) (*UpgradeCheckReport, error) {
	proj, err := st.GetSSDSProject()
	if err != nil {
		return nil, err
	}
	if proj == nil {
		return nil, errors.New("SSDS project is not initialized")
	}
	curRP, err := LoadRulepack(proj.AppliedSSDSVersion)
	if err != nil {
		return nil, err
	}
	targetVer := cleanVersion(targetVersion)
	tgtRP, err := LoadRulepack(targetVer)
	if err != nil {
		return nil, err
	}

	rep := &UpgradeCheckReport{CurrentVersion: proj.AppliedSSDSVersion, TargetVersion: targetVer}
	if parseSemverTriplet(targetVer)[0] > parseSemverTriplet(proj.AppliedSSDSVersion)[0] {
		rep.BreakingDiffs = append(rep.BreakingDiffs, fmt.Sprintf("MAJOR version jump (%s -> %s)", proj.AppliedSSDSVersion, targetVer))
	}

	phases, err := st.ListSSDSPhases()
	if err != nil {
		return nil, err
	}
	phaseMap := make(map[string]storage.SSDSPhase, len(phases))
	for _, ph := range phases {
		phaseMap[ph.PhaseID] = ph
	}

	targetPhases := tgtRP.Workflows[proj.WorkflowTier]
	if len(targetPhases) == 0 {
		targetPhases = proj.ActivePhases
	}
	for _, pid := range targetPhases {
		tSpec, ok := tgtRP.PhaseByID(pid)
		if !ok {
			continue
		}
		cSpec, existed := curRP.PhaseByID(pid)
		existingRow, inProj := phaseMap[pid]
		if !inProj {
			rep.BreakingDiffs = append(rep.BreakingDiffs, fmt.Sprintf("workflow %s adds new required phase %s", proj.WorkflowTier, pid))
		} else if !existed || (!cSpec.RequiresUserApproval && tSpec.RequiresUserApproval && existingRow.Status == "COMPLETED" && existingRow.ApprovedQuote == "") {
			rep.BreakingDiffs = append(rep.BreakingDiffs, fmt.Sprintf("phase %s newly requires explicit user approval in v%s", pid, targetVer))
		}

		// Đối chiếu tài liệu thực tế trên đĩa cho các pha đang chạy hoặc đã vượt qua.
		if inProj && existingRow.Status != "NOT_STARTED" && strings.TrimSpace(root) != "" {
			delivs := tSpec.EffectiveDeliverables(proj.WorkflowTier)
			headings := tSpec.EffectiveHeadings(proj.WorkflowTier)
			for _, rel := range delivs {
				full := filepath.Join(root, filepath.FromSlash(rel))
				rawBytes, rErr := os.ReadFile(full)
				if rErr != nil {
					rep.MissingDeliverables = append(rep.MissingDeliverables, fmt.Sprintf("%s: missing %s", pid, rel))
					continue
				}
				for _, h := range headings[rel] {
					if okH, _ := markdownHasPopulatedSection(string(rawBytes), h); !okH {
						rep.MissingSections = append(rep.MissingSections, fmt.Sprintf("%s (%s): missing or placeholder section %q", pid, rel, h))
					}
				}
			}
		}
	}
	rep.Compatible = len(rep.BreakingDiffs) == 0 && len(rep.MissingDeliverables) == 0 && len(rep.MissingSections) == 0
	return rep, nil
}

// PerformControlledUpgrade nâng cấp phiên bản SSDS, đồng bộ ActivePhases và sao lưu .sen/ssds_upgrade_backup.json khi có phê duyệt CRITICAL.
func PerformControlledUpgrade(st *storage.Store, root, targetVersion, approvalQuote string) error {
	quote := strings.TrimSpace(approvalQuote)
	if quote == "" {
		return errors.New("SSDS §21 violation: upgrading SSDS version is CRITICAL and requires a non-empty approvalQuote")
	}
	proj, err := st.GetSSDSProject()
	if err != nil {
		return err
	}
	if proj == nil {
		return errors.New("SSDS project is not initialized")
	}
	targetVer := cleanVersion(targetVersion)
	tgtRP, err := LoadRulepack(targetVer)
	if err != nil {
		return err
	}
	phases, err := st.ListSSDSPhases()
	if err != nil {
		return err
	}

	prevVer := proj.AppliedSSDSVersion
	bkp := upgradeBackup{
		PreviousVersion:      prevVer,
		PreviousActivePhases: append([]string(nil), proj.ActivePhases...),
		TargetVersion:        targetVer,
		ApprovalQuote:        quote,
		PhasesSnapshot:       phases,
		SavedAt:              time.Now().UTC().Format(time.RFC3339),
	}
	if strings.TrimSpace(root) != "" {
		bkpPath := filepath.Join(root, filepath.FromSlash(upgradeBackupRelPath))
		if err := os.MkdirAll(filepath.Dir(bkpPath), 0o755); err != nil {
			return err
		}
		rawBkp, _ := json.MarshalIndent(bkp, "", "  ")
		if err := os.WriteFile(bkpPath, rawBkp, 0o644); err != nil {
			return err
		}
	}

	existingByID := make(map[string]storage.SSDSPhase, len(phases))
	for _, ph := range phases {
		existingByID[ph.PhaseID] = ph
		if ph.Status == "COMPLETED" || ph.Status == "APPROVED" {
			continue
		}
		if spec, ok := tgtRP.PhaseByID(ph.PhaseID); ok {
			ph.PrimaryRole = spec.PrimaryRole
			ph.SupportingRoles = spec.SupportingRoles
			ph.RequiredDeliverables = spec.EffectiveDeliverables(proj.WorkflowTier)
			ph.RequiresUserApproval = spec.RequiresUserApproval
			if err := st.UpsertSSDSPhase(ph); err != nil {
				return err
			}
		}
	}

	if nextActive := tgtRP.Workflows[proj.WorkflowTier]; len(nextActive) > 0 {
		for _, pid := range nextActive {
			if _, exists := existingByID[pid]; !exists {
				if spec, ok := tgtRP.PhaseByID(pid); ok {
					newPh := storage.SSDSPhase{
						PhaseID:              pid,
						Status:               "NOT_STARTED",
						PrimaryRole:          spec.PrimaryRole,
						SupportingRoles:      spec.SupportingRoles,
						RequiredDeliverables: spec.EffectiveDeliverables(proj.WorkflowTier),
						RequiresUserApproval: spec.RequiresUserApproval,
					}
					if err := st.UpsertSSDSPhase(newPh); err != nil {
						return err
					}
				}
			}
		}
		proj.ActivePhases = nextActive
	}
	proj.AppliedSSDSVersion = targetVer
	if err := st.SaveSSDSProject(*proj); err != nil {
		return err
	}

	payloadBytes, _ := json.Marshal(map[string]string{
		"action":         "SSDS_VERSION_UPGRADE",
		"from_version":   prevVer,
		"to_version":     targetVer,
		"approval_quote": quote,
	})
	_, err = st.AppendGovernanceRecord(storage.GovernanceRecord{
		RecordKind: "DECISION", PhaseID: proj.CurrentPhase, AuthorityLevel: "CRITICAL",
		AuthorRole: "Release Manager", ReviewerRole: "User", EvidenceMode: "HUMAN_APPROVAL",
		Verdict: "PASS", PayloadJSON: string(payloadBytes), Approved: true,
	})
	return err
}

// RollbackSSDSUpgrade khôi phục nguyên trạng phiên bản SSDS, ActivePhases và các dòng ssds_phases từ .sen/ssds_upgrade_backup.json.
func RollbackSSDSUpgrade(st *storage.Store, root string) (string, error) {
	if strings.TrimSpace(root) == "" {
		return "", errors.New("project root is required for SSDS rollback")
	}
	bkpPath := filepath.Join(root, filepath.FromSlash(upgradeBackupRelPath))
	raw, err := os.ReadFile(bkpPath)
	if err != nil {
		return "", fmt.Errorf("no SSDS upgrade backup found at %s: %w", upgradeBackupRelPath, err)
	}
	var bkp upgradeBackup
	if err := json.Unmarshal(raw, &bkp); err != nil {
		return "", fmt.Errorf("corrupt SSDS upgrade backup: %w", err)
	}

	snapIDs := make(map[string]bool, len(bkp.PhasesSnapshot))
	for _, ph := range bkp.PhasesSnapshot {
		snapIDs[ph.PhaseID] = true
		if err := st.UpsertSSDSPhase(ph); err != nil {
			return "", err
		}
	}
	if curPhases, lErr := st.ListSSDSPhases(); lErr == nil {
		for _, curPh := range curPhases {
			if !snapIDs[curPh.PhaseID] {
				_ = st.DeleteSSDSPhase(curPh.PhaseID)
			}
		}
	}

	proj, err := st.GetSSDSProject()
	if err != nil || proj == nil {
		return "", errors.New("SSDS project is not initialized")
	}
	proj.AppliedSSDSVersion = bkp.PreviousVersion
	if len(bkp.PreviousActivePhases) > 0 {
		proj.ActivePhases = bkp.PreviousActivePhases
	}
	if err := st.SaveSSDSProject(*proj); err != nil {
		return "", err
	}

	curPhase := proj.CurrentPhase
	if curPhase == "" {
		curPhase = "P0"
	}
	payloadBytes, _ := json.Marshal(map[string]string{
		"action":           "SSDS_VERSION_ROLLBACK",
		"restored_version": bkp.PreviousVersion,
		"rolled_back_from": bkp.TargetVersion,
	})
	_, _ = st.AppendGovernanceRecord(storage.GovernanceRecord{
		RecordKind: "DECISION", PhaseID: curPhase, AuthorityLevel: "CRITICAL",
		AuthorRole: "Release Manager", ReviewerRole: "User", EvidenceMode: "HUMAN_APPROVAL",
		Verdict: "PASS", PayloadJSON: string(payloadBytes), Approved: true,
	})
	_ = os.Remove(bkpPath)
	return bkp.PreviousVersion, nil
}
