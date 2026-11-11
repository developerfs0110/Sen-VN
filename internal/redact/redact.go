// Package redact cung cấp bộ lọc biến môi trường nhạy cảm cho tiến trình con và che giấu bí mật (secret) trong văn bản.
package redact

import (
	"regexp"
	"strings"
)

// safeEnvAllowlist liệt kê các biến môi trường hệ thống cốt lõi luôn được giữ lại để trình biên dịch hoạt động ổn định.
var safeEnvAllowlist = map[string]bool{
	"PATH":         true,
	"PATHEXT":      true,
	"GOPATH":       true,
	"GOROOT":       true,
	"GOCACHE":      true,
	"GOMODCACHE":   true,
	"GOENV":        true,
	"LOCALAPPDATA": true,
	"USERPROFILE":  true,
	"APPDATA":      true,
	"TEMP":         true,
	"TMP":          true,
	"SYSTEMROOT":   true,
	"WINDIR":       true,
	"COMSPEC":      true,
	"HOMEDRIVE":    true,
	"HOMEPATH":     true,
	"HOME":         true,
	"OS":           true,
}

// sensitiveEnvSubstrings chứa các từ khóa nhận diện tên biến môi trường mang thông tin xác thực hoặc khóa bí mật.
var sensitiveEnvSubstrings = []string{
	"KEY",
	"TOKEN",
	"SECRET",
	"PASSWORD",
	"PASSWD",
	"CREDENTIAL",
	"PRIVATE",
	"DATABASE_URL",
	"DSN",
}

// Các biểu thức chính quy nhận diện khóa riêng PEM, thông tin đăng nhập trong URL và các mẫu API token phổ biến.
var (
	rePEMKey    = regexp.MustCompile(`(?s)-----BEGIN [A-Z0-9 ]*PRIVATE KEY-----.*?-----END [A-Z0-9 ]*PRIVATE KEY-----`)
	reURLAuth   = regexp.MustCompile(`(?i)\b([a-z][a-z0-9+.-]*://[^/\s:@]+:)([^/\s@]+)(@)`)
	reOpenAIKey = regexp.MustCompile(`\bsk-[A-Za-z0-9_-]{12,}\b`)
	reGitHubPat = regexp.MustCompile(`\b(?:ghp|gho|ghu|ghs|ghr|github_pat)_[A-Za-z0-9_]{12,}\b`)
	reGoogleKey = regexp.MustCompile(`\bAIza[0-9A-Za-z_-]{20,}\b`)
	reAWSKey    = regexp.MustCompile(`\bAKIA[0-9A-Z]{16}\b`)
	reSlackTok  = regexp.MustCompile(`\bxox[baprs]-[A-Za-z0-9-]{10,}\b`)
	reBearer    = regexp.MustCompile(`(?i)(\bBearer\s+)[A-Za-z0-9._~+/=-]{8,}`)
	reKVSecret  = regexp.MustCompile(`(?i)(\b[A-Z0-9_]*(?:KEY|TOKEN|SECRET|PASSWORD|PASSWD|CREDENTIAL|DSN)[A-Z0-9_]*\s*[:=]\s*)([^\s"']+|"[^"]*"|'[^']*')`)
)

// FilterEnv loại bỏ các biến môi trường chứa từ khóa nhạy cảm trước khi khởi chạy tiến trình kiểm thử con.
func FilterEnv(env []string) []string {
	out := make([]string, 0, len(env))
	for _, entry := range env {
		idx := strings.IndexByte(entry, '=')
		if idx <= 0 {
			continue
		}
		upperKey := strings.ToUpper(strings.TrimSpace(entry[:idx]))
		if safeEnvAllowlist[upperKey] {
			out = append(out, entry)
			continue
		}
		if !hasSensitiveKeyword(upperKey) {
			out = append(out, entry)
		}
	}
	return out
}

// hasSensitiveKeyword kiểm tra xem tên biến môi trường (in hoa) có chứa hậu tố/trung tố nhạy cảm hay không.
func hasSensitiveKeyword(upperKey string) bool {
	for _, sub := range sensitiveEnvSubstrings {
		if strings.Contains(upperKey, sub) {
			return true
		}
	}
	return false
}

// ScrubText thay thế các chuỗi token bí mật, mật khẩu trong URL, khóa PEM và cặp khóa-giá trị nhạy cảm bằng [REDACTED].
func ScrubText(s string) string {
	if s == "" {
		return ""
	}
	res := rePEMKey.ReplaceAllString(s, "[REDACTED]")
	res = reURLAuth.ReplaceAllString(res, "${1}[REDACTED]${3}")
	res = reOpenAIKey.ReplaceAllString(res, "[REDACTED]")
	res = reGitHubPat.ReplaceAllString(res, "[REDACTED]")
	res = reGoogleKey.ReplaceAllString(res, "[REDACTED]")
	res = reAWSKey.ReplaceAllString(res, "[REDACTED]")
	res = reSlackTok.ReplaceAllString(res, "[REDACTED]")
	res = reBearer.ReplaceAllString(res, "${1}[REDACTED]")
	res = reKVSecret.ReplaceAllString(res, "${1}[REDACTED]")
	return res
}
