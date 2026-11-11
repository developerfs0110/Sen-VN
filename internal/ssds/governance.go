// Tệp governance.go thực thi khóa phân tích tác động thay đổi CIA (§17) và quản lý ngoại lệ có thời hạn EXC (§27).
package ssds

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/developerfs0110/Sen-VN/internal/storage"
)

// ChangeImpactSpec định nghĩa đủ 10 chiều phân tích tác động thay đổi bắt buộc theo điều khoản SSDS §17.
type ChangeImpactSpec struct {
	Summary                    string   `json:"summary,omitempty"`
	TargetDocument             string   `json:"target_document"`
	SealedBaselineSHA256       string   `json:"sealed_baseline_sha256,omitempty"`
	AffectedRequirements       []string `json:"affected_requirements"`
	AffectedADRs               []string `json:"affected_adrs"`
	AffectedComponents         []string `json:"affected_components"`
	AffectedModules            []string `json:"affected_modules"`
	AffectedDatabase           string   `json:"affected_database"`
	AffectedAPIs               string   `json:"affected_apis"`
	AffectedTests              []string `json:"affected_tests"`
	AffectedDocs               []string `json:"affected_docs"`
	BackwardCompatibilityRisks string   `json:"backward_compatibility_risks"`
	MigrationAndRollback       string   `json:"migration_and_rollback"`
}

// ExceptionSpec định nghĩa đủ 7 trường bắt buộc cho hồ sơ ngoại lệ có thời hạn theo điều khoản SSDS §27.
type ExceptionSpec struct {
	ExceptionID         string `json:"exception_id"`
	Reason              string `json:"reason"`
	AffectedRequirement string `json:"affected_requirement"`
	RiskAssessment      string `json:"risk_assessment"`
	Mitigation          string `json:"mitigation"`
	Approval            string `json:"approval"`
	Expiration          string `json:"expiration"`
}

// baselineOwnerPhase xác định pha sở hữu baseline (P1 hoặc P2) cho đường dẫn tài liệu viết thường.
func baselineOwnerPhase(lower string) string {
	switch {
	case lower == "docs/01-requirements.md":
		return "P1"
	case lower == "docs/02-architecture.md" ||
		lower == "docs/03-database-design.md" ||
		lower == "docs/04-api-specification.md" ||
		lower == "docs/05-security-design.md" ||
		(strings.HasPrefix(lower, "docs/adr/adr-") && strings.HasSuffix(lower, ".md")):
		return "P2"
	default:
		return ""
	}
}

// lookupSealedDeliverableHash tra cứu mã băm SHA-256 đang niêm phong của một tài liệu trong pha sở hữu.
func lookupSealedDeliverableHash(ph *storage.SSDSPhase, lowerRel string) string {
	if ph == nil {
		return ""
	}
	for k, h := range ph.VerifiedDeliverables {
		if strings.ToLower(filepath.ToSlash(strings.TrimSpace(k))) == lowerRel {
			return h
		}
	}
	return ""
}

// checkBaselineCIALock chặn sửa đổi tài liệu yêu cầu/kiến trúc đã APPROVED nếu chưa có hồ sơ CHANGE_IMPACT (CIA-*) hợp lệ.
func checkBaselineCIALock(st *storage.Store, rel, lower string) error {
	ownerPhase := baselineOwnerPhase(lower)
	if ownerPhase == "" {
		return nil
	}
	ph, err := st.GetSSDSPhase(ownerPhase)
	if err != nil || (ph.Status != "APPROVED" && ph.Status != "COMPLETED") {
		return nil
	}
	okCIA, err := hasAuthorizedChangeImpact(st, lower, lookupSealedDeliverableHash(ph, lower))
	if err != nil {
		return err
	}
	if !okCIA {
		return fmt.Errorf("SSDS §17 Change Impact Analysis lock: baseline document %q (%s is %s) requires an active CHANGE_IMPACT (CIA-*) record (and User approval if CRITICAL)",
			rel, ownerPhase, ph.Status)
	}
	return nil
}

