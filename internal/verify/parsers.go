// Package verify thực thi kiểm chứng nhị phân trực tiếp, chặn bộ kiểm thử/media rỗng và đánh giá cổng nghiệm thu cuối cùng.
package verify

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
)

// Các biểu thức chính quy bóc tách số lượng bài kiểm thử chạy thành công từ đầu ra của Go, Pytest và Jest/Vitest.
var (
	reGoPassFunc   = regexp.MustCompile(`(?m)^--- PASS:\s+\S+`)
	reGoOkPkg      = regexp.MustCompile(`(?m)^ok\s+\S+([^\r\n]*)`)
	rePytestPassed = regexp.MustCompile(`(?i)(\d+)\s+passed`)
	reJestTests    = regexp.MustCompile(`(?im)^\s*Tests:\s+(?:[^\r\n]*?\s)?(\d+)\s+passed`)
	reGenericPass  = regexp.MustCompile(`(?i)(\d+)\s+passed`)
)

// DefaultAllowedBinaries trả về danh sách trắng các trình kiểm thử, kiểm định media, tài liệu và biên dịch an toàn mặc định.
func DefaultAllowedBinaries() []string {
	return []string{
		"go", "pytest", "jest", "vitest", "npm", "npx",
		"pnpm", "tsc", "cargo", "dotnet", "ffprobe", "magick",
		"markdownlint", "pandoc",
	}
}

// forbiddenShells cấm tuyệt đối các trình thông dịch shell và sqlite3 để ngăn chèn lệnh hoặc sửa trộm sổ cái.
var forbiddenShells = map[string]bool{
	"sh": true, "bash": true, "zsh": true, "cmd": true, "powershell": true, "pwsh": true,
	"wscript": true, "cscript": true, "mshta": true, "rundll32": true, "regsvr32": true, "sqlite3": true,
}

// nodeShimAllowlist liệt kê các công cụ hệ sinh thái Node.js hợp lệ vốn phân phối qua tệp .cmd trên Windows.
var nodeShimAllowlist = map[string]bool{
	"npm": true, "npx": true, "pnpm": true, "jest": true, "vitest": true, "tsc": true, "markdownlint": true,
}

// NormalizeBinName chuẩn hóa tên tệp thực thi về chữ thường và lược bỏ phần mở rộng .exe/.cmd/.bat.
func NormalizeBinName(bin string) string {
	base := strings.ToLower(filepath.Base(strings.ReplaceAll(strings.TrimSpace(bin), "\\", "/")))
	for _, ext := range []string{".exe", ".cmd", ".bat"} {
		base = strings.TrimSuffix(base, ext)
	}
	return base
}

// ValidateAllowedBinary chặn trình thông dịch shell, chặn đường dẫn kèm thư mục và đối chiếu danh sách trắng.
func ValidateAllowedBinary(bin string, allowedBins []string) (string, error) {
	trimmed := strings.TrimSpace(bin)
	if trimmed == "" {
		return "", errors.New("empty verification binary")
	}
	if len(allowedBins) == 0 {
		allowedBins = DefaultAllowedBinaries()
	}
	norm := NormalizeBinName(trimmed)
	if forbiddenShells[norm] {
		return "", fmt.Errorf("shell/interpreter %q is forbidden in sen verify/done", norm)
	}

	if strings.ContainsAny(trimmed, `/\:`) {
		cleanBin := filepath.Clean(trimmed)
		for _, a := range allowedBins {
			if strings.ContainsAny(a, `/\:`) && equalOSPath(filepath.Clean(a), cleanBin) {
				return norm, nil
			}
		}
		return "", fmt.Errorf("path-qualified binary %q is forbidden; pass bare binary name from allowed_binaries", trimmed)
	}

	for _, a := range allowedBins {
		if !strings.ContainsAny(a, `/\:`) && NormalizeBinName(a) == norm {
			return norm, nil
		}
	}
	return "", fmt.Errorf("binary %q is not in allowed_binaries %v", norm, allowedBins)
}

// ResolveSafeBinary phân giải đường dẫn thực thi từ PATH và chặn giả mạo binary nằm trong workspace hoặc thư mục tạm (%TEMP%).
func ResolveSafeBinary(bin string, allowedBins []string, workspaceRoot string) (string, error) {
	norm, err := ValidateAllowedBinary(bin, allowedBins)
	if err != nil {
		return "", err
	}
	resolved, err := exec.LookPath(strings.TrimSpace(bin))
	if err != nil {
		return "", fmt.Errorf("locate binary %q: %w", bin, err)
	}
	absBin, err := filepath.Abs(resolved)
	if err != nil {
		return "", err
	}
	if realBin, err := filepath.EvalSymlinks(absBin); err == nil {
		absBin = realBin
	}
	if workspaceRoot != "" && isWithinDir(absBin, workspaceRoot) {
		return "", fmt.Errorf("binary %q resolved inside workspace (%s) is forbidden", bin, absBin)
	}
	if isWithinDir(absBin, os.TempDir()) {
		return "", fmt.Errorf("binary %q resolved inside temp directory (%s) is forbidden", bin, absBin)
	}
	ext := strings.ToLower(filepath.Ext(absBin))
	if (ext == ".cmd" || ext == ".bat") && !nodeShimAllowlist[norm] {
		return "", fmt.Errorf(".cmd/.bat wrapper %q is forbidden for binary %q", absBin, norm)
	}
	return absBin, nil
}

// equalOSPath so sánh hai đường dẫn tệp theo quy tắc phân biệt hoa thường của từng hệ điều hành.
func equalOSPath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

