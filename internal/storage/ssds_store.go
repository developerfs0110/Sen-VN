// Tệp ssds_store.go cung cấp các hàm giao dịch ACID cho bảng định danh dự án (ssds_project) và vòng đời pha (ssds_phases).
package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/developerfs0110/Sen-VN/internal/redact"
)

// SSDSProject lưu định danh duy nhất (UUIDv4), phiên bản SSDS đang ghim, phân loại và pha hiện hành của dự án.
type SSDSProject struct {
	ProjectID          string
	ProjectName        string
	AppliedSSDSVersion string
	Category           string
	WorkflowTier       string
	ActivePhases       []string
	RiskAssessment     map[string]string
	CurrentPhase       string
	BrownfieldAudited  bool
	ReleaseTag         string
	CreatedAt          string
	UpdatedAt          string
}

// SSDSPhase biểu diễn trạng thái vòng đời, vai trò phụ trách, deliverables và cổng phê duyệt của một pha P0..P9.
type SSDSPhase struct {
	PhaseID              string
	Status               string
	PrimaryRole          string
	SupportingRoles      []string
	RequiredDeliverables []string
	VerifiedDeliverables map[string]string
	RequiresUserApproval bool
	GatePassed           bool
	ApprovedBy           string
	ApprovedQuote        string
	ApprovedAt           string
	UpdatedAt            string
}

