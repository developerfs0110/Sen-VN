// Package ssds thực thi toàn bộ tiêu chuẩn SEN SOFTWARE DEVELOPMENT STANDARD (SEN-STD-SDLC-001) đa phiên bản bằng Go.
package ssds

import (
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const (
	// StandardID định danh mã chuẩn kỹ thuật phần mềm cấp hệ thống của Sen Core.
	StandardID = "SEN-STD-SDLC-001"
	// BundledLatestVersion là phiên bản ổn định mới nhất được nhúng sẵn trong binary Sen-VN.
	BundledLatestVersion = "1.0.0"
	// ExtraVersionsEnvVar cho phép nạp bổ sung các gói quy chuẩn SSDS thử nghiệm qua biến môi trường khi chạy E2E CLI.
	ExtraVersionsEnvVar = "SEN_SSDS_EXTRA_VERSIONS_DIR"
	// upgradeBackupRelPath là tệp sao lưu trạng thái trước khi nâng cấp phiên bản SSDS trong thư mục .sen/.
	upgradeBackupRelPath = ".sen/ssds_upgrade_backup.json"
)

// embeddedAssets nhúng nhật ký thay đổi chính thức và toàn bộ các gói quy chuẩn theo từng phiên bản.
//
//go:embed SSDS_CHANGELOG.md versions/*
var embeddedAssets embed.FS

var (
	testRulepackMu sync.RWMutex
	testRulepacks  = make(map[string]Rulepack)
)

// PhaseSpec định nghĩa cấu hình máy trạng thái, vai trò, deliverables và mục tiêu đề bắt buộc của một pha SDLC.
type PhaseSpec struct {
	PhaseID                   string              `json:"phase_id"`
	Title                     string              `json:"title"`
	PrimaryRole               string              `json:"primary_role"`
	SupportingRoles           []string            `json:"supporting_roles"`
	RequiredDeliverables      []string            `json:"required_deliverables"`
	RequiredHeadings          map[string][]string `json:"required_headings"`
	MicroDeliverables         []string            `json:"micro_deliverables,omitempty"`
	MicroHeadings             map[string][]string `json:"micro_headings,omitempty"`
	RequiresUserApproval      bool                `json:"requires_user_approval"`
	RequiresIndependentReview bool                `json:"requires_independent_review"`
}

// EffectiveDeliverables trả về danh sách tệp bàn giao bắt buộc tương ứng với bậc quy trình (WF_MICRO hoặc chuẩn).
func (ps PhaseSpec) EffectiveDeliverables(workflowTier string) []string {
	if strings.EqualFold(workflowTier, "WF_MICRO") && ps.MicroDeliverables != nil {
		return ps.MicroDeliverables
	}
	return ps.RequiredDeliverables
}

// EffectiveHeadings trả về bản đồ tiêu đề Markdown bắt buộc theo từng tệp bàn giao tương ứng với bậc quy trình.
func (ps PhaseSpec) EffectiveHeadings(workflowTier string) map[string][]string {
	if strings.EqualFold(workflowTier, "WF_MICRO") && ps.MicroHeadings != nil {
		return ps.MicroHeadings
	}
	return ps.RequiredHeadings
}

// Rulepack lưu toàn bộ quy tắc thực thi được giải mã từ manifest.json của một phiên bản SSDS cụ thể.
type Rulepack struct {
	StandardID          string              `json:"standard_id"`
	Version             string              `json:"version"`
	Classification      string              `json:"classification"`
	Categories          []string            `json:"categories"`
	Workflows           map[string][]string `json:"workflows"`
	RiskDimensions      []string            `json:"risk_dimensions"`
	RequirementPrefixes map[string]string   `json:"requirement_prefixes"`
	PhaseStatuses       []string            `json:"phase_statuses"`
	TaskStates          []string            `json:"task_states"`
	Verdicts            []string            `json:"verdicts"`
	AuthorityLevels     []string            `json:"authority_levels"`
	EvidenceModes       []string            `json:"evidence_modes"`
	Phases              []PhaseSpec         `json:"phases"`
}

// PhaseByID tra cứu cấu hình chi tiết của một pha (P0..P9) bên trong gói quy chuẩn hiện hành.
func (rp *Rulepack) PhaseByID(phaseID string) (*PhaseSpec, bool) {
	if rp == nil {
		return nil, false
	}
	clean := strings.TrimSpace(phaseID)
	for i := range rp.Phases {
		if strings.EqualFold(rp.Phases[i].PhaseID, clean) {
			cp := rp.Phases[i]
			return &cp, true
		}
	}
	return nil, false
}

// cleanVersion chuẩn hóa chuỗi phiên bản SSDS bằng cách cắt khoảng trắng và tiền tố chữ 'v'.
func cleanVersion(ver string) string {
	return strings.TrimPrefix(strings.TrimPrefix(strings.TrimSpace(ver), "v"), "V")
}

// RegisterRulepackForTest đăng ký gói quy tắc giả lập (an toàn luồng) để kiểm thử ghim phiên bản, nâng cấp và hoàn tác.
func RegisterRulepackForTest(rp Rulepack) {
	v := cleanVersion(rp.Version)
	rp.Version = v
	testRulepackMu.Lock()
	defer testRulepackMu.Unlock()
	testRulepacks[v] = rp
}

// UnregisterRulepackForTest gỡ bỏ gói quy tắc giả lập đã đăng ký trong bộ kiểm thử.
func UnregisterRulepackForTest(version string) {
	testRulepackMu.Lock()
	defer testRulepackMu.Unlock()
	delete(testRulepacks, cleanVersion(version))
}

// LoadRulepack nạp cấu hình quy chuẩn của đúng phiên bản SSDS được chỉ định từ bộ nhớ thử nghiệm, thư mục phụ hoặc //go:embed.
func LoadRulepack(version string) (*Rulepack, error) {
	ver := cleanVersion(version)
	if ver == "" {
		ver = BundledLatestVersion
	}
	testRulepackMu.RLock()
	if rp, ok := testRulepacks[ver]; ok {
		testRulepackMu.RUnlock()
		cp := rp
		return &cp, nil
	}
	testRulepackMu.RUnlock()

	if raw, err := embeddedAssets.ReadFile("versions/v" + ver + "/manifest.json"); err == nil {
		var rp Rulepack
		if err := json.Unmarshal(raw, &rp); err != nil {
			return nil, fmt.Errorf("parse SSDS manifest v%s: %w", ver, err)
		}
		return &rp, nil
	}

	if extraDir := strings.TrimSpace(os.Getenv(ExtraVersionsEnvVar)); extraDir != "" {
		extPath := filepath.Join(extraDir, "v"+ver, "manifest.json")
		if rawExt, err := os.ReadFile(extPath); err == nil {
			var rp Rulepack
			if err := json.Unmarshal(rawExt, &rp); err != nil {
				return nil, fmt.Errorf("parse external SSDS manifest v%s: %w", ver, err)
			}
			return &rp, nil
		}
	}

	return nil, fmt.Errorf("SSDS rulepack version %q not found in registry", ver)
}

// LoadStandardMarkdown đọc toàn văn tài liệu chuẩn SEN-STD-SDLC-001 ở định dạng Markdown cho phiên bản tương ứng.
func LoadStandardMarkdown(version string) (string, error) {
	ver := cleanVersion(version)
	if ver == "" {
		ver = BundledLatestVersion
	}
	raw, err := embeddedAssets.ReadFile("versions/v" + ver + "/standard.md")
	if err != nil {
		return "", fmt.Errorf("SSDS standard.md v%s not found: %w", ver, err)
	}
	return string(raw), nil
}

// ListBundledVersions trả về danh sách tất cả phiên bản SSDS khả dụng sắp xếp tăng dần theo ngữ nghĩa SemVer.
func ListBundledVersions() []string {
	seen := make(map[string]bool)
	entries, err := embeddedAssets.ReadDir("versions")
	if err == nil {
		for _, e := range entries {
			if e.IsDir() {
				seen[cleanVersion(e.Name())] = true
			}
		}
	}
	if extraDir := strings.TrimSpace(os.Getenv(ExtraVersionsEnvVar)); extraDir != "" {
		if extEntries, err := os.ReadDir(extraDir); err == nil {
			for _, e := range extEntries {
				if e.IsDir() {
					seen[cleanVersion(e.Name())] = true
				}
			}
		}
	}
	testRulepackMu.RLock()
	for v := range testRulepacks {
		seen[v] = true
	}
	testRulepackMu.RUnlock()

	out := make([]string, 0, len(seen))
	for v := range seen {
		if v != "" {
			out = append(out, v)
		}
	}
	sort.Slice(out, func(i, j int) bool { return CompareSemver(out[i], out[j]) < 0 })
	return out
}

// LatestAvailableVersion trả về phiên bản SSDS cao nhất hiện có trong Registry (mặc định là BundledLatestVersion).
func LatestAvailableVersion() string {
	vers := ListBundledVersions()
	if len(vers) == 0 {
		return BundledLatestVersion
	}
	return vers[len(vers)-1]
}

// ChangelogMarkdown trả về nội dung nhật ký thay đổi chính thức của chuẩn SSDS (SSDS_CHANGELOG.md).
func ChangelogMarkdown() string {
	raw, _ := embeddedAssets.ReadFile("SSDS_CHANGELOG.md")
	return string(raw)
}

// CompareSemver so sánh hai chuỗi phiên bản ngữ nghĩa X.Y.Z (trả về 1 nếu a > b, -1 nếu a < b, 0 nếu bằng nhau).
func CompareSemver(a, b string) int {
	pa := parseSemverTriplet(a)
	pb := parseSemverTriplet(b)
	for i := 0; i < 3; i++ {
		if pa[i] > pb[i] {
			return 1
		}
		if pa[i] < pb[i] {
			return -1
		}
	}
	return 0
}

// parseSemverTriplet tách chuỗi phiên bản thành bộ 3 số nguyên [major, minor, patch].
func parseSemverTriplet(v string) [3]int {
	var out [3]int
	parts := strings.Split(cleanVersion(v), ".")
	for i := 0; i < 3 && i < len(parts); i++ {
		seg := strings.SplitN(parts[i], "-", 2)[0]
		out[i], _ = strconv.Atoi(seg)
	}
	return out
}
