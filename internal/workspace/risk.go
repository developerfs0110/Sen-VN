// Package workspace quản lý kiểm tra an toàn đường dẫn, đánh giá rủi ro tự động, sao lưu snapshot và hoàn tác từng byte.
package workspace

import (
	"errors"
	"fmt"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// reValidTaskID chỉ cho phép chữ cái, chữ số và các ký tự . _ - để triệt tiêu nguy cơ leo thang thư mục (path traversal).
var reValidTaskID = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`)

// exactHighRiskFiles liệt kê các tệp cấu hình/khóa phụ thuộc luôn được phân loại rủi ro mức HIGH.
var exactHighRiskFiles = map[string]bool{
	"go.mod":            true,
	"go.sum":            true,
	"package.json":      true,
	"package-lock.json": true,
	"pnpm-lock.yaml":    true,
	"cargo.toml":        true,
	"pyproject.toml":    true,
	"requirements.txt":  true,
	"dockerfile":        true,
}

// highRiskTokens nhận diện các phân hệ nhạy cảm (xác thực, thanh toán, cơ sở dữ liệu, cấu hình) trong đường dẫn tệp.
var highRiskTokens = []string{
	"auth",
	"config",
	"schema",
	"migration",
	"secret",
	"credential",
	"payment",
}

// ValidateTaskID kiểm tra nghiêm ngặt khuôn dạng mã task và chặn các mã dành riêng của hệ thống.
func ValidateTaskID(taskID string) error {
	trimmed := strings.TrimSpace(taskID)
	if trimmed != taskID || trimmed == "" {
		return errors.New("task_id cannot be empty or padded with whitespace")
	}
	if strings.EqualFold(trimmed, BaselineTaskID) || strings.EqualFold(trimmed, "__DONE__") {
		return fmt.Errorf("task_id %q is reserved", taskID)
	}
	if !reValidTaskID.MatchString(trimmed) {
		return fmt.Errorf("invalid task_id %q: must match ^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$", taskID)
	}
	return nil
}

// ValidateKind chuẩn hóa và chỉ chấp nhận 3 phân loại công việc hợp lệ: FEATURE, BUGFIX hoặc UI.
func ValidateKind(raw string) (string, error) {
	trimmed := strings.ToUpper(strings.TrimSpace(raw))
	switch trimmed {
	case "", "FEATURE", "BUGFIX", "UI":
		return trimmed, nil
	default:
		return "", fmt.Errorf("invalid --kind %q: must be feature, bugfix, or ui", raw)
	}
}

// NormalizeRelPath chuẩn hóa đường dẫn về dạng tương đối dùng dấu gạch chéo xuôi (/), chặn thoát khỏi workspace và NTFS ADS.
func NormalizeRelPath(root, p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", errors.New("empty file path")
	}
	var slash string
	if filepath.IsAbs(p) {
		if root == "" {
			return "", fmt.Errorf("absolute path %q cannot be normalized without workspace root", p)
		}
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return "", err
		}
		absTarget, err := filepath.Abs(p)
		if err != nil {
			return "", err
		}
		rel, err := filepath.Rel(absRoot, absTarget)
		if err != nil {
			return "", fmt.Errorf("path %q escapes workspace root: %w", p, err)
		}
		if filepath.VolumeName(rel) != "" || strings.Contains(rel, ":") {
			return "", fmt.Errorf("path %q crosses volume boundary", p)
		}
		slash = filepath.ToSlash(rel)
	} else {
		// Chặn đường dẫn tương đối gắn ổ đĩa Windows (C:..) và luồng dữ liệu phụ NTFS (:ads).
		if filepath.VolumeName(p) != "" || strings.Contains(p, ":") {
			return "", fmt.Errorf("drive-relative or alternate-data-stream path %q is forbidden", p)
		}
		for _, seg := range strings.FieldsFunc(p, func(r rune) bool { return r == '/' || r == '\\' }) {
			if seg == ".." {
				return "", fmt.Errorf("path traversal %q is forbidden", p)
			}
		}
		cleaned := filepath.Clean(filepath.FromSlash(strings.ReplaceAll(p, "\\", "/")))
		slash = strings.TrimPrefix(filepath.ToSlash(cleaned), "./")
	}

	if slash == "" || slash == "." || slash == ".." || strings.HasPrefix(slash, "../") || strings.Contains(slash, "/../") {
		return "", fmt.Errorf("path %q escapes workspace root", p)
	}
	if root != "" {
		absRoot, err := filepath.Abs(root)
		if err != nil {
			return "", err
		}
		joined := filepath.Join(absRoot, filepath.FromSlash(slash))
		relCheck, err := filepath.Rel(absRoot, joined)
		if err != nil || relCheck == "." || relCheck == ".." || strings.HasPrefix(filepath.ToSlash(relCheck), "../") {
			return "", fmt.Errorf("path %q escapes workspace root", p)
		}
	}
	return slash, nil
}

// PathKey sinh khóa so sánh đường dẫn chuẩn (không phân biệt hoa thường trên hệ thống tệp Windows NTFS).
func PathKey(relPath string) string {
	slash := filepath.ToSlash(relPath)
	if runtime.GOOS == "windows" {
		return strings.ToLower(slash)
	}
	return slash
}

// IsTestFile nhận diện tệp kiểm thử tự động theo quy ước đặt tên phổ biến của Go, Python, JS/TS và PowerShell.
func IsTestFile(relPath string) bool {
	base := strings.ToLower(filepath.Base(filepath.FromSlash(relPath)))
	switch {
	case strings.HasSuffix(base, "_test.go"):
		return true
	case strings.HasPrefix(base, "test_") && strings.HasSuffix(base, ".py"):
		return true
	case strings.HasSuffix(base, "_test.py"):
		return true
	case strings.HasSuffix(base, ".test.ts"), strings.HasSuffix(base, ".test.tsx"),
		strings.HasSuffix(base, ".test.js"), strings.HasSuffix(base, ".test.jsx"):
		return true
	case strings.HasSuffix(base, ".spec.ts"), strings.HasSuffix(base, ".spec.tsx"),
		strings.HasSuffix(base, ".spec.js"), strings.HasSuffix(base, ".spec.jsx"):
		return true
	case strings.HasSuffix(base, ".tests.ps1"):
		return true
	default:
		return false
	}
}

// ComputeRisk tính mức rủi ro tất định (LOW, MEDIUM, HIGH) cho danh sách tệp phạm vi không kèm cờ xóa.
func ComputeRisk(files []string, kind string, workspaceRoot string) (string, []string) {
	return ComputeRiskWithOpts(files, kind, false, workspaceRoot)
}

// ComputeRiskWithOpts phân loại rủi ro tất định dựa trên loại task, quyền xóa tệp, số lượng tệp và đường dẫn nhạy cảm.
func ComputeRiskWithOpts(files []string, kind string, allowDelete bool, workspaceRoot string) (string, []string) {
	var reasons []string
	if strings.EqualFold(strings.TrimSpace(kind), "BUGFIX") {
		reasons = append(reasons, "kind=BUGFIX")
	}
	if allowDelete {
		reasons = append(reasons, "file deletion declared (--allow-delete)")
	}
	if len(files) > 5 {
		reasons = append(reasons, fmt.Sprintf("scope_files=%d (>5)", len(files)))
	}
	for _, raw := range files {
		if reason := fileHighRiskReason(workspaceRoot, raw); reason != "" {
			reasons = append(reasons, reason)
		}
	}
	if len(reasons) > 0 {
		return "HIGH", reasons
	}
	if len(files) >= 3 {
		return "MEDIUM", []string{fmt.Sprintf("scope_files=%d (>=3)", len(files))}
	}
	return "LOW", nil
}

// fileHighRiskReason trả về lý do nếu đường dẫn chạm tới tệp cấu hình, CI/CD, SQL hoặc phân hệ trọng yếu.
func fileHighRiskReason(workspaceRoot, raw string) string {
	rel, err := NormalizeRelPath(workspaceRoot, raw)
	if err != nil {
		rel = filepath.ToSlash(raw)
	}
	lower := strings.ToLower(rel)
	base := strings.ToLower(filepath.Base(rel))
	if exactHighRiskFiles[base] || strings.HasPrefix(base, ".env") || strings.HasPrefix(base, "docker-compose") {
		return fmt.Sprintf("sensitive file %s", rel)
	}
	if strings.HasPrefix(lower, ".github/") || strings.HasPrefix(lower, ".gitlab/") || strings.HasSuffix(lower, ".sql") {
		return fmt.Sprintf("critical path %s", rel)
	}
	for _, tok := range highRiskTokens {
		if strings.Contains(lower, tok) {
			return fmt.Sprintf("critical path (%s) %s", tok, rel)
		}
	}
	return ""
}
