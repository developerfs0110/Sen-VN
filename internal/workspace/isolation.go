// Tệp isolation.go định vị ranh giới dự án gần nhất và chặn tuyệt đối nhiễm chéo giữa các dự án con lồng nhau.
package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// standaloneManifestFiles liệt kê các tệp cấu hình gốc nhận diện biên giới của một dự án phần mềm độc lập.
var standaloneManifestFiles = []string{
	"go.mod",
	"Cargo.toml",
	"pyproject.toml",
	"pubspec.yaml",
	"pom.xml",
	"package.json",
}

// softwareManifestNames liệt kê tập tên tệp manifest/build dùng để nhận diện thao tác ghi cấu hình phần mềm tại PreToolUse.
var softwareManifestNames = map[string]bool{
	"dockerfile":       true,
	"go.mod":           true,
	"go.sum":           true,
	"package.json":     true,
	"pyproject.toml":   true,
	"cargo.toml":       true,
	"pubspec.yaml":     true,
	"pom.xml":          true,
	"requirements.txt": true,
	"build.gradle":     true,
	"build.gradle.kts": true,
	"composer.json":    true,
	"gemfile":          true,
	"cmakelists.txt":   true,
	"tsconfig.json":    true,
}

// softwareSourceExts liệt kê tập phần mở rộng mã nguồn phần mềm/web/mobile/SQL bị kiểm soát bởi tường lửa pha SSDS.
var softwareSourceExts = map[string]bool{
	".go":     true,
	".py":     true,
	".ts":     true,
	".tsx":    true,
	".js":     true,
	".jsx":    true,
	".mjs":    true,
	".cjs":    true,
	".rs":     true,
	".java":   true,
	".cs":     true,
	".kt":     true,
	".kts":    true,
	".swift":  true,
	".dart":   true,
	".cpp":    true,
	".cc":     true,
	".cxx":    true,
	".c":      true,
	".h":      true,
	".hpp":    true,
	".sql":    true,
	".vue":    true,
	".svelte": true,
	".php":    true,
	".rb":     true,
	".sh":     true,
	".ps1":    true,
	".bat":    true,
	".cmd":    true,
}

// ResolveNearestProjectRoot dò ngược từ đường dẫn tệp đích về thư mục chứa .sen/state.db hoặc ranh giới .git/manifest gần nhất.
func ResolveNearestProjectRoot(targetPath, fallbackCwd string) (projectRoot string, hasSenDB bool) {
	var absCwd string
	if strings.TrimSpace(fallbackCwd) != "" {
		if resolved, err := filepath.Abs(fallbackCwd); err == nil {
			absCwd = filepath.Clean(resolved)
		}
	}

	startDir := resolveInitialSearchDir(targetPath, absCwd)
	if startDir == "" {
		return absCwd, false
	}

	cur := stripAgentsSubtree(startDir)
	firstManifestOrGitDir := ""
	prevDir := ""

	for {
		if strings.EqualFold(filepath.Base(cur), ".agents") {
			parent := filepath.Dir(cur)
			if parent == cur {
				break
			}
			cur = parent
			continue
		}

		if prevDir != "" && IsUmbrellaMultiProjectDir(cur) {
			if firstManifestOrGitDir != "" {
				return firstManifestOrGitDir, false
			}
			if absCwd != "" && strings.EqualFold(filepath.Clean(cur), filepath.Clean(absCwd)) {
				return prevDir, false
			}
			break
		}

		if pathExists(filepath.Join(cur, ".sen", "state.db")) {
			return cur, true
		}

		hasGit := pathExists(filepath.Join(cur, ".git"))
		if firstManifestOrGitDir == "" && (hasGit || hasStandaloneManifest(cur)) {
			firstManifestOrGitDir = cur
		}
		// Dừng ngay tại biên giới .git đầu tiên nếu chưa gặp .sen/state.db để không rò rỉ lên Workspace mẹ bên ngoài.
		if hasGit {
			return cur, false
		}

		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		prevDir = cur
		cur = parent
	}

	if firstManifestOrGitDir != "" {
		return firstManifestOrGitDir, false
	}
	if absCwd != "" {
		return stripAgentsSubtree(absCwd), false
	}
	return stripAgentsSubtree(startDir), false
}

// CheckNoSubprojectCrossing chặn tuyệt đối đường dẫn phạm vi xuyên qua một thư mục con đã có .sen/state.db hoặc .git riêng.
func CheckNoSubprojectCrossing(projectRoot, relPath string) error {
	rel, err := NormalizeRelPath(projectRoot, relPath)
	if err != nil {
		return err
	}
	if strings.TrimSpace(projectRoot) == "" {
		return nil
	}
	absRoot, err := filepath.Abs(projectRoot)
	if err != nil {
		return err
	}

	parts := strings.Split(rel, "/")
	for i := 0; i < len(parts)-1; i++ {
		if i == 0 && (strings.EqualFold(parts[0], ".git") || strings.EqualFold(parts[0], ".sen")) {
			continue
		}
		subRel := strings.Join(parts[:i+1], "/")
		subAbs := filepath.Join(absRoot, filepath.FromSlash(subRel))
		if isIsolatedNestedProjectDir(subAbs) {
			return fmt.Errorf("sub-project boundary violation: path %q crosses into isolated nested project %q", rel, subRel)
		}
	}
	// Nếu đích đến chính là một thư mục con cô lập hiện hữu, cũng từ chối ngay.
	fullTarget := filepath.Join(absRoot, filepath.FromSlash(rel))
	if info, sErr := os.Lstat(fullTarget); sErr == nil && info.IsDir() && isIsolatedNestedProjectDir(fullTarget) {
		return fmt.Errorf("sub-project boundary violation: path %q crosses into isolated nested project %q", rel, rel)
	}
	return nil
}

