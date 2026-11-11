// Tệp agy_cmd_guard.go thực thi bộ lọc bảo mật cho lệnh run_command (chặn đụng .sen, biến môi trường nội bộ và tự phê duyệt SSDS).
package bridge

import (
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// reDotSenRef phát hiện tham chiếu trực tiếp tới thư mục sổ cái nội bộ .sen.
	reDotSenRef = regexp.MustCompile(`(?i)(^|[\s"'/\\=])\.sen($|[\s"'/\\])`)
	// reSenProtectedFiles phát hiện thao tác trực tiếp lên tệp cơ sở dữ liệu, chữ ký cấu hình hoặc bản sao lưu nâng cấp SSDS.
	reSenProtectedFiles = regexp.MustCompile(`(?i)(^|[\s"'/\\=])(state\.db|config\.sha256|ssds_upgrade_backup(?:\.json)?)($|[\s"'/\\])`)
	// reSqliteAgainstSen phát hiện công cụ sqlite3 nhắm vào cơ sở dữ liệu nội bộ của Sen.
	reSqliteAgainstSen = regexp.MustCompile(`(?i)\bsqlite3(?:\.exe)?\b[^\r\n]*\b(state\.db|ssds_|governance|evidence)\b`)
	// reProtectedEnvVar phát hiện hành vi ghi đè biến môi trường nạp Rulepack hoặc nguồn tải gói của Sen.
	reProtectedEnvVar = regexp.MustCompile(`(?i)\b(SEN_SSDS_EXTRA_VERSIONS_DIR|SEN_PACKAGE_URL_OVERRIDES|SEN_UPDATE_API_URL)\b`)
)

// commandTouchesDotSen kiểm tra câu lệnh shell có truy cập trái phép vào .sen, state.db, config.sha256 hoặc sqlite3 hay không.
func commandTouchesDotSen(cmdLine string) bool {
	trimmed := strings.TrimSpace(cmdLine)
	if trimmed == "" {
		return false
	}
	return reDotSenRef.MatchString(trimmed) ||
		reSenProtectedFiles.MatchString(trimmed) ||
		reSqliteAgainstSen.MatchString(trimmed)
}

// commandOverridesProtectedEnv chặn Agent tự ý gán biến môi trường nội bộ để giả mạo Rulepack SSDS hoặc nguồn cài đặt gói.
func commandOverridesProtectedEnv(cmdLine string) string {
	m := reProtectedEnvVar.FindString(cmdLine)
	if m == "" {
		return ""
	}
	return "Protected Sen environment variable " + m + " cannot be set or overridden via agent shell commands."
}

// shellEscapeStripper loại bỏ ký tự thoát đặc thù của PowerShell (`) và cmd.exe (^) khi phân tích tên lệnh/cờ.
var shellEscapeStripper = strings.NewReplacer("`", "", "^", "")

// commandRequiresHumanGovernanceConfirmation buộc IDE hỏi xác nhận người dùng thật (force_ask) khi lệnh cố tự phê duyệt hoặc bật --express.
func commandRequiresHumanGovernanceConfirmation(cmdLine string) (bool, string) {
	return commandRequiresHumanGovernanceConfirmationDepth(cmdLine, 0)
}

// commandRequiresHumanGovernanceConfirmationDepth đệ quy tối đa 3 tầng để bắt cả lệnh bọc qua powershell -c, cmd /c, sh -c, go run.
func commandRequiresHumanGovernanceConfirmationDepth(cmdLine string, depth int) (bool, string) {
	if depth > 3 {
		return false, ""
	}
	for _, seg := range splitShellCommandSegments(cmdLine) {
		tokens := tokenizeShellSegment(seg)
		for i, tok := range tokens {
			if strings.ContainsAny(tok, " \t\r\n;|&") {
				if hit, reason := commandRequiresHumanGovernanceConfirmationDepth(tok, depth+1); hit {
					return true, reason
				}
			}
			isGoRunSen := i > 0 && strings.EqualFold(normalizeShellToken(tokens[i-1]), "run") && isCmdSenPackageToken(tok)
			if !isSenBinaryToken(tok) && !isGoRunSen {
				continue
			}
			rest := tokens[i+1:]
			sawSDLC := false
			for _, arg := range rest {
				clean := normalizeShellToken(arg)
				if clean == "--approve" || clean == "-approve" ||
					strings.HasPrefix(clean, "--approve=") || strings.HasPrefix(clean, "-approve=") {
					return true, "SSDS §21 Governance Gate: Human confirmation required — AI agent cannot self-execute `--approve`."
				}
				if clean == "--express" || clean == "-express" ||
					strings.HasPrefix(clean, "--express=") || strings.HasPrefix(clean, "-express=") {
					return true, "Autonomous Mode Gate: Human confirmation required before enabling `--express`."
				}
				if clean == "sdlc" {
					sawSDLC = true
					continue
				}
				if sawSDLC && (clean == "approve" || clean == "exception") {
					return true, "SSDS §21/§27 Governance Gate: Human confirmation required for `sen sdlc " + clean + "`."
				}
			}
		}
	}
	return false, ""
}

