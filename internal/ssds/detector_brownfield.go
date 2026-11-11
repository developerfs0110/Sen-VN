// Tệp detector_brownfield.go khảo sát hiện trạng kho mã nguồn có sẵn (§2.3) và tôn trọng ranh giới cô lập dự án con (.git/.sen).
package ssds

import (
	"encoding/json"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/developerfs0110/Sen-VN/internal/storage"
	"github.com/developerfs0110/Sen-VN/internal/workspace"
)

// BrownfieldAudit lưu báo cáo khảo sát hiện trạng kho mã nguồn có sẵn theo đúng 8 hạng mục của SSDS §2.3.
type BrownfieldAudit struct {
	ProjectRoot   string   `json:"project_root"`
	TopDirs       []string `json:"top_dirs"`
	Languages     []string `json:"languages"`
	Manifests     []string `json:"manifests"`
	ExistingDocs  []string `json:"existing_docs"`
	ExistingADRs  []string `json:"existing_adrs"`
	SourceFiles   int      `json:"source_files"`
	TestFiles     int      `json:"test_files"`
	HasGit        bool     `json:"has_git"`
	GitDirtyFiles int      `json:"git_dirty_files"`
	AuditedAt     string   `json:"audited_at"`
}

// InspectBrownfieldRepo quét cấu trúc repo hiện hữu theo SSDS §2.3, lưu tóm tắt vào .sen/brownfield_audit.json và đánh dấu SetBrownfieldAudited(true).
func InspectBrownfieldRepo(projectRoot string) (*BrownfieldAudit, error) {
	audit, err := scanBrownfieldLayout(projectRoot)
	if err != nil {
		return nil, err
	}
	senDir := filepath.Join(projectRoot, ".sen")
	if info, sErr := os.Stat(senDir); sErr == nil && info.IsDir() {
		raw, _ := json.MarshalIndent(audit, "", "  ")
		_ = os.WriteFile(filepath.Join(senDir, "brownfield_audit.json"), raw, 0o644)
	}
	dbPath := filepath.Join(senDir, "state.db")
	if _, sErr := os.Stat(dbPath); sErr == nil {
		if st, oErr := storage.Open(dbPath); oErr == nil {
			_ = st.SetBrownfieldAudited(true)
			_ = st.Close()
		}
	}
	return audit, nil
}

// AuditBrownfieldProject thực hiện khảo sát Brownfield §2.3 trực tiếp trên kết nối Store đang mở mà không cần mở kết nối thứ hai.
func AuditBrownfieldProject(st *storage.Store, projectRoot string) (*BrownfieldAudit, error) {
	audit, err := scanBrownfieldLayout(projectRoot)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(projectRoot) != "" {
		senDir := filepath.Join(projectRoot, ".sen")
		_ = os.MkdirAll(senDir, 0o755)
		raw, _ := json.MarshalIndent(audit, "", "  ")
		_ = os.WriteFile(filepath.Join(senDir, "brownfield_audit.json"), raw, 0o644)
	}
	if st != nil {
		_ = st.SetBrownfieldAudited(true)
	}
	return audit, nil
}

