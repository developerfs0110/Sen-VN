// Tệp validator.go điều phối cổng kiểm định chất lượng (EvaluatePhaseGate) và phân tích cấu trúc tài liệu P0, P1, P2.
package ssds

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/developerfs0110/Sen-VN/internal/storage"
	"github.com/developerfs0110/Sen-VN/internal/workspace"
)

// GateResult chứa kết quả đánh giá cổng chất lượng tự động của một pha SDLC theo đúng Rulepack đang ghim.
type GateResult struct {
	PhaseID           string            `json:"phase_id"`
	AppliedVersion    string            `json:"applied_version"`
	Passed            bool              `json:"passed"`
	Verdict           string            `json:"verdict"`
	Violations        []string          `json:"violations"`
	Warnings          []string          `json:"warnings"`
	DeliverableHashes map[string]string `json:"deliverable_hashes"`
	SyncedReqs        int               `json:"synced_reqs"`
}

var (
	reHeadingLine  = regexp.MustCompile(`^(#{1,4})\s+(.+?)\s*$`)
	reLeadNumber   = regexp.MustCompile(`^(?i)(?:section\s+)?\d+(?:\.\d+)*[.):\-\s]+`)
	reReqDefLine   = regexp.MustCompile(`(?i)^\s*(?:[-*+]|\d+\.|#{2,5})?\s*\**\s*(REQ-(?:F|NF|BR|CON)-\d+)\s*\**\s*[:\-–]\s*(.+)$`)
	reUserStory    = regexp.MustCompile(`(?is)\bas\s+an?\s+.+?,?\s+i\s+want\s+.+?,?\s+so\s+that\s+.+`)
	reTaskIDToken  = regexp.MustCompile(`\bTASK-[A-Za-z0-9_-]+\b`)
	reClaimedPass  = regexp.MustCompile(`(?i)\bPASS(?:ED)?\b`)
	reOpenSeverity = regexp.MustCompile(`(?i)\b(P0|P1)\b[^\n]*\b(OPEN|UNRESOLVED|NEW|ACTIVE)\b|\b(OPEN|UNRESOLVED|NEW|ACTIVE)\b[^\n]*\b(P0|P1)\b`)
)

// RunPhaseGate thực thi kiểm định cổng chất lượng và tự động chuyển trạng thái pha nếu kết quả đạt PASS.
func RunPhaseGate(st *storage.Store, projectRoot, phaseID string) (*GateResult, *storage.SSDSPhase, error) {
	gate, err := EvaluatePhaseGate(st, projectRoot, phaseID)
	if err != nil {
		return nil, nil, err
	}
	if !gate.Passed {
		ph, _ := st.GetSSDSPhase(phaseID)
		return gate, ph, nil
	}
	ph, err := AdvancePhaseAfterGate(st, projectRoot, phaseID, gate)
	return gate, ph, err
}

// EvaluatePhaseGate nạp gói quy tắc đã ghim của dự án và kiểm định toàn diện tài liệu, mã băm, bằng chứng test và hồ sơ quản trị.
func EvaluatePhaseGate(st *storage.Store, projectRoot, phaseID string) (*GateResult, error) {
	proj, err := st.GetSSDSProject()
	if err != nil {
		return nil, err
	}
	if proj == nil {
		return nil, fmt.Errorf("SSDS project is not initialized")
	}
	rp, err := LoadRulepack(proj.AppliedSSDSVersion)
	if err != nil {
		return nil, err
	}
	pid := strings.ToUpper(strings.TrimSpace(phaseID))
	spec, ok := rp.PhaseByID(pid)
	if !ok {
		return nil, fmt.Errorf("unknown SSDS phase %q in rulepack v%s", pid, rp.Version)
	}

	res := &GateResult{
		PhaseID:           pid,
		AppliedVersion:    rp.Version,
		DeliverableHashes: make(map[string]string),
	}

	if excErr := CheckActiveExceptions(st); excErr != nil {
		res.Violations = append(res.Violations, excErr.Error())
	}

	validatePreCodeBaselineIntegrity(st, proj, projectRoot, pid, res)
	validatePhaseDeliverablesAndHeadings(projectRoot, proj.WorkflowTier, *spec, res)
	validateGovernanceReviewGate(st, proj.WorkflowTier, *spec, res)

	switch pid {
	case "P0":
		validatePhase0Brownfield(st, proj, projectRoot, res)
	case "P1":
		validateAndSyncPhase1Requirements(st, proj, projectRoot, res)
	case "P2":
		validatePhase2Architecture(st, proj, projectRoot, res)
	case "P3":
		validatePhase3Planning(st, proj, projectRoot, res)
	case "P4":
		validatePhase4Foundation(st, res)
	case "P5":
		validatePhase5Implementation(st, res)
	case "P6":
		validatePhase6QAAndSecurity(st, projectRoot, res)
	case "P7":
		validatePhase7Release(st, projectRoot, res)
	}

	res.Passed = len(res.Violations) == 0
	if res.Passed {
		res.Verdict = "PASS"
	} else {
		res.Verdict = "FAIL"
	}
	return res, nil
}

