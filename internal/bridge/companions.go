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

// runtimeConfigView ánh xạ các trường cấu hình vận hành trong .sen/config.json cần thiết cho hook và brief.
type runtimeConfigView struct {
	Version          string   `json:"version"`
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

// ReadVerifiedRuntimeConfig đọc .sen/config.json và đối chiếu chữ ký .sen/config.sha256 để chống Agent bật lén express_mode.
func ReadVerifiedRuntimeConfig(root string) (bool, string, bool) {
	defaultProf := workspace.DetectDomainProfile(root)
	if strings.TrimSpace(root) == "" {
		return false, defaultProf, false
	}
	cfgPath := filepath.Join(root, ".sen", "config.json")
	raw, err := os.ReadFile(cfgPath)
	if err != nil || len(raw) == 0 {
		return false, defaultProf, false
	}
	sumBytes, sErr := os.ReadFile(filepath.Join(root, ".sen", "config.sha256"))
	if sErr != nil {
		return false, defaultProf, true
	}
	sum := sha256.Sum256(raw)
	actualHex := hex.EncodeToString(sum[:])
	if strings.TrimSpace(string(sumBytes)) != actualHex {
		return false, defaultProf, true
	}
	var cfg runtimeConfigView
	if jErr := json.Unmarshal(raw, &cfg); jErr != nil {
		return false, defaultProf, true
	}
	return cfg.ExpressMode, workspace.ResolveDomainProfile(root, cfg.DomainProfile), false
}

// WriteSignedRuntimeConfig cập nhật express_mode và domain_profile vào .sen/config.json kèm chữ ký SHA-256 hợp lệ.
func WriteSignedRuntimeConfig(root string, expressMode bool, domainProfile string) error {
	cfgPath := filepath.Join(root, ".sen", "config.json")
	cfg := runtimeConfigView{
		Version:        "1.1",
		TimeoutSeconds: 120,
		DomainProfile:  "auto",
	}
	if raw, err := os.ReadFile(cfgPath); err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &cfg)
	}
	cfg.Version = "1.1"
	cfg.ExpressMode = expressMode
	if strings.TrimSpace(domainProfile) != "" {
		cfg.DomainProfile = strings.ToLower(strings.TrimSpace(domainProfile))
	}
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

// appendTaskBriefWarnings ghi cảnh báo task OPEN bị gián đoạn hoặc chạm ngưỡng cách ly QUARANTINE (3/3 failed verifies).
func appendTaskBriefWarnings(b *strings.Builder, st *storage.Store, root string) map[string]bool {
	verified := make(map[string]bool)
	tasks, _ := st.ListTasks()
	for _, t := range tasks {
		if t.Status == "OPEN" {
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
		}
		if t.Status == "VERIFIED" {
			for _, r := range t.ReqIDs {
				verified[r] = true
			}
		}
	}
	return verified
}