// scanBrownfieldLayout duyệt cây dự án (bỏ qua các thư mục con có .git hoặc .sen riêng) để thu thập ngôn ngữ, manifests, docs, ADRs và Git.
func scanBrownfieldLayout(projectRoot string) (*BrownfieldAudit, error) {
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return nil, err
	}
	audit := &BrownfieldAudit{
		ProjectRoot: absRoot,
		AuditedAt:   time.Now().UTC().Format(time.RFC3339),
	}
	if _, gErr := os.Lstat(filepath.Join(absRoot, ".git")); gErr == nil {
		audit.HasGit = true
		if out, cErr := exec.Command("git", "-C", absRoot, "status", "--porcelain").Output(); cErr == nil {
			trimmed := strings.TrimSpace(string(out))
			if trimmed != "" {
				audit.GitDirtyFiles = len(strings.Split(trimmed, "\n"))
			}
		}
	}

	langSet := make(map[string]bool)
	topEntries, _ := os.ReadDir(absRoot)
	for _, e := range topEntries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") && !hasNestedChildBoundary(filepath.Join(absRoot, e.Name())) {
			audit.TopDirs = append(audit.TopDirs, e.Name())
		}
	}
	_ = filepath.WalkDir(absRoot, func(full string, d fs.DirEntry, wErr error) error {
		if wErr != nil || full == absRoot {
			return nil
		}
		nameLower := strings.ToLower(d.Name())
		if d.IsDir() {
			if isSkippedScanDir(nameLower) || hasNestedChildBoundary(full) {
				return filepath.SkipDir
			}
			return nil
		}
		rel, _ := filepath.Rel(absRoot, full)
		slash := filepath.ToSlash(rel)
		if workspace.IsTestFile(slash) {
			audit.TestFiles++
		}
		if workspace.IsSoftwareSourceOrManifestPath(slash) {
			ext := strings.ToLower(filepath.Ext(slash))
			if ext == "" || nameLower == "go.mod" || nameLower == "package.json" || nameLower == "pyproject.toml" || nameLower == "cargo.toml" || nameLower == "pubspec.yaml" || nameLower == "pom.xml" || nameLower == "requirements.txt" {
				audit.Manifests = append(audit.Manifests, slash)
			} else {
				audit.SourceFiles++
				langSet[strings.TrimPrefix(ext, ".")] = true
			}
		}
		if strings.HasSuffix(nameLower, ".md") {
			if strings.Contains(strings.ToLower(slash), "adr/") || strings.HasPrefix(nameLower, "adr-") {
				audit.ExistingADRs = append(audit.ExistingADRs, slash)
			} else {
				audit.ExistingDocs = append(audit.ExistingDocs, slash)
			}
		}
		return nil
	})
	for l := range langSet {
		audit.Languages = append(audit.Languages, l)
	}
	sort.Strings(audit.Languages)
	return audit, nil
}

// quickInspectSoftwareFootprint kiểm tra nhanh thư mục có sẵn tệp mã nguồn/manifest của chính dự án đó hay không (bỏ qua dự án con .git/.sen).
func quickInspectSoftwareFootprint(projectRoot string) (bool, string) {
	if strings.TrimSpace(projectRoot) == "" {
		return false, "code"
	}
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		absRoot = projectRoot
	}
	hasCode := false
	scanned := 0
	_ = filepath.WalkDir(absRoot, func(full string, d fs.DirEntry, wErr error) error {
		if wErr != nil || full == absRoot {
			return nil
		}
		nameLower := strings.ToLower(d.Name())
		if d.IsDir() {
			if isSkippedScanDir(nameLower) || hasNestedChildBoundary(full) {
				return filepath.SkipDir
			}
			return nil
		}
		scanned++
		if scanned > 300 {
			return fs.SkipAll
		}
		rel, _ := filepath.Rel(absRoot, full)
		if workspace.IsSoftwareSourceOrManifestPath(filepath.ToSlash(rel)) {
			hasCode = true
			return fs.SkipAll
		}
		return nil
	})
	return hasCode, workspace.DetectDomainProfile(projectRoot)
}

// isSkippedScanDir nhận diện các thư mục hệ thống/phụ thuộc cần bỏ qua khi quét mã nguồn dự án.
func isSkippedScanDir(nameLower string) bool {
	switch nameLower {
	case ".git", ".sen", ".agents", "node_modules", ".venv", "dist", "build", "__pycache__":
		return true
	default:
		return false
	}
}

// hasNestedChildBoundary kiểm tra xem thư mục con có chứa ranh giới dự án độc lập (.sen hoặc .git) hay không.
func hasNestedChildBoundary(dirAbs string) bool {
	if _, err := os.Lstat(filepath.Join(dirAbs, ".sen")); err == nil {
		return true
	}
	if _, err := os.Lstat(filepath.Join(dirAbs, ".git")); err == nil {
		return true
	}
	return false
}

