// Package bridge kết nối Sen MVP với hệ sinh thái kỹ năng đồng hành (.agents/), sinh AGENTS.md và xử lý JSON hook Antigravity.
package bridge

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"strings"

	"github.com/developerfs0110/Sen-VN/internal/storage"
)

// CompanionStatus ghi nhận trạng thái hiện diện của các gói kỹ năng bổ trợ trong cây thư mục .agents/.
type CompanionStatus struct {
	Superpowers       bool
	SuperpowersPath   string
	SecurityAudit     bool
	SecurityAuditPath string
	VnLawCheck        bool
	VnLawCheckPath    string
	AntigravityEnv    bool
}

// dirExists kiểm tra đường dẫn tồn tại và là một thư mục hợp lệ.
func dirExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// DetectCompanions dò ngược tối đa 3 cấp thư mục cha từ root để phát hiện superpowers, security-audit và vn-law-check.
func DetectCompanions(root string) CompanionStatus {
	var st CompanionStatus
	cur, err := filepath.Abs(root)
	if err != nil {
		cur = root
	}
	for depth := 0; depth <= 3 && cur != ""; depth++ {
		agentsDir := filepath.Join(cur, ".agents")
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
			sa := filepath.Join(agentsDir, "skills", "security-audit")
			if dirExists(sa) {
				st.SecurityAudit = true
				st.SecurityAuditPath = sa
			}
		}
		if !st.VnLawCheck {
			vn := filepath.Join(agentsDir, "skills", "vn-law-check")
			if dirExists(vn) {
				st.VnLawCheck = true
				st.VnLawCheckPath = vn
			}
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}
	return st
}

// FormatCompact định dạng ngắn gọn trạng thái bật/tắt của 3 gói kỹ năng đồng hành trên một dòng.
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

// FormatBrief kết xuất bản tóm tắt ngữ cảnh siêu gọn (<250 token) gồm kỹ năng đồng hành, cảnh báo OPEN, luật và R_i còn thiếu.
func FormatBrief(st *storage.Store, root, activePath string) string {
	comp := DetectCompanions(root)
	var b strings.Builder
	b.WriteString(fmt.Sprintf("[SEN BRIEF] companions(%s)\n", comp.FormatCompact()))
	if st == nil {
		b.WriteString("status: uninitialized (run `sen init`)\n")
		return b.String()
	}

	tasks, _ := st.ListTasks()
	verifiedTasks := make(map[string]bool)
	for _, t := range tasks {
		if t.Status == "OPEN" {
			b.WriteString(fmt.Sprintf("WARNING: Interrupted OPEN task %s (%s, risk=%s)! Run `sen undo %s` to restore clean files or `sen verify %s -- <cmd>`.\n",
				t.ID, t.Kind, t.RiskAuto, t.ID, t.ID))
		}
		if t.Status == "VERIFIED" {
			for _, r := range t.ReqIDs {
				verifiedTasks[r] = true
			}
		}
	}

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
