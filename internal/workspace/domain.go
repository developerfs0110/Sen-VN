// Tệp domain.go quản lý nhận diện chuyên ngành dự án (DomainProfile) và chốt chặn vòng lặp sửa lỗi (Quarantine <= 3 vòng).
package workspace

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/developerfs0110/Sen-VN/internal/storage"
)

const (
	// CurrentVersion định danh phiên bản phát hành chính thức của Sen-VN.
	CurrentVersion = "1.1.0"
	// MaxRepairRounds giới hạn tối đa 3 lần chạy verify thất bại liên tiếp trước khi kích hoạt cách ly (Quarantine).
	MaxRepairRounds = 3
)

// domainMarkers gom nhóm các dấu hiệu tệp tin để phân loại chuyên ngành của workspace.
type domainMarkers struct {
	video bool
	media bool
	web   bool
	code  bool
	docs  bool
}

// ValidateDomainProfile kiểm tra và chuẩn hóa giá trị --profile thuộc tập auto, code, web, video, media, docs.
func ValidateDomainProfile(raw string) (string, error) {
	p := strings.ToLower(strings.TrimSpace(raw))
	if p == "" {
		return "auto", nil
	}
	switch p {
	case "auto", "code", "web", "video", "media", "docs":
		return p, nil
	default:
		return "", fmt.Errorf("invalid --profile %q: must be auto, code, web, video, media, or docs", raw)
	}
}

// ResolveDomainProfile trả về chuyên ngành thực tế của dự án (tự động dò tìm nếu cấu hình đang đặt là auto hoặc rỗng).
func ResolveDomainProfile(root, configured string) string {
	norm := strings.ToLower(strings.TrimSpace(configured))
	switch norm {
	case "code", "web", "video", "media", "docs":
		return norm
	default:
		return DetectDomainProfile(root)
	}
}

// DetectDomainProfile quét nhanh cấu trúc thư mục và phần mở rộng tệp để tự động nhận diện chuyên ngành dự án.
func DetectDomainProfile(root string) string {
	if strings.TrimSpace(root) == "" {
		return "code"
	}
	var m domainMarkers
	scanned := 0
	_ = filepath.WalkDir(root, func(fullPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil || fullPath == root {
			return nil
		}
		nameLower := strings.ToLower(d.Name())
		if d.IsDir() {
			if ignoredDirs[nameLower] {
				return filepath.SkipDir
			}
			if rawMediaDirNames[nameLower] {
				m.video = true
			}
			return nil
		}
		scanned++
		if scanned > 600 {
			return fs.SkipAll
		}
		classifyFileForDomain(nameLower, &m)
		return nil
	})
	switch {
	case m.video && !m.code && !m.web:
		return "video"
	case m.media && !m.code && !m.web:
		return "media"
	case m.web:
		return "web"
	case m.code:
		return "code"
	case m.video:
		return "video"
	case m.docs:
		return "docs"
	default:
		return "code"
	}
}

// classifyFileForDomain cập nhật các cờ nhận diện chuyên ngành dựa trên tên tệp và phần mở rộng.
func classifyFileForDomain(nameLower string, m *domainMarkers) {
	ext := strings.ToLower(filepath.Ext(nameLower))
	switch {
	case nameLower == "index.html" || nameLower == "pubspec.yaml" || nameLower == "angular.json" ||
		strings.HasPrefix(nameLower, "vite.config.") || strings.HasPrefix(nameLower, "next.config.") ||
		strings.HasPrefix(nameLower, "nuxt.config.") || strings.HasPrefix(nameLower, "svelte.config."):
		m.web = true
	case nameLower == "go.mod" || nameLower == "package.json" || nameLower == "pyproject.toml" ||
		nameLower == "cargo.toml" || nameLower == "requirements.txt" || nameLower == "pom.xml":
		m.code = true
	}
	switch ext {
	case ".mp4", ".mov", ".mkv", ".avi", ".webm", ".srt", ".ass", ".prproj", ".drp":
		m.video = true
	case ".psd", ".blend", ".xcf", ".ai", ".wav", ".flac":
		m.media = true
	case ".tsx", ".jsx", ".vue", ".svelte":
		m.web = true
	case ".go", ".py", ".rs", ".cs", ".java", ".ts", ".js", ".cpp", ".c":
		m.code = true
	case ".md", ".mdx", ".docx", ".rst", ".tex", ".pdf":
		m.docs = true
	}
}

// QuarantineResetPath trả về đường dẫn tệp dấu mở khóa vòng lặp sửa lỗi cho một task cụ thể trong .sen/.
func QuarantineResetPath(root, taskID string) string {
	return filepath.Join(root, ".sen", "quarantine_reset_"+taskID)
}

// WriteQuarantineReset ghi mốc seq hiện tại vào .sen/quarantine_reset_<taskID> khi người dùng duyệt mở khóa (--approve).
func WriteQuarantineReset(st *storage.Store, root, taskID string) error {
	if err := ValidateTaskID(taskID); err != nil {
		return err
	}
	maxSeq, err := st.MaxEvidenceSeq()
	if err != nil {
		return err
	}
	p := QuarantineResetPath(root, taskID)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	return os.WriteFile(p, []byte(fmt.Sprintf("seq=%d\n", maxSeq)), 0o644)
}

// ClearQuarantineReset dọn tệp dấu mở khóa sau khi task đã được verify thành công hoặc được hoàn tác (undo).
func ClearQuarantineReset(root, taskID string) {
	if root == "" || taskID == "" {
		return
	}
	_ = os.Remove(QuarantineResetPath(root, taskID))
}

// ActiveFailedVerifyCount trả về số lần verify lỗi liên tiếp trong chu kỳ hiện tại (tính từ sau lần mở khóa gần nhất).
func ActiveFailedVerifyCount(st *storage.Store, root, taskID string) (int, error) {
	if st == nil || taskID == "" {
		return 0, nil
	}
	afterSeq, hasManualToken := readQuarantineResetSeq(root, taskID)
	if hasManualToken {
		return 0, nil
	}
	return st.CountConsecutiveFailedEvidenceSince(taskID, afterSeq)
}

// ConsumeQuarantineResetForAttempt chuẩn hóa tệp dấu mở khóa (nếu được tạo thủ công) sang mốc seq hiện tại trước khi chạy verify.
func ConsumeQuarantineResetForAttempt(st *storage.Store, root, taskID string) (int, error) {
	if st == nil || taskID == "" {
		return 0, nil
	}
	afterSeq, hasManualToken := readQuarantineResetSeq(root, taskID)
	if hasManualToken {
		maxSeq, err := st.MaxEvidenceSeq()
		if err != nil {
			return 0, err
		}
		_ = os.WriteFile(QuarantineResetPath(root, taskID), []byte(fmt.Sprintf("seq=%d\n", maxSeq)), 0o644)
		return 0, nil
	}
	return st.CountConsecutiveFailedEvidenceSince(taskID, afterSeq)
}

// readQuarantineResetSeq đọc mốc seq từ tệp .sen/quarantine_reset_<taskID>; trả về hasManualToken=true nếu tệp không ghi seq.
func readQuarantineResetSeq(root, taskID string) (int64, bool) {
	if root == "" || taskID == "" {
		return 0, false
	}
	raw, err := os.ReadFile(QuarantineResetPath(root, taskID))
	if err != nil {
		return 0, false
	}
	text := strings.TrimSpace(string(raw))
	if strings.HasPrefix(text, "seq=") {
		if n, pErr := strconv.ParseInt(strings.TrimPrefix(text, "seq="), 10, 64); pErr == nil && n >= 0 {
			return n, false
		}
	}
	return 0, true
}

