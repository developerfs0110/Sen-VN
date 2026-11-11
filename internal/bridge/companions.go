// Package bridge kết nối Sen-VN với hệ sinh thái kỹ năng (.agents/), quản lý gói đồng hành và xử lý JSON hook Antigravity.
package bridge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/developerfs0110/Sen-VN/internal/ssds"
	"github.com/developerfs0110/Sen-VN/internal/storage"
	"github.com/developerfs0110/Sen-VN/internal/workspace"
)

// CompanionStatus ghi nhận trạng thái hiện diện của các gói kỹ năng nền tảng trong cây thư mục .agents/.
type CompanionStatus struct {
	Superpowers       bool
	SuperpowersPath   string
	SecurityAudit     bool
	SecurityAuditPath string
	VnLawCheck        bool
	VnLawCheckPath    string
	Dialectical       bool
	SkillCapture      bool
	AntigravityEnv    bool
}

// runtimeConfigView ánh xạ các trường cấu hình vận hành trong .sen/config.json cần thiết cho hook, SSDS và brief.
type runtimeConfigView struct {
	Version          string   `json:"version"`
	SSDSVersion      string   `json:"ssds_version,omitempty"`
	ProjectID        string   `json:"project_id,omitempty"`
	AllowedBinaries  []string `json:"allowed_binaries,omitempty"`
	ProjectVerifyCmd []string `json:"project_verify_cmd,omitempty"`
	TimeoutSeconds   int      `json:"timeout_seconds,omitempty"`
	ExpressMode      bool     `json:"express_mode"`
	DomainProfile    string   `json:"domain_profile,omitempty"`
}

// FormatCompact định dạng ngắn gọn trạng thái bật/tắt của 3 gói kỹ năng đồng hành cốt lõi trên một dòng.
func (c CompanionStatus) FormatCompact() string {
	yn := func(b bool) string {
		if b {
			return "on"
		}
		return "off"
	}
	return fmt.Sprintf("superpowers=%s security-audit=%s vn-law-check=%s",
		yn(c.Superpowers), yn(c.SecurityAudit), yn(c.VnLawCheck))
}