// GetSSDSProject đọc bản ghi dự án SSDS duy nhất; trả về (nil, nil) khi bảng đang trống.
func (s *Store) GetSSDSProject() (*SSDSProject, error) {
	var p SSDSProject
	var phasesJSON, riskJSON string
	var audited int
	err := s.db.QueryRow(`
		SELECT project_id, project_name, applied_ssds_version, category, workflow_tier,
		       active_phases_json, risk_assessment_json, current_phase, brownfield_audited,
		       release_tag, created_at, updated_at
		FROM ssds_project LIMIT 1
	`).Scan(
		&p.ProjectID, &p.ProjectName, &p.AppliedSSDSVersion, &p.Category, &p.WorkflowTier,
		&phasesJSON, &riskJSON, &p.CurrentPhase, &audited, &p.ReleaseTag, &p.CreatedAt, &p.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	p.BrownfieldAudited = audited != 0
	_ = json.Unmarshal([]byte(phasesJSON), &p.ActivePhases)
	p.RiskAssessment = make(map[string]string)
	_ = json.Unmarshal([]byte(riskJSON), &p.RiskAssessment)
	return &p, nil
}

// SaveSSDSProject lưu trạng thái dự án đơn dòng và chặn tuyệt đối việc ghi đè bằng một project_id khác trên cùng state.db.
func (s *Store) SaveSSDSProject(p SSDSProject) error {
	if strings.TrimSpace(p.ProjectID) == "" {
		return errors.New("ssds project_id cannot be empty")
	}
	if strings.TrimSpace(p.AppliedSSDSVersion) == "" || strings.TrimSpace(p.Category) == "" ||
		strings.TrimSpace(p.WorkflowTier) == "" || strings.TrimSpace(p.CurrentPhase) == "" {
		return errors.New("applied_ssds_version, category, workflow_tier, and current_phase are required")
	}
	activePhases := p.ActivePhases
	if activePhases == nil {
		activePhases = []string{}
	}
	riskMap := p.RiskAssessment
	if riskMap == nil {
		riskMap = map[string]string{}
	}
	phasesBytes, err := json.Marshal(activePhases)
	if err != nil {
		return err
	}
	riskBytes, err := json.Marshal(riskMap)
	if err != nil {
		return err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var existingID, existingCreated string
	err = tx.QueryRow(`SELECT project_id, created_at FROM ssds_project LIMIT 1`).Scan(&existingID, &existingCreated)
	if err == nil && existingID != p.ProjectID {
		return fmt.Errorf("cross-project contamination blocked: .sen/state.db belongs to project_id %q, cannot overwrite with %q", existingID, p.ProjectID)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	ts := nowUTC()
	created := p.CreatedAt
	if existingCreated != "" {
		created = existingCreated
	} else if created == "" {
		created = ts
	}

	_, err = tx.Exec(`
		INSERT INTO ssds_project (
			project_id, project_name, applied_ssds_version, category, workflow_tier,
			active_phases_json, risk_assessment_json, current_phase, brownfield_audited,
			release_tag, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(project_id) DO UPDATE SET
			project_name = excluded.project_name,
			applied_ssds_version = excluded.applied_ssds_version,
			category = excluded.category,
			workflow_tier = excluded.workflow_tier,
			active_phases_json = excluded.active_phases_json,
			risk_assessment_json = excluded.risk_assessment_json,
			current_phase = excluded.current_phase,
			brownfield_audited = excluded.brownfield_audited,
			release_tag = excluded.release_tag,
			updated_at = excluded.updated_at
	`, p.ProjectID, p.ProjectName, p.AppliedSSDSVersion, p.Category, p.WorkflowTier,
		string(phasesBytes), string(riskBytes), p.CurrentPhase, boolToInt(p.BrownfieldAudited), p.ReleaseTag, created, ts)
	if err != nil {
		return err
	}
	return tx.Commit()
}

// UpdateCurrentPhase cập nhật mã pha hiện hành đang hoạt động trong bảng ssds_project.
func (s *Store) UpdateCurrentPhase(phaseID string) error {
	clean := strings.TrimSpace(phaseID)
	if clean == "" {
		return errors.New("current_phase cannot be empty")
	}
	res, err := s.db.Exec(`UPDATE ssds_project SET current_phase = ?, updated_at = ?`, clean, nowUTC())
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("ssds_project is not initialized")
	}
	return nil
}

// SetBrownfieldAudited đánh dấu hoàn tất khảo sát hiện trạng kho mã có sẵn theo điều khoản SSDS §2.3.
func (s *Store) SetBrownfieldAudited(audited bool) error {
	res, err := s.db.Exec(`UPDATE ssds_project SET brownfield_audited = ?, updated_at = ?`, boolToInt(audited), nowUTC())
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("ssds_project is not initialized")
	}
	return nil
}

// UpdateAppliedSSDSVersion cập nhật phiên bản chuẩn SSDS đang áp dụng cho dự án khi nâng cấp hoặc hoàn tác có kiểm soát.
func (s *Store) UpdateAppliedSSDSVersion(newVer string) error {
	clean := strings.TrimSpace(newVer)
	if clean == "" {
		return errors.New("applied_ssds_version cannot be empty")
	}
	res, err := s.db.Exec(`UPDATE ssds_project SET applied_ssds_version = ?, updated_at = ?`, clean, nowUTC())
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("ssds_project is not initialized")
	}
	return nil
}

// UpsertSSDSPhase thêm mới hoặc cập nhật trạng thái, vai trò và cổng phê duyệt của một pha trong bảng ssds_phases.
func (s *Store) UpsertSSDSPhase(ph SSDSPhase) error {
	if strings.TrimSpace(ph.PhaseID) == "" || strings.TrimSpace(ph.Status) == "" || strings.TrimSpace(ph.PrimaryRole) == "" {
		return errors.New("phase_id, status, and primary_role are required")
	}
	supRoles := ph.SupportingRoles
	if supRoles == nil {
		supRoles = []string{}
	}
	reqDeliv := ph.RequiredDeliverables
	if reqDeliv == nil {
		reqDeliv = []string{}
	}
	verDeliv := ph.VerifiedDeliverables
	if verDeliv == nil {
		verDeliv = map[string]string{}
	}
	supBytes, _ := json.Marshal(supRoles)
	reqBytes, _ := json.Marshal(reqDeliv)
	verBytes, _ := json.Marshal(verDeliv)
	ts := nowUTC()

	_, err := s.db.Exec(`
		INSERT INTO ssds_phases (
			phase_id, status, primary_role, supporting_roles_json, required_deliverables_json,
			verified_deliverables_json, requires_user_approval, gate_passed, approved_by,
			approved_quote, approved_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(phase_id) DO UPDATE SET
			status = excluded.status,
			primary_role = excluded.primary_role,
			supporting_roles_json = excluded.supporting_roles_json,
			required_deliverables_json = excluded.required_deliverables_json,
			verified_deliverables_json = excluded.verified_deliverables_json,
			requires_user_approval = excluded.requires_user_approval,
			gate_passed = excluded.gate_passed,
			approved_by = excluded.approved_by,
			approved_quote = excluded.approved_quote,
			approved_at = excluded.approved_at,
			updated_at = excluded.updated_at
	`, ph.PhaseID, ph.Status, ph.PrimaryRole, string(supBytes), string(reqBytes), string(verBytes),
		boolToInt(ph.RequiresUserApproval), boolToInt(ph.GatePassed), ph.ApprovedBy, redact.ScrubText(ph.ApprovedQuote), ph.ApprovedAt, ts)
	return err
}