// hasAuthorizedChangeImpact kiểm tra bản ghi CIA-* khớp đúng tài liệu và mã băm niêm phong hiện tại (tự hết hiệu lực sau khi re-seal).
func hasAuthorizedChangeImpact(st *storage.Store, lowerRel, currentSealedHash string) (bool, error) {
	records, err := st.ListGovernanceRecords("CHANGE_IMPACT", "")
	if err != nil {
		return false, err
	}
	for _, r := range records {
		if r.Verdict == "FAIL" || r.Verdict == "BLOCKED" {
			continue
		}
		if strings.EqualFold(r.AuthorityLevel, "CRITICAL") && !r.Approved {
			continue
		}
		var spec ChangeImpactSpec
		if err := json.Unmarshal([]byte(r.PayloadJSON), &spec); err != nil {
			continue
		}
		tgt := strings.ToLower(filepath.ToSlash(strings.TrimSpace(spec.TargetDocument)))
		if tgt == "" || tgt != lowerRel {
			continue
		}
		if currentSealedHash != "" && spec.SealedBaselineSHA256 != "" && spec.SealedBaselineSHA256 != currentSealedHash {
			continue
		}
		return true, nil
	}
	return false, nil
}

// CheckActiveExceptions kiểm tra toàn bộ hồ sơ EXCEPTION (EXC-*) đảm bảo đã được duyệt, đủ thông tin và chưa quá hạn hiệu lực (§27).
func CheckActiveExceptions(st *storage.Store) error {
	if err := st.VerifyGovernanceChain(); err != nil {
		return err
	}
	excs, err := st.ListGovernanceRecords("EXCEPTION", "")
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	for _, ex := range excs {
		if !ex.Approved {
			return fmt.Errorf("SSDS §27 violation: exception %s is not approved", ex.RecordID)
		}
		expStr := strings.TrimSpace(ex.ExpiresAt)
		if expStr == "" {
			return fmt.Errorf("SSDS §27 violation: exception %s is missing expiration timestamp", ex.RecordID)
		}
		expTime, pErr := parseFlexibleTime(expStr)
		if pErr != nil {
			return fmt.Errorf("SSDS §27 violation: exception %s has invalid expires_at %q: %w", ex.RecordID, expStr, pErr)
		}
		if !expTime.After(now) {
			return fmt.Errorf("SSDS §27 violation: exception %s expired at %s", ex.RecordID, expStr)
		}
	}
	return nil
}

// parseFlexibleTime hỗ trợ giải mã hạn hiệu lực ngoại lệ ở định dạng RFC3339 hoặc ngày YYYY-MM-DD.
func parseFlexibleTime(raw string) (time.Time, error) {
	if t, err := time.Parse(time.RFC3339, raw); err == nil {
		return t, nil
	}
	return time.Parse("2006-01-02", raw)
}

// RecordChangeImpact ghi nhận hồ sơ phân tích tác động thay đổi đủ 10 chiều theo điều khoản SSDS §17 vào ssds_governance.
func RecordChangeImpact(
	st *storage.Store, phaseID, authorityLevel, authorRole string,
	spec ChangeImpactSpec, approved bool, approverQuote string,
) (*storage.GovernanceRecord, error) {
	spec.TargetDocument = filepath.ToSlash(strings.TrimSpace(spec.TargetDocument))
	if spec.TargetDocument == "" {
		return nil, errors.New("SSDS §17 violation: specific non-empty TargetDocument (--doc) is required for Change Impact Analysis")
	}
	lowerDoc := strings.ToLower(spec.TargetDocument)
	if owner := baselineOwnerPhase(lowerDoc); owner != "" && spec.SealedBaselineSHA256 == "" {
		if ph, pErr := st.GetSSDSPhase(owner); pErr == nil {
			spec.SealedBaselineSHA256 = lookupSealedDeliverableHash(ph, lowerDoc)
		}
	}
	auth := strings.ToUpper(strings.TrimSpace(authorityLevel))
	if auth == "" {
		auth = "CRITICAL"
	}
	if auth == "MAJOR" || auth == "CRITICAL" {
		if missing := validateTenImpactDimensions(spec); len(missing) > 0 {
			return nil, fmt.Errorf("SSDS §17 violation: %s change impact is missing mandatory dimensions: %s", auth, strings.Join(missing, ", "))
		}
	}
	if auth == "CRITICAL" && approved && strings.TrimSpace(approverQuote) == "" {
		return nil, errors.New("SSDS §17/§21 violation: CRITICAL change impact cannot be marked approved without approverQuote")
	}
	directApprove := approved && auth != "CRITICAL" && strings.TrimSpace(approverQuote) == ""
	raw, _ := json.Marshal(spec)
	rec, err := st.AppendGovernanceRecord(storage.GovernanceRecord{
		RecordKind: "CHANGE_IMPACT", PhaseID: phaseID, AuthorityLevel: auth,
		AuthorRole: authorRole, EvidenceMode: "SELF_REVIEW", Verdict: "PASS",
		PayloadJSON: string(raw), Approved: directApprove,
	})
	if err != nil {
		return nil, err
	}
	if approved && !directApprove {
		if err := st.ApproveGovernanceRecord(rec.RecordID, approverQuote); err != nil {
			return nil, err
		}
		list, _ := st.ListGovernanceRecords("CHANGE_IMPACT", phaseID)
		for i := len(list) - 1; i >= 0; i-- {
			if list[i].RecordID == rec.RecordID {
				cp := list[i]
				return &cp, nil
			}
		}
	}
	return rec, nil
}