// isWithinDir kiểm tra xem đường dẫn tệp thực thi có nằm bên trong một thư mục bị cấm hay không.
func isWithinDir(targetPath, dirPath string) bool {
	if strings.TrimSpace(dirPath) == "" {
		return false
	}
	absDir, err := filepath.Abs(dirPath)
	if err != nil {
		return false
	}
	if realDir, err := filepath.EvalSymlinks(absDir); err == nil {
		absDir = realDir
	}
	rel, err := filepath.Rel(absDir, targetPath)
	if err != nil {
		return false
	}
	slash := filepath.ToSlash(rel)
	return slash != ".." && !strings.HasPrefix(slash, "../") && filepath.VolumeName(rel) == ""
}

// classifyRunner phân loại độ mạnh của lệnh kiểm chứng thành TEST, MEDIA, BUILD hoặc RUN kèm tên bộ phân tích.
func classifyRunner(normBin string, args []string) (string, string) {
	first := ""
	if len(args) > 0 {
		first = strings.ToLower(strings.TrimSpace(args[0]))
	}
	joined := strings.ToLower(strings.Join(args, " "))

	switch normBin {
	case "ffprobe":
		return "MEDIA", "ffprobe"
	case "go":
		if first == "test" {
			return "TEST", "go"
		}
		if first == "build" || first == "vet" {
			return "BUILD", "go"
		}
		return "RUN", "go"
	case "pytest":
		return "TEST", "pytest"
	case "jest", "vitest":
		return "TEST", "jest"
	case "npm", "pnpm", "npx":
		if strings.Contains(joined, "pytest") {
			return "TEST", "pytest"
		}
		if first == "test" || first == "t" || strings.Contains(joined, "jest") || strings.Contains(joined, "vitest") || strings.Contains(joined, "run test") {
			return "TEST", "jest"
		}
		if strings.Contains(joined, "build") || strings.Contains(joined, "tsc") || strings.Contains(joined, "typecheck") {
			return "BUILD", normBin
		}
		return "RUN", normBin
	case "tsc":
		return "BUILD", "tsc"
	case "cargo":
		if first == "test" {
			return "TEST", "cargo"
		}
		if first == "build" || first == "check" {
			return "BUILD", "cargo"
		}
		return "RUN", "cargo"
	case "dotnet":
		if first == "test" {
			return "TEST", "dotnet"
		}
		if first == "build" {
			return "BUILD", "dotnet"
		}
		return "RUN", "dotnet"
	default:
		return "RUN", normBin
	}
}

// ParseTestRun phân tích đầu ra TEST/MEDIA và từ chối gian lận khi lệnh trả mã 0 nhưng không có bài test hoặc luồng hợp lệ.
func ParseTestRun(bin string, args []string, stdout string, exitCode int) (string, int, error) {
	strength, runner := classifyRunner(NormalizeBinName(bin), args)
	if strength == "MEDIA" {
		return parseFFprobeOutput(stdout, exitCode)
	}
	if strength != "TEST" {
		return strength, 0, nil
	}
	switch runner {
	case "go":
		return parseGoTestOutput(stdout, exitCode)
	case "pytest":
		return parsePytestOutput(stdout, exitCode)
	case "jest":
		count := extractFirstInt(reJestTests, stdout)
		if count == 0 {
			count = extractFirstInt(reGenericPass, stdout)
		}
		if exitCode == 0 && count == 0 {
			return "TEST", 0, errors.New("zero tests executed: jest/vitest reported 0 passed tests")
		}
		return "TEST", count, nil
	default:
		count := extractFirstInt(reGenericPass, stdout)
		if exitCode == 0 && count == 0 {
			return "TEST", 0, fmt.Errorf("zero tests executed for %s test runner", runner)
		}
		return "TEST", count, nil
	}
}

// parseGoTestOutput đếm số hàm TestXxx hoặc gói thực thi thật sự, chặn trường hợp [no test files] / [no tests to run].
func parseGoTestOutput(stdout string, exitCode int) (string, int, error) {
	passFuncs := len(reGoPassFunc.FindAllStringIndex(stdout, -1))
	okRealPkgs := 0
	for _, m := range reGoOkPkg.FindAllStringSubmatch(stdout, -1) {
		if !strings.Contains(strings.ToLower(m[1]), "no tests to run") {
			okRealPkgs++
		}
	}
	count := passFuncs
	if count == 0 {
		count = okRealPkgs
	}
	if exitCode == 0 && count == 0 {
		return "TEST", 0, errors.New("zero tests executed: go test found [no test files] or [no tests to run]")
	}
	return "TEST", count, nil
}

// parsePytestOutput chặn mã thoát 5 của pytest và đảm bảo có ít nhất 1 bài kiểm thử vượt qua.
func parsePytestOutput(stdout string, exitCode int) (string, int, error) {
	if exitCode == 5 {
		return "TEST", 0, errors.New("zero tests executed: pytest exited with code 5 (no tests collected)")
	}
	lower := strings.ToLower(stdout)
	if strings.Contains(lower, "collected 0 items") || strings.Contains(lower, "no tests ran") {
		return "TEST", 0, errors.New("zero tests executed: pytest collected 0 items")
	}
	count := extractFirstInt(rePytestPassed, stdout)
	if exitCode == 0 && count == 0 {
		return "TEST", 0, errors.New("zero tests executed: pytest reported 0 passed tests")
	}
	return "TEST", count, nil
}

// extractFirstInt trích xuất số nguyên đầu tiên từ nhóm khớp con thứ nhất của biểu thức chính quy.
func extractFirstInt(re *regexp.Regexp, s string) int {
	m := re.FindStringSubmatch(s)
	if len(m) < 2 {
		return 0
	}
	n, _ := strconv.Atoi(m[1])
	return n
}
