// Tệp roles.go quản lý danh mục 27 vai trò kỹ thuật chuyên môn (§4.1) và cơ chế kiểm chứng phản biện độc lập (§4.3, §18).
package ssds

import (
	"fmt"
	"strings"
)

// RoleDefinition mô tả một vai trò kỹ thuật phần mềm chuẩn thuộc 1 trong 6 khối chuyên môn của SSDS §4.1.
type RoleDefinition struct {
	Name             string
	Group            string
	Responsibilities string
}

// engineeringCatalog định nghĩa đầy đủ 27 vai trò chuyên môn thuộc 6 nhóm trong SSDS §4.1.
var engineeringCatalog = []RoleDefinition{
	// 1. Khối Nghiệp vụ và Sản phẩm (4 vai trò)
	{Name: "Product Manager", Group: "Business and Product", Responsibilities: "Business vision, intake charter, scope prioritization, success metrics"},
	{Name: "Product Owner", Group: "Business and Product", Responsibilities: "Backlog prioritization, user story value alignment, stakeholder sign-off"},
	{Name: "Business Analyst", Group: "Business and Product", Responsibilities: "Elicit functional/non-functional requirements, business rules, acceptance criteria"},
	{Name: "System Analyst", Group: "Business and Product", Responsibilities: "Cross-check requirement feasibility, consistency, boundary interactions"},
	// 2. Khối Kiến trúc và Thiết kế (5 vai trò)
	{Name: "Software Architect", Group: "Architecture and Design", Responsibilities: "System boundaries, module responsibilities, ADRs, dependency direction"},
	{Name: "Solution Architect", Group: "Architecture and Design", Responsibilities: "End-to-end multi-system integration architecture and technology alignment"},
	{Name: "Database Architect", Group: "Architecture and Design", Responsibilities: "Data models, schema normalization, indexing, migration/rollback safety"},
	{Name: "Security Architect", Group: "Architecture and Design", Responsibilities: "Threat modeling, authn/authz design, security boundaries, OWASP controls"},
	{Name: "UX/UI Designer", Group: "Architecture and Design", Responsibilities: "User journeys, interface consistency, accessibility, interaction states"},
	// 3. Khối Phát triển Phần mềm (7 vai trò)
	{Name: "Technical Lead", Group: "Development", Responsibilities: "Technical task breakdown, code quality standards, vertical-slice execution"},
	{Name: "Senior Software Engineer", Group: "Development", Responsibilities: "Core implementation, clean modular code, unit/integration testing"},
	{Name: "Backend Engineer", Group: "Development", Responsibilities: "APIs, domain services, business logic, background workers"},
	{Name: "Frontend Engineer", Group: "Development", Responsibilities: "Web client components, state management, responsive UI"},
	{Name: "Mobile Engineer", Group: "Development", Responsibilities: "iOS/Android/cross-platform mobile client architecture and lifecycle"},
	{Name: "Database Engineer", Group: "Development", Responsibilities: "Queries, transactions, schema migrations, data integrity"},
	{Name: "Integration Engineer", Group: "Development", Responsibilities: "External API adapters, webhooks, retries, contract testing"},
	// 4. Khối Đảm bảo Chất lượng và Bảo mật (5 vai trò)
	{Name: "QA Engineer", Group: "Quality and Security", Responsibilities: "Requirement-based, boundary, negative, and regression test validation"},
	{Name: "Test Automation Engineer", Group: "Quality and Security", Responsibilities: "Automated unit, integration, and end-to-end test suites"},
	{Name: "Security Engineer", Group: "Quality and Security", Responsibilities: "Vulnerability scanning, secret hygiene, security validation reports"},
	{Name: "Performance Engineer", Group: "Quality and Security", Responsibilities: "Latency, throughput, memory profiling, and load validation"},
	{Name: "Independent Reviewer", Group: "Quality and Security", Responsibilities: "Objective critique across completeness, correctness, security, testability"},
	// 5. Khối Vận hành và Độ tin cậy (4 vai trò)
	{Name: "DevOps Engineer", Group: "Operations", Responsibilities: "Build, lint, CI/CD pipelines, reproducible environment configuration"},
	{Name: "Release Manager", Group: "Operations", Responsibilities: "Release readiness, versioning, deployment runbook, rollback verification"},
	{Name: "Site Reliability Engineer", Group: "Operations", Responsibilities: "Health checks, observability, operational resilience, incident recovery"},
	{Name: "Maintenance Engineer", Group: "Operations", Responsibilities: "Root cause analysis, defect triage, safe long-term maintenance"},
	// 6. Khối Tài liệu Kỹ thuật và Trưởng nhóm Kiểm thử (2 vai trò)
	{Name: "Technical Writer", Group: "Documentation", Responsibilities: "README, installation/user/developer/API/troubleshooting guides, changelog"},
	{Name: "QA Lead", Group: "Quality and Security", Responsibilities: "QA sign-off gate, defect severity governance, test evidence integrity"},
}