// validatePhaseDeliverablesAndHeadings kiểm tra từng tệp bàn giao trên đĩa và từ chối mục rỗng hoặc chỉ ghi TBD/TODO/...
func validatePhaseDeliverablesAndHeadings(root, tier string, spec PhaseSpec, res *GateResult) {
	delivs := spec.EffectiveDeliverables(tier)
	headings := spec.EffectiveHeadings(tier)

	for _, rel := range delivs {
		full := filepath.Join(root, filepath.FromSlash(rel))
		raw, err := os.ReadFile(full)
		if err != nil {
			res.Violations = append(res.Violations, fmt.Sprintf("%s missing required deliverable: %s", spec.PhaseID, rel))
			continue
		}
		content := string(raw)
		if isPlaceholderTextBody(content) {
			res.Violations = append(res.Violations, fmt.Sprintf("%s deliverable %s is empty or placeholder-only", spec.PhaseID, rel))
			continue
		}
		if sum, hErr := hashDeliverableFile(full); hErr == nil {
			res.DeliverableHashes[rel] = sum
		}
		for _, wantH := range headings[rel] {
			if okH, reason := markdownHasPopulatedSection(content, wantH); !okH {
				res.Violations = append(res.Violations, fmt.Sprintf("%s (%s): %s", spec.PhaseID, rel, reason))
			}
		}
	}
}

// validatePreCodeBaselineIntegrity kiểm tra tính toàn vẹn __BASELINE__ tại P0..P4 để phát hiện tệp mã nguồn bị tạo lén bằng shell ngoài luồng.
func validatePreCodeBaselineIntegrity(st *storage.Store, proj *storage.SSDSProject, root, pid string, res *GateResult) {
	if strings.TrimSpace(root) == "" {
		return
	}
	if pid != "P0" && pid != "P1" && pid != "P2" && pid != "P3" && pid != "P4" {
		return
	}
	baseSnaps, err := st.GetSnapshots(workspace.BaselineTaskID)
	if err != nil || len(baseSnaps) == 0 {
		return
	}
	allowed := []string{"AGENTS.md", "PROJECT_STATUS.md", "ROADMAP.md", "TASKS.md", "docs/DECISIONS.md"}
	allowed = append(allowed, collectNonCodeDocDeliverables(st, root)...)
	if proj.Category == "PROTOTYPE" {
		for _, sub := range []string{"prototype", "spike"} {
			_ = filepath.WalkDir(filepath.Join(root, sub), func(p string, d os.DirEntry, wErr error) error {
				if wErr == nil && !d.IsDir() {
					if rel, rErr := workspace.NormalizeRelPath(root, p); rErr == nil {
						allowed = append(allowed, rel)
					}
				}
				return nil
			})
		}
	}
	if iErr := workspace.CheckIntegrity(st, root, "", allowed); iErr != nil {
		res.Violations = append(res.Violations, fmt.Sprintf("SSDS phase firewall integrity violation (%s): %v", pid, iErr))
	}
}