// dirExists kiểm tra đường dẫn tồn tại và là một thư mục hợp lệ.
func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// DetectCompanions dò ngược tối đa 3 cấp thư mục cha từ root để phát hiện superpowers, security-audit, vn-law-check và kỹ năng nội bộ.
func DetectCompanions(root string) CompanionStatus {
	var st CompanionStatus
	cur, err := filepath.Abs(root)
	if err != nil {
		cur = root
	}
	for depth := 0; depth <= 3 && cur != ""; depth++ {
		scanAgentsDirAtLevel(filepath.Join(cur, ".agents"), &st)
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return st
}

// scanAgentsDirAtLevel kiểm tra sự tồn tại của từng gói kỹ năng trong một thư mục .agents cụ thể.
func scanAgentsDirAtLevel(agentsDir string, st *CompanionStatus) {
	if dirExists(agentsDir) {
		st.AntigravityEnv = true
	}
	if !st.Superpowers {
		sp1 := filepath.Join(agentsDir, "plugins", "superpowers")
		sp2 := filepath.Join(agentsDir, "skills", "brainstorming")
		if dirExists(sp1) {
			st.Superpowers = true
			st.SuperpowersPath = sp1
		} else if dirExists(sp2) {
			st.Superpowers = true
			st.SuperpowersPath = sp2
		}
	}
	if !st.SecurityAudit {
		if sa := filepath.Join(agentsDir, "skills", "security-audit"); dirExists(sa) {
			st.SecurityAudit = true
			st.SecurityAuditPath = sa
		}
	}
	if !st.VnLawCheck {
		if vn := filepath.Join(agentsDir, "skills", "vn-law-check"); dirExists(vn) {
			st.VnLawCheck = true
			st.VnLawCheckPath = vn
		}
	}
	if dirExists(filepath.Join(agentsDir, "skills", "sen-dialectical")) {
		st.Dialectical = true
	}
	if dirExists(filepath.Join(agentsDir, "skills", "sen-skill-capture")) {
		st.SkillCapture = true
	}
}

// verifyExistingSignedConfig kiểm tra nghiêm ngặt tính toàn vẹn cặp tệp .sen/config.json và .sen/config.sha256 (bắt cả lỗi xóa/rỗng).
func verifyExistingSignedConfig(root string) (*runtimeConfigView, bool, error) {
	cfgPath := filepath.Join(root, ".sen", "config.json")
	sumPath := filepath.Join(root, ".sen", "config.sha256")
	_, cfgErr := os.Stat(cfgPath)
	_, sumErr := os.Stat(sumPath)
	if os.IsNotExist(cfgErr) && os.IsNotExist(sumErr) {
		return nil, false, nil
	}
	raw, err := os.ReadFile(cfgPath)
	if err != nil || len(raw) == 0 {
		return nil, true, fmt.Errorf(".sen/config.json missing or empty while signature exists (tampering detected)")
	}
	sumBytes, sErr := os.ReadFile(sumPath)
	if sErr != nil {
		return nil, true, fmt.Errorf(".sen/config.sha256 missing (tampering detected)")
	}
	sum := sha256.Sum256(raw)
	if strings.TrimSpace(string(sumBytes)) != hex.EncodeToString(sum[:]) {
		return nil, true, fmt.Errorf(".sen/config.json integrity hash mismatch (tampering detected)")
	}
	var cfg runtimeConfigView
	if jErr := json.Unmarshal(raw, &cfg); jErr != nil {
		return nil, true, fmt.Errorf("invalid .sen/config.json: %w", jErr)
	}
	return &cfg, true, nil
}

// ReadVerifiedRuntimeConfig đọc .sen/config.json và đối chiếu chữ ký .sen/config.sha256 để chống Agent sửa/xóa trộm cấu hình.
func ReadVerifiedRuntimeConfig(root string) (bool, string, bool) {
	defaultProf := workspace.DetectDomainProfile(root)
	if strings.TrimSpace(root) == "" {
		return false, defaultProf, false
	}
	cfg, exists, err := verifyExistingSignedConfig(root)
	if err != nil {
		return false, defaultProf, true
	}
	if !exists || cfg == nil {
		return false, defaultProf, false
	}
	return cfg.ExpressMode, workspace.ResolveDomainProfile(root, cfg.DomainProfile), false
}

// WriteSignedRuntimeConfig cập nhật express_mode và domain_profile vào .sen/config.json (không kế thừa dữ liệu từ tệp cấu hình bị sửa trộm).
func WriteSignedRuntimeConfig(root string, expressMode bool, domainProfile string) error {
	existing, hasExisting, err := verifyExistingSignedConfig(root)
	cfg := runtimeConfigView{
		Version:        "1.2",
		TimeoutSeconds: 120,
		DomainProfile:  "auto",
	}
	if err == nil && hasExisting && existing != nil {
		cfg = *existing
	} else if st, oErr := storage.Open(filepath.Join(root, ".sen", "state.db")); oErr == nil {
		if proj, _ := st.GetSSDSProject(); proj != nil {
			cfg.ProjectID = proj.ProjectID
			cfg.SSDSVersion = proj.AppliedSSDSVersion
		}
		_ = st.Close()
	}
	if cfg.Version == "" || cfg.Version == "1.0" || cfg.Version == "1.1" {
		cfg.Version = "1.2"
	}
	cfg.ExpressMode = expressMode
	if strings.TrimSpace(domainProfile) != "" {
		cfg.DomainProfile = strings.ToLower(strings.TrimSpace(domainProfile))
	}
	return writeAndSignConfigView(root, cfg)
}

// WriteSignedSSDSConfig cập nhật project_id và ssds_version vào .sen/config.json (không kế thừa dữ liệu từ tệp cấu hình bị sửa trộm).
func WriteSignedSSDSConfig(root, projectID, ssdsVersion string, expressMode *bool) error {
	existing, hasExisting, err := verifyExistingSignedConfig(root)
	cfg := runtimeConfigView{
		Version:        "1.2",
		TimeoutSeconds: 120,
		DomainProfile:  "auto",
	}
	if err == nil && hasExisting && existing != nil {
		cfg = *existing
	}
	cfg.Version = "1.2"
	if strings.TrimSpace(projectID) != "" {
		cfg.ProjectID = strings.TrimSpace(projectID)
	}
	if strings.TrimSpace(ssdsVersion) != "" {
		cfg.SSDSVersion = strings.TrimSpace(ssdsVersion)
	}
	if expressMode != nil {
		cfg.ExpressMode = *expressMode
	}
	return writeAndSignConfigView(root, cfg)
}

// writeAndSignConfigView ghi tệp .sen/config.json và xuất chữ ký .sen/config.sha256 tương ứng.
func writeAndSignConfigView(root string, cfg runtimeConfigView) error {
	cfgPath := filepath.Join(root, ".sen", "config.json")
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(cfgPath, data, 0o644); err != nil {
		return err
	}
	sum := sha256.Sum256(data)
	return os.WriteFile(filepath.Join(root, ".sen", "config.sha256"), []byte(hex.EncodeToString(sum[:])+"\n"), 0o644)
}

// MatchRuleGlob kiểm tra xem đường dẫn tệp đang xét có khớp với mẫu path_glob của một luật dự án hay không.
func MatchRuleGlob(glob, targetPath string) bool {
	glob = strings.TrimSpace(glob)
	if glob == "" || glob == "*" || glob == "**" || targetPath == "" {
		return true
	}
	slash := filepath.ToSlash(targetPath)
	if ok, err := path.Match(glob, slash); err == nil && ok {
		return true
	}
	if ok, err := path.Match(glob, path.Base(slash)); err == nil && ok {
		return true
	}
	prefix := strings.TrimSuffix(glob, "/*")
	prefix = strings.TrimSuffix(prefix, "/**")
	if prefix != glob && strings.HasPrefix(slash, prefix+"/") {
		return true
	}
	return false
}

// FormatBrief kết xuất bản tóm tắt ngữ cảnh siêu gọn (<250 token) gồm profile, express, kỹ năng đồng hành, QUARANTINE và R_i.
func FormatBrief(st *storage.Store, root, activePath string) string {
	comp := DetectCompanions(root)
	express, prof, _ := ReadVerifiedRuntimeConfig(root)
	expLabel := "off"
	if express {
		expLabel = "on"
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("[SEN BRIEF] profile=%s express=%s companions(%s)\n", prof, expLabel, comp.FormatCompact()))
	if st == nil {
		b.WriteString("status: uninitialized (run `sen init`)\n")
		return b.String()
	}

	if proj, _ := st.GetSSDSProject(); proj != nil {
		phStatus := "IN_PROGRESS"
		primaryRole := "Technical Lead"
		if ph, pErr := st.GetSSDSPhase(proj.CurrentPhase); pErr == nil && ph != nil {
			phStatus = ph.Status
			primaryRole = ph.PrimaryRole
		}
		fmt.Fprintf(&b, "[SSDS v%s | cat=%s | wf=%s | phase=%s:%s | role=%s]\n",
			proj.AppliedSSDSVersion, proj.Category, proj.WorkflowTier, proj.CurrentPhase, phStatus, primaryRole)
		latestVer := ssds.LatestAvailableVersion()
		if ssds.CompareSemver(latestVer, proj.AppliedSSDSVersion) > 0 {
			fmt.Fprintf(&b, "ssds_notice: pinned v%s (v%s available — run 'sen sdlc version --check %s')\n",
				proj.AppliedSSDSVersion, latestVer, latestVer)
		}
	}

	verifiedTasks := appendTaskBriefWarnings(&b, st, root)
	rules, _ := st.ListRules()
	if len(rules) > 0 {
		b.WriteString(fmt.Sprintf("rules (%d/%d):\n", len(rules), storage.MaxRules))
		for _, r := range rules {
			if MatchRuleGlob(r.PathGlob, activePath) {
				b.WriteString(fmt.Sprintf("  #%d [%s] %s\n", r.ID, r.PathGlob, r.RuleText))
			}
		}
	}

	reqs, _ := st.ListRequirements()
	var pending []string
	for _, r := range reqs {
		if !verifiedTasks[r.ID] {
			pending = append(pending, fmt.Sprintf("%s:%s", r.ID, r.Summary))
		}
	}
	if len(pending) > 0 {
		b.WriteString("pending_reqs: " + strings.Join(pending, " | ") + "\n")
	} else if len(reqs) > 0 {
		b.WriteString(fmt.Sprintf("reqs: %d/%d verified (ready for `sen done`)\n", len(reqs), len(reqs)))
	} else {
		b.WriteString("reqs: none (capture user intent via `sen intake --quote ... --req ...`)\n")
	}
	return b.String()
}

// appendTaskBriefWarnings ghi cảnh báo task OPEN bị gián đoạn, trạng thái QUARANTINE (3/3) và gợi ý task kế tiếp.
func appendTaskBriefWarnings(b *strings.Builder, st *storage.Store, root string) map[string]bool {
	verified := make(map[string]bool)
	tasks, _ := st.ListTasks()
	nextTask := ""
	for _, t := range tasks {
		if t.Status == "OPEN" {
			if nextTask == "" {
				nextTask = fmt.Sprintf("%s (%s)", t.ID, t.Title)
			}
			failed, _ := workspace.ActiveFailedVerifyCount(st, root, t.ID)
			if failed >= workspace.MaxRepairRounds {
				b.WriteString(fmt.Sprintf(
					"QUARANTINE (3/3 failed verifies) on task %s! Stop blind retries and report objective dialectical root-cause to user (unlock via `sen status --approve` or rollback via `sen undo %s`).\n",
					t.ID, t.ID,
				))
			} else {
				b.WriteString(fmt.Sprintf(
					"WARNING: Interrupted OPEN task %s (%s, risk=%s)! Run `sen undo %s` to restore clean files or `sen verify %s -- <cmd>`.\n",
					t.ID, t.Kind, t.RiskAuto, t.ID, t.ID,
				))
			}
		} else if t.Status != "UNDONE" && t.Status != "VERIFIED" && nextTask == "" {
			nextTask = fmt.Sprintf("%s (%s)", t.ID, t.Title)
		}
		if t.Status == "VERIFIED" {
			for _, r := range t.ReqIDs {
				verified[r] = true
			}
		}
	}
	if nextTask != "" {
		fmt.Fprintf(b, "next_task: %s\n", nextTask)
	}
	return verified
}