// IsSoftwareSourceOrManifestPath nhận diện tệp mã nguồn hoặc manifest phần mềm (loại trừ .md/.txt/.json trong .agents/ và .sen/).
func IsSoftwareSourceOrManifestPath(relPath string) bool {
	clean := strings.Trim(strings.ToLower(filepath.ToSlash(strings.ReplaceAll(strings.TrimSpace(relPath), "\\", "/"))), "/")
	if clean == "" {
		return false
	}
	if clean == ".sen" || strings.HasPrefix(clean, ".sen/") || strings.Contains(clean, "/.sen/") {
		return false
	}
	if clean == ".agents" || strings.HasPrefix(clean, ".agents/") || strings.Contains(clean, "/.agents/") {
		return false
	}

	base := filepath.Base(clean)
	if softwareManifestNames[base] || strings.HasPrefix(base, "dockerfile.") ||
		strings.HasPrefix(base, "vite.config.") || strings.HasPrefix(base, "next.config.") {
		return true
	}
	ext := filepath.Ext(base)
	return softwareSourceExts[ext]
}

// resolveInitialSearchDir tính thư mục bắt đầu dò ngược từ targetPath (tương đối hoặc tuyệt đối) và fallbackCwd.
func resolveInitialSearchDir(targetPath, absCwd string) string {
	trimmed := strings.TrimSpace(targetPath)
	if trimmed == "" {
		return absCwd
	}
	var fullTarget string
	cleanedTarget := filepath.FromSlash(strings.ReplaceAll(trimmed, "\\", "/"))
	if filepath.IsAbs(cleanedTarget) {
		fullTarget = filepath.Clean(cleanedTarget)
	} else if absCwd != "" {
		fullTarget = filepath.Join(absCwd, cleanedTarget)
	} else if abs, err := filepath.Abs(cleanedTarget); err == nil {
		fullTarget = filepath.Clean(abs)
	} else {
		return ""
	}

	info, err := os.Stat(fullTarget)
	if err == nil {
		if info.IsDir() {
			return fullTarget
		}
		return filepath.Dir(fullTarget)
	}
	if filepath.Ext(fullTarget) != "" {
		return filepath.Dir(fullTarget)
	}
	return fullTarget
}

// stripAgentsSubtree nhảy thoát khỏi nhánh thư mục .agents/... về thư mục dự án bao ngoài nếu đường dẫn nằm trong .agents.
func stripAgentsSubtree(dir string) string {
	cur := filepath.Clean(dir)
	for {
		if strings.EqualFold(filepath.Base(cur), ".agents") {
			parent := filepath.Dir(cur)
			if parent != cur {
				return parent
			}
			return cur
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return dir
		}
		cur = parent
	}
}

// isIsolatedNestedProjectDir kiểm tra xem một thư mục có chứa sổ cái .sen/state.db hoặc kho .git độc lập riêng hay không.
func isIsolatedNestedProjectDir(dirAbs string) bool {
	return pathExists(filepath.Join(dirAbs, ".sen", "state.db")) || pathExists(filepath.Join(dirAbs, ".git"))
}

// hasStandaloneManifest kiểm tra sự tồn tại của bất kỳ tệp manifest gốc nào trong thư mục hiện xét.
func hasStandaloneManifest(dirAbs string) bool {
	entries, err := os.ReadDir(dirAbs)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		for _, mf := range standaloneManifestFiles {
			if strings.EqualFold(e.Name(), mf) {
				return true
			}
		}
	}
	return false
}

// IsUmbrellaMultiProjectDir nhận diện thư mục mẹ đa dự án (chứa các dự án con có .git hoặc .sen riêng ở tầng 1–2).
func IsUmbrellaMultiProjectDir(dir string) bool {
	trimmed := strings.TrimSpace(dir)
	if trimmed == "" {
		return false
	}
	absDir, err := filepath.Abs(trimmed)
	if err != nil {
		return false
	}
	if pathExists(filepath.Join(absDir, ".sen", "state.db")) {
		return false
	}
	lvl1, err := os.ReadDir(absDir)
	if err != nil {
		return false
	}
	for _, e1 := range lvl1 {
		if !e1.IsDir() || isUmbrellaIgnoredChild(e1.Name()) {
			continue
		}
		d1 := filepath.Join(absDir, e1.Name())
		if isIsolatedNestedProjectDir(d1) || hasStandaloneManifest(d1) {
			return true
		}
		lvl2, rErr := os.ReadDir(d1)
		if rErr != nil {
			continue
		}
		for _, e2 := range lvl2 {
			if !e2.IsDir() || isUmbrellaIgnoredChild(e2.Name()) {
				continue
			}
			d2 := filepath.Join(d1, e2.Name())
			if isIsolatedNestedProjectDir(d2) || hasStandaloneManifest(d2) {
				return true
			}
		}
	}
	return false
}

// isUmbrellaIgnoredChild bỏ qua các thư mục hệ thống/cấu hình dùng chung khi quét dự án con trong Workspace mẹ.
func isUmbrellaIgnoredChild(name string) bool {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case ".git", ".sen", ".agents", "node_modules", ".venv", "dist", "build", "__pycache__":
		return true
	default:
		return false
	}
}

// pathExists kiểm tra nhanh sự hiện diện của tệp hoặc thư mục bằng os.Lstat.
func pathExists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}