// validateGovernanceReviewGate kiểm tra biên bản REVIEW trong ssds_governance và từ chối SELF_REVIEW khi pha bắt buộc phản biện độc lập (§4.3/§18).
func validateGovernanceReviewGate(st *storage.Store, tier string, spec PhaseSpec, res *GateResult) {
	pid := spec.PhaseID
	needReview := strings.EqualFold(tier, "WF_CRITICAL") ||
		(strings.EqualFold(tier, "WF_STANDARD") && spec.RequiresIndependentReview)
	if !needReview {
		return
	}
	revs, err := st.ListGovernanceRecords("REVIEW", pid)
	if err != nil {
		res.Violations = append(res.Violations, fmt.Sprintf("read governance reviews for %s: %v", pid, err))
		return
	}
	hasIndependentException := false
	if spec.RequiresIndependentReview && strings.EqualFold(tier, "WF_STANDARD") {
		if excs, eErr := st.ListGovernanceRecords("EXCEPTION", pid); eErr == nil && CheckActiveExceptions(st) == nil {
			for _, ex := range excs {
				if ex.Approved && ex.Verdict == "PASS" {
					hasIndependentException = true
					break
				}
			}
		}
	}
	sawSelfReview := false
	for _, r := range revs {
		if r.Verdict != "PASS" {
			continue
		}
		if !spec.RequiresIndependentReview || hasIndependentException {
			return
		}
		if r.EvidenceMode == "INDEPENDENT_AI_REVIEW" || r.EvidenceMode == "HUMAN_APPROVAL" {
			return
		}
		sawSelfReview = true
	}
	if sawSelfReview && spec.RequiresIndependentReview {
		res.Violations = append(res.Violations, fmt.Sprintf(
			"SSDS §4.3/§18 gate: phase %s (%s) requires INDEPENDENT_AI_REVIEW (--ref) or HUMAN_APPROVAL; SELF_REVIEW is rejected",
			pid, tier,
		))
		return
	}
	res.Violations = append(res.Violations, fmt.Sprintf("SSDS §4.3/§18 gate: phase %s (%s) requires a passing REVIEW record in ssds_governance", pid, tier))
}

// validatePhase0Brownfield xác minh dự án có sẵn mã nguồn đã thực hiện khảo sát kho mã theo điều khoản SSDS §2.3.
func validatePhase0Brownfield(st *storage.Store, proj *storage.SSDSProject, root string, res *GateResult) {
	if proj.Category != "BROWNFIELD" && proj.Category != "MAINTENANCE" && proj.Category != "MIGRATION" && proj.Category != "INTEGRATION" {
		return
	}
	if proj.BrownfieldAudited {
		return
	}
	if _, err := os.Stat(filepath.Join(root, ".sen", "brownfield_audit.json")); err == nil {
		_ = st.SetBrownfieldAudited(true)
		return
	}
	res.Violations = append(res.Violations, fmt.Sprintf("SSDS §2.3 violation: %s project requires InspectBrownfieldRepo before passing P0 gate", proj.Category))
}

// validateAndSyncPhase1Requirements phân tích cú pháp REQ-F/NF/BR/CON, User Story, Given/When/Then và đồng bộ vào SQLite.
func validateAndSyncPhase1Requirements(st *storage.Store, proj *storage.SSDSProject, root string, res *GateResult) {
	reqPath := filepath.Join(root, "docs", "01-requirements.md")
	raw, err := os.ReadFile(reqPath)
	if err != nil {
		return
	}
	content := string(raw)
	docSHA, _ := hashDeliverableFile(reqPath)

	if !reUserStory.MatchString(content) {
		res.Violations = append(res.Violations, "SSDS §6 violation: docs/01-requirements.md must include User Stories matching 'As a [user], I want [capability], so that [value]'")
	}
	lower := strings.ToLower(content)
	if !strings.Contains(lower, "given ") || !strings.Contains(lower, "when ") || !strings.Contains(lower, "then ") {
		res.Violations = append(res.Violations, "SSDS §6 violation: Acceptance Criteria in docs/01-requirements.md must use Given / When / Then")
	}

	seenDefs := make(map[string]bool)
	type parsedReq struct{ id, summary string }
	var list []parsedReq
	funcCount := 0
	for _, line := range strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n") {
		if strings.HasPrefix(strings.TrimSpace(line), "|") {
			continue
		}
		m := reReqDefLine.FindStringSubmatch(line)
		if len(m) != 3 {
			continue
		}
		id := strings.ToUpper(m[1])
		sum := strings.TrimSpace(m[2])
		if isPlaceholderValue(sum) {
			res.Violations = append(res.Violations, fmt.Sprintf("SSDS §6 violation: requirement %s has placeholder summary", id))
			continue
		}
		if seenDefs[id] {
			res.Violations = append(res.Violations, fmt.Sprintf("SSDS §6 violation: duplicate requirement definition %s in docs/01-requirements.md", id))
			continue
		}
		seenDefs[id] = true
		if strings.HasPrefix(id, "REQ-F-") {
			funcCount++
		}
		list = append(list, parsedReq{id: id, summary: sum})
	}
	if funcCount == 0 {
		res.Violations = append(res.Violations, "SSDS §6 violation: docs/01-requirements.md must define at least one functional requirement (REQ-F-001: ...)")
	}
	if len(res.Violations) > 0 {
		return
	}
	for _, item := range list {
		if _, uErr := st.UpsertTypedRequirement(item.id, "", "docs/01-requirements.md", item.summary, "Given/When/Then verified in docs/01-requirements.md", docSHA, proj.Category == "MAINTENANCE"); uErr == nil {
			res.SyncedReqs++
		}
	}
}