// DeleteSSDSPhase xóa bản ghi của một pha phát sinh thêm khi thực hiện hoàn tác phiên bản SSDS (RollbackSSDSUpgrade).
func (s *Store) DeleteSSDSPhase(phaseID string) error {
	_, err := s.db.Exec(`DELETE FROM ssds_phases WHERE phase_id = ?`, strings.TrimSpace(phaseID))
	return err
}

// GetSSDSPhase đọc thông tin chi tiết của một pha SDLC cụ thể theo mã phase_id (P0..P9).
func (s *Store) GetSSDSPhase(phaseID string) (*SSDSPhase, error) {
	row := s.db.QueryRow(`
		SELECT phase_id, status, primary_role, supporting_roles_json, required_deliverables_json,
		       verified_deliverables_json, requires_user_approval, gate_passed, approved_by,
		       approved_quote, approved_at, updated_at
		FROM ssds_phases WHERE phase_id = ?
	`, strings.TrimSpace(phaseID))
	ph, err := scanSSDSPhaseRow(row.Scan)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("ssds phase %q not found", phaseID)
	}
	if err != nil {
		return nil, err
	}
	return &ph, nil
}

// ListSSDSPhases trả về danh sách toàn bộ các pha của dự án sắp xếp tăng dần theo mã pha (P0..P9).
func (s *Store) ListSSDSPhases() ([]SSDSPhase, error) {
	rows, err := s.db.Query(`
		SELECT phase_id, status, primary_role, supporting_roles_json, required_deliverables_json,
		       verified_deliverables_json, requires_user_approval, gate_passed, approved_by,
		       approved_quote, approved_at, updated_at
		FROM ssds_phases ORDER BY phase_id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []SSDSPhase
	for rows.Next() {
		ph, err := scanSSDSPhaseRow(rows.Scan)
		if err != nil {
			return nil, err
		}
		out = append(out, ph)
	}
	return out, rows.Err()
}

// scanSSDSPhaseRow giải mã một dòng kết quả truy vấn từ bảng ssds_phases sang cấu trúc SSDSPhase.
func scanSSDSPhaseRow(scanFn func(dest ...any) error) (SSDSPhase, error) {
	var ph SSDSPhase
	var supJSON, reqJSON, verJSON string
	var reqApp, gateOK int
	err := scanFn(
		&ph.PhaseID, &ph.Status, &ph.PrimaryRole, &supJSON, &reqJSON,
		&verJSON, &reqApp, &gateOK, &ph.ApprovedBy, &ph.ApprovedQuote, &ph.ApprovedAt, &ph.UpdatedAt,
	)
	if err != nil {
		return ph, err
	}
	ph.RequiresUserApproval = reqApp != 0
	ph.GatePassed = gateOK != 0
	_ = json.Unmarshal([]byte(supJSON), &ph.SupportingRoles)
	_ = json.Unmarshal([]byte(reqJSON), &ph.RequiredDeliverables)
	ph.VerifiedDeliverables = make(map[string]string)
	_ = json.Unmarshal([]byte(verJSON), &ph.VerifiedDeliverables)
	return ph, nil
}