// normalizeShellToken chuẩn hóa đối số dòng lệnh bằng cách cắt dấu nháy và gỡ ký tự thoát ngụy trang (` hoặc ^).
func normalizeShellToken(raw string) string {
	trimmed := strings.Trim(strings.TrimSpace(raw), `"'`)
	return strings.ToLower(shellEscapeStripper.Replace(trimmed))
}

// isCmdSenPackageToken nhận diện đối số gói nguồn ./cmd/sen khi chạy qua `go run ./cmd/sen ...`.
func isCmdSenPackageToken(raw string) bool {
	norm := strings.ReplaceAll(normalizeShellToken(raw), `\`, `/`)
	return norm == "./cmd/sen" || norm == "cmd/sen" || strings.HasSuffix(norm, "/cmd/sen")
}

// isSenBinaryToken nhận diện tên tệp thực thi sen trên mọi hệ điều hành (sen, sen.exe, ./bin/sen, bin\sen.cmd, kể cả khi chèn ` hoặc ^).
func isSenBinaryToken(raw string) bool {
	cleaned := shellEscapeStripper.Replace(strings.Trim(strings.TrimSpace(raw), `"'()&`))
	if cleaned == "" || strings.HasPrefix(cleaned, "-") {
		return false
	}
	norm := strings.ReplaceAll(cleaned, `\`, `/`)
	base := strings.ToLower(filepath.Base(norm))
	switch base {
	case "sen", "sen.exe", "sen.cmd", "sen.bat", "sen.ps1":
		return true
	default:
		return false
	}
}

// splitShellCommandSegments tách chuỗi lệnh phức hợp theo các toán tử nối lệnh (;, |, &, xuống dòng) có tôn trọng dấu nháy.
func splitShellCommandSegments(cmdLine string) []string {
	var segs []string
	var cur strings.Builder
	var quote rune
	for _, r := range cmdLine {
		if quote != 0 {
			cur.WriteRune(r)
			if r == quote {
				quote = 0
			}
			continue
		}
		if r == '"' || r == '\'' {
			quote = r
			cur.WriteRune(r)
			continue
		}
		if r == ';' || r == '|' || r == '&' || r == '\n' || r == '\r' {
			if s := strings.TrimSpace(cur.String()); s != "" {
				segs = append(segs, s)
			}
			cur.Reset()
			continue
		}
		cur.WriteRune(r)
	}
	if s := strings.TrimSpace(cur.String()); s != "" {
		segs = append(segs, s)
	}
	return segs
}

// tokenizeShellSegment tách một đoạn lệnh đơn thành mảng đối số (giữ nguyên cụm nằm trong dấu nháy đơn hoặc nháy kép).
func tokenizeShellSegment(seg string) []string {
	var tokens []string
	var cur strings.Builder
	var quote rune
	for _, r := range seg {
		if quote != 0 {
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
			continue
		}
		if r == '"' || r == '\'' {
			quote = r
			continue
		}
		if r == ' ' || r == '\t' {
			if cur.Len() > 0 {
				tokens = append(tokens, cur.String())
				cur.Reset()
			}
			continue
		}
		cur.WriteRune(r)
	}
	if cur.Len() > 0 {
		tokens = append(tokens, cur.String())
	}
	return tokens
}