// validatePhase2Architecture kiểm tra tệp ADR-*.md bắt buộc và đối chiếu độ phủ yêu cầu REQ-F/NF trong kiến trúc.
func validatePhase2Architecture(st *storage.Store, proj *storage.SSDSProject, root string, res *GateResult) {
	archPath := filepath.Join(root, "docs", "02-architecture.md")
	rawArch, err := os.ReadFile(archPath)
	if err != nil {
		return
	}
	archUpper := strings.ToUpper(string(rawArch))

	if proj.WorkflowTier != "WF_MICRO" {
		adrDir := filepath.Join(root, "docs", "adr")
		entries, dErr := os.ReadDir(adrDir)
		validADR := 0
		if dErr == nil {
			for _, e := range entries {
				nameLower := strings.ToLower(e.Name())
				if !e.IsDir() && strings.HasPrefix(nameLower, "adr-") && strings.HasSuffix(nameLower, ".md") {
					fullADR := filepath.Join(adrDir, e.Name())
					if b, rErr := os.ReadFile(fullADR); rErr == nil && !isPlaceholderTextBody(string(b)) {
						validADR++
						if sum, hErr := hashDeliverableFile(fullADR); hErr == nil {
							res.DeliverableHashes["docs/adr/"+e.Name()] = sum
						}
					}
				}
			}
		}
		if validADR == 0 {
			res.Violations = append(res.Violations, "SSDS §7 violation: at least one non-empty docs/adr/ADR-*.md decision record is required")
		}
	}

	reqs, _ := st.ListRequirements()
	for _, r := range reqs {
		u := strings.ToUpper(r.ID)
		if (strings.HasPrefix(u, "REQ-F-") || strings.HasPrefix(u, "REQ-NF-")) && !strings.Contains(archUpper, u) {
			res.Violations = append(res.Violations, fmt.Sprintf("SSDS §7 violation: docs/02-architecture.md does not map requirement %s to an architecture component", r.ID))
		}
	}
}

// markdownHasPopulatedSection kiểm tra tệp Markdown có chứa tiêu đề yêu cầu với phần nội dung thực chất (không rỗng/TBD/TODO).
func markdownHasPopulatedSection(content, wantedHeading string) (bool, string) {
	wantNorm := strings.ToLower(strings.TrimSpace(wantedHeading))
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")

	for i := 0; i < len(lines); i++ {
		m := reHeadingLine.FindStringSubmatch(lines[i])
		if len(m) != 3 {
			continue
		}
		level := len(m[1])
		cleanTitle := strings.ToLower(strings.TrimSpace(reLeadNumber.ReplaceAllString(strings.TrimSpace(m[2]), "")))
		if !strings.Contains(cleanTitle, wantNorm) {
			continue
		}
		var bodyLines []string
		for j := i + 1; j < len(lines); j++ {
			if nextH := reHeadingLine.FindStringSubmatch(lines[j]); len(nextH) == 3 && len(nextH[1]) <= level {
				break
			}
			bodyLines = append(bodyLines, lines[j])
		}
		body := strings.TrimSpace(strings.Join(bodyLines, "\n"))
		if isPlaceholderTextBody(body) {
			return false, fmt.Sprintf("section %q is empty or contains only placeholder (TBD/TODO/...)", wantedHeading)
		}
		return true, ""
	}
	return false, fmt.Sprintf("missing required section heading %q", wantedHeading)
}

// isPlaceholderTextBody nhận diện đoạn văn bản trống rỗng hoặc chỉ chứa từ khóa giữ chỗ tạm bợ.
func isPlaceholderTextBody(body string) bool {
	cleaned := strings.Trim(strings.TrimSpace(body), "-*+#>` \t\r\n.")
	if cleaned == "" {
		return true
	}
	switch strings.ToUpper(cleaned) {
	case "TBD", "TODO", "WIP", "FIXME", "COMING SOON":
		return true
	default:
		return false
	}
}