// AllEngineeringRoles trả về bản sao danh sách toàn bộ 27 vai trò chuyên môn chuẩn của SSDS §4.1.
func AllEngineeringRoles() []RoleDefinition {
	out := make([]RoleDefinition, len(engineeringCatalog))
	copy(out, engineeringCatalog)
	return out
}

// RecommendedReviewerRole gợi ý vai trò phản biện độc lập khác góc nhìn với người tạo deliverable theo SSDS §4.3.
func RecommendedReviewerRole(phaseID string) string {
	switch strings.ToUpper(strings.TrimSpace(phaseID)) {
	case "P1":
		return "System Analyst"
	case "P2":
		return "Security Architect"
	case "P3":
		return "Software Architect"
	case "P5":
		return "QA Engineer"
	case "P6":
		return "Security Engineer"
	case "P7":
		return "QA Lead"
	default:
		return "Independent Reviewer"
	}
}

// ValidateIndependentReview kiểm tra tính trung thực của biên bản phản biện theo SSDS §4.3 và §18 (chống đánh tráo Self-review).
func ValidateIndependentReview(
	phaseID, authorRole, reviewerRole, evidenceMode string,
	hasSeparateReviewerRef bool,
) (normalizedMode string, warning string, err error) {
	mode := strings.ToUpper(strings.TrimSpace(evidenceMode))
	if mode == "" {
		mode = "INDEPENDENT_AI_REVIEW"
	}
	switch mode {
	case "SELF_REVIEW", "INDEPENDENT_AI_REVIEW", "AUTOMATED_VALIDATION", "HUMAN_APPROVAL":
	default:
		return "", "", fmt.Errorf("invalid evidence_mode %q: must be SELF_REVIEW, INDEPENDENT_AI_REVIEW, AUTOMATED_VALIDATION, or HUMAN_APPROVAL", evidenceMode)
	}

	author := strings.TrimSpace(authorRole)
	reviewer := strings.TrimSpace(reviewerRole)

	// Cấm khai báo phản biện độc lập nhưng người duyệt lại trùng vai trò với người tạo sản phẩm!
	if mode != "SELF_REVIEW" && mode != "AUTOMATED_VALIDATION" {
		if author != "" && reviewer != "" && rolesEquivalent(author, reviewer) {
			return "", "", fmt.Errorf(
				"SSDS §4.3 violation on phase %s: reviewer_role %q cannot match author_role %q for independent review",
				phaseID, reviewer, author,
			)
		}
	}

	if mode == "INDEPENDENT_AI_REVIEW" {
		if !hasSeparateReviewerRef {
			return "SELF_REVIEW",
				"SSDS §4.3/§18 limitation disclosure: no separate reviewer/subagent execution context detected; downgraded to SELF_REVIEW. Self-review must not be represented as independent verification.",
				nil
		}
		if reviewer == "" {
			return "", "", fmt.Errorf("SSDS §4.3 violation on phase %s: reviewer_role is required for INDEPENDENT_AI_REVIEW", phaseID)
		}
	}
	return mode, "", nil
}

// rolesEquivalent đối chiếu hai chuỗi tên vai trò không phân biệt hoa thường và chuẩn hóa chức danh tương đương.
func rolesEquivalent(a, b string) bool {
	na := normalizeRoleKey(a)
	nb := normalizeRoleKey(b)
	return na != "" && na == nb
}

// normalizeRoleKey rút gọn các tiền tố cấp bậc (Senior/Principal/Lead) để phát hiện trùng lặp vai trò cốt lõi.
func normalizeRoleKey(r string) string {
	s := strings.ToLower(strings.TrimSpace(r))
	for _, prefix := range []string{"senior ", "principal ", "lead "} {
		s = strings.TrimPrefix(s, prefix)
	}
	return strings.TrimSpace(s)
}