// validateTenImpactDimensions kiểm tra đủ 10 chiều tác động bắt buộc của SSDS §17 đối với thay đổi cấp MAJOR và CRITICAL.
func validateTenImpactDimensions(spec ChangeImpactSpec) []string {
	var missing []string
	if len(spec.AffectedRequirements) == 0 {
		missing = append(missing, "affected_requirements")
	}
	if len(spec.AffectedADRs) == 0 {
		missing = append(missing, "affected_adrs")
	}
	if len(spec.AffectedComponents) == 0 {
		missing = append(missing, "affected_components")
	}
	if len(spec.AffectedModules) == 0 {
		missing = append(missing, "affected_modules")
	}
	if strings.TrimSpace(spec.AffectedDatabase) == "" {
		missing = append(missing, "affected_database")
	}
	if strings.TrimSpace(spec.AffectedAPIs) == "" {
		missing = append(missing, "affected_apis")
	}
	if len(spec.AffectedTests) == 0 {
		missing = append(missing, "affected_tests")
	}
	if len(spec.AffectedDocs) == 0 {
		missing = append(missing, "affected_docs")
	}
	if strings.TrimSpace(spec.BackwardCompatibilityRisks) == "" {
		missing = append(missing, "backward_compatibility_risks")
	}
	if strings.TrimSpace(spec.MigrationAndRollback) == "" {
		missing = append(missing, "migration_and_rollback")
	}
	return missing
}

// RecordException kiểm tra đủ 7 trường bắt buộc của SSDS §27 và lưu hồ sơ ngoại lệ có thời hạn vào ssds_governance.
func RecordException(
	st *storage.Store, phaseID, authorityLevel, authorRole string, spec ExceptionSpec,
) (*storage.GovernanceRecord, error) {
	if strings.TrimSpace(spec.ExceptionID) == "" || strings.TrimSpace(spec.Reason) == "" ||
		strings.TrimSpace(spec.AffectedRequirement) == "" || strings.TrimSpace(spec.RiskAssessment) == "" ||
		strings.TrimSpace(spec.Mitigation) == "" || strings.TrimSpace(spec.Approval) == "" ||
		strings.TrimSpace(spec.Expiration) == "" {
		return nil, errors.New("SSDS §27 violation: all 7 exception fields (ID, Reason, AffectedRequirement, RiskAssessment, Mitigation, Approval, Expiration) are required")
	}
	if _, err := parseFlexibleTime(strings.TrimSpace(spec.Expiration)); err != nil {
		return nil, fmt.Errorf("invalid exception expiration %q: %w", spec.Expiration, err)
	}
	raw, _ := json.Marshal(spec)
	return st.AppendGovernanceRecord(storage.GovernanceRecord{
		RecordID: strings.TrimSpace(spec.ExceptionID), RecordKind: "EXCEPTION", PhaseID: phaseID,
		AuthorityLevel: authorityLevel, AuthorRole: authorRole, ReviewerRole: "User",
		EvidenceMode: "HUMAN_APPROVAL", Verdict: "PASS", PayloadJSON: string(raw),
		Approved: true, ExpiresAt: strings.TrimSpace(spec.Expiration),
	})
}

