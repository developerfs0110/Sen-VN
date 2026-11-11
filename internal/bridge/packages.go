// Tệp packages.go quản lý danh mục gói kỹ năng nhúng (//go:embed), tự động cài đặt gói mặc định và các gói mở rộng (--packs).
package bridge

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/developerfs0110/Sen-VN/internal/workspace"
)

// embeddedPacksFS nhúng sẵn packages.lock.json và 5 gói kỹ năng chuẩn vào tệp thực thi sen để hoạt động độc lập.
//
//go:embed embedded_packs
var embeddedPacksFS embed.FS

// CatalogManifest ánh xạ toàn bộ tệp danh mục gốc packages.lock.json.
type CatalogManifest struct {
	Version  string           `json:"version"`
	Packages []CatalogPackage `json:"packages"`
}

// CatalogPackage mô tả nguồn tải chính, nguồn dự phòng fork developerfs0110, mã commit cố định và thư mục đích của một gói.
type CatalogPackage struct {
	ID           string `json:"id"`
	Pack         string `json:"pack"`
	Default      bool   `json:"default"`
	Description  string `json:"description"`
	SourceType   string `json:"sourceType"`
	PrimaryURL   string `json:"primaryUrl,omitempty"`
	FallbackURL  string `json:"fallbackUrl,omitempty"`
	PinnedCommit string `json:"pinnedCommit,omitempty"`
	SubPath      string `json:"subPath,omitempty"`
	EmbeddedDir  string `json:"embeddedDir,omitempty"`
	Dest         string `json:"dest"`
}

// PackageURLOverride cho phép ghi đè URL kho Git và mã commit qua biến môi trường SEN_PACKAGE_URL_OVERRIDES khi kiểm thử E2E.
type PackageURLOverride struct {
	PrimaryURL   string `json:"primaryUrl"`
	FallbackURL  string `json:"fallbackUrl"`
	PinnedCommit string `json:"pinnedCommit"`
}

// InstalledPackageResult báo cáo kết quả cài đặt hoặc bỏ qua (exists) của từng gói kỹ năng.
type InstalledPackageResult struct {
	ID         string `json:"id"`
	Pack       string `json:"pack"`
	Status     string `json:"status"`
	SourceUsed string `json:"sourceUsed"`
	Commit     string `json:"commit"`
	TreeHash   string `json:"treeHash"`
	Dest       string `json:"dest"`
}

// WorkspaceLockFile lưu trữ trạng thái thực tế của các gói đã cài tại .agents/sen.lock.json của workspace.
type WorkspaceLockFile struct {
	Version   string                 `json:"version"`
	UpdatedAt string                 `json:"updatedAt"`
	Packages  []InstalledPackageResult `json:"packages"`
}

// LoadCatalogManifestWithOverrides đọc danh mục nhúng và áp dụng cấu hình ghi đè từ SEN_PACKAGE_URL_OVERRIDES (nếu có).
func LoadCatalogManifestWithOverrides() (*CatalogManifest, error) {
	raw, err := embeddedPacksFS.ReadFile("embedded_packs/packages.lock.json")
	if err != nil {
		return nil, fmt.Errorf("read embedded packages.lock.json: %w", err)
	}
	var manifest CatalogManifest
	if err := json.Unmarshal(raw, &manifest); err != nil {
		return nil, fmt.Errorf("parse packages.lock.json: %w", err)
	}
	ovRaw := strings.TrimSpace(os.Getenv("SEN_PACKAGE_URL_OVERRIDES"))
	if ovRaw != "" {
		var overrides map[string]PackageURLOverride
		if err := json.Unmarshal([]byte(ovRaw), &overrides); err != nil {
			return nil, fmt.Errorf("invalid SEN_PACKAGE_URL_OVERRIDES JSON: %w", err)
		}
		for i := range manifest.Packages {
			applyPackageOverride(&manifest.Packages[i], overrides)
		}
	}
	return &manifest, nil
}

// applyPackageOverride cập nhật URL chính/dự phòng và commit mục tiêu nếu khớp mã gói hoặc tên nhóm pack.
func applyPackageOverride(pkg *CatalogPackage, overrides map[string]PackageURLOverride) {
	ov, ok := overrides[pkg.ID]
	if !ok {
		ov, ok = overrides[pkg.Pack]
	}
	if !ok {
		ov, ok = overrides["*"]
	}
	if !ok {
		return
	}
	if strings.TrimSpace(ov.PrimaryURL) != "" {
		pkg.PrimaryURL = strings.TrimSpace(ov.PrimaryURL)
	}
	if strings.TrimSpace(ov.FallbackURL) != "" {
		pkg.FallbackURL = strings.TrimSpace(ov.FallbackURL)
	}
	if strings.TrimSpace(ov.PinnedCommit) != "" {
		pkg.PinnedCommit = strings.TrimSpace(ov.PinnedCommit)
	}
}

// FormatPacksCatalog kết xuất bảng hướng dẫn các gói mặc định (core) và 5 nhóm gói mở rộng tùy chọn (--packs list).
func FormatPacksCatalog() string {
	manifest, err := LoadCatalogManifestWithOverrides()
	if err != nil {
		return "Error loading catalog: " + err.Error() + "\n"
	}
	var b strings.Builder
	b.WriteString("=== SEN-VN COMPANION & OPTIONAL PACKS CATALOG ===\n")
	b.WriteString("Default Bundle (`sen init --install-companions`):\n")
	for _, p := range manifest.Packages {
		if p.Default {
			b.WriteString(fmt.Sprintf("  - %-22s -> %-34s (%s)\n", p.ID, p.Dest, p.Description))
		}
	}
	b.WriteString("\nOptional Recommended Packs (`sen init --packs <pack1,pack2|all>`):\n")
	for _, p := range manifest.Packages {
		if !p.Default {
			b.WriteString(fmt.Sprintf("  - [%-16s] %-24s -> %s\n", p.Pack, p.ID, p.Description))
		}
	}
	return b.String()
}

// InstallCompanionsAndPacks cài đặt các gói mặc định và/hoặc nhóm gói tùy chọn, tuyệt đối giữ nguyên gói đã có sẵn.
func InstallCompanionsAndPacks(
	ctx context.Context,
	root string,
	installDefaults bool,
	packsCSV string,
	stdout io.Writer,
) ([]InstalledPackageResult, error) {
	manifest, err := LoadCatalogManifestWithOverrides()
	if err != nil {
		return nil, err
	}
	selected, err := selectCatalogPackages(manifest.Packages, installDefaults, packsCSV)
	if err != nil {
		return nil, err
	}
	comp := DetectCompanions(root)
	cache := make(map[string]clonedRepoEntry)
	defer cleanupCloneCache(cache)

	var results []InstalledPackageResult
	var newlyLocked []InstalledPackageResult
	for _, pkg := range selected {
		if packageAlreadyExists(root, pkg, comp) {
			res := InstalledPackageResult{ID: pkg.ID, Pack: pkg.Pack, Status: "exists", Dest: pkg.Dest}
			results = append(results, res)
			if stdout != nil {
				fmt.Fprintf(stdout, "  [exists]    %-24s (%s)\n", pkg.ID, pkg.Dest)
			}
			continue
		}
		res, err := installSingleCatalogPackage(ctx, root, pkg, cache)
		if err != nil {
			return results, err
		}
		results = append(results, res)
		newlyLocked = append(newlyLocked, res)
		if stdout != nil {
			fmt.Fprintf(stdout, "  [installed] %-24s via %-8s -> %s\n", res.ID, res.SourceUsed, res.Dest)
		}
	}
	if len(newlyLocked) > 0 {
		if err := upsertWorkspaceLock(root, newlyLocked); err != nil {
			return results, err
		}
	}
	return results, nil
}

// selectCatalogPackages lọc danh sách các gói cần cài dựa trên cờ installDefaults và chuỗi tham số packsCSV.
func selectCatalogPackages(all []CatalogPackage, installDefaults bool, packsCSV string) ([]CatalogPackage, error) {
	var out []CatalogPackage
	seen := make(map[string]bool)
	add := func(p CatalogPackage) {
		if !seen[p.ID] {
			seen[p.ID] = true
			out = append(out, p)
		}
	}
	if installDefaults {
		for _, p := range all {
			if p.Default {
				add(p)
			}
		}
	}
	for _, rawTok := range strings.Split(packsCSV, ",") {
		tok := strings.ToLower(strings.TrimSpace(rawTok))
		if tok == "" {
			continue
		}
		if tok == "all" {
			for _, p := range all {
				if !p.Default {
					add(p)
				}
			}
			continue
		}
		matched := false
		for _, p := range all {
			if strings.EqualFold(p.Pack, tok) || strings.EqualFold(p.ID, tok) {
				add(p)
				matched = true
			}
		}
		if !matched {
			return nil, fmt.Errorf("unknown pack %q (run `sen init --packs list` to inspect available packs)", rawTok)
		}
	}
	return out, nil
}

// packageAlreadyExists kiểm tra xem kỹ năng đã tồn tại ở thư mục đích hoặc ở các cấp cha .agents/ hay chưa.
func packageAlreadyExists(root string, pkg CatalogPackage, comp CompanionStatus) bool {
	switch pkg.ID {
	case "superpowers":
		if comp.Superpowers {
			return true
		}
	case "security-audit":
		if comp.SecurityAudit {
			return true
		}
	case "vn-law-check":
		if comp.VnLawCheck {
			return true
		}
	}
	destAbs := filepath.Join(root, filepath.FromSlash(pkg.Dest))
	entries, err := os.ReadDir(destAbs)
	return err == nil && len(entries) > 0
}

// installEmbeddedPackage trích xuất gói kỹ năng nhúng từ embeddedPacksFS sang thư mục .agents/ của dự án.
func installEmbeddedPackage(root string, pkg CatalogPackage) (InstalledPackageResult, error) {
	srcPrefix := path.Join("embedded_packs", pkg.EmbeddedDir)
	destAbs := filepath.Join(root, filepath.FromSlash(pkg.Dest))
	err := fs.WalkDir(embeddedPacksFS, srcPrefix, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel := strings.TrimPrefix(strings.TrimPrefix(p, srcPrefix), "/")
		if rel == "" {
			return os.MkdirAll(destAbs, 0o755)
		}
		target := filepath.Join(destAbs, filepath.FromSlash(rel))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, rErr := embeddedPacksFS.ReadFile(p)
		if rErr != nil {
			return rErr
		}
		if mErr := os.MkdirAll(filepath.Dir(target), 0o755); mErr != nil {
			return mErr
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		return InstalledPackageResult{}, fmt.Errorf("extract embedded skill %s: %w", pkg.ID, err)
	}
	commitTag := "embedded-" + workspace.CurrentVersion
	treeHash, err := writePackageIntegrityMarkers(destAbs, commitTag)
	if err != nil {
		return InstalledPackageResult{}, err
	}
	return InstalledPackageResult{
		ID: pkg.ID, Pack: pkg.Pack, Status: "installed", SourceUsed: "embedded",
		Commit: commitTag, TreeHash: treeHash, Dest: pkg.Dest,
	}, nil
}

// ComputeDirTreeHash tính mã băm SHA-256 tất định cho toàn bộ cây tệp của gói kỹ năng (bỏ qua các tệp dấu .sen-*).
func ComputeDirTreeHash(dir string) (string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(full string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".sen-") {
			return nil
		}
		rel, rErr := filepath.Rel(dir, full)
		if rErr != nil {
			return rErr
		}
		files = append(files, filepath.ToSlash(rel))
		return nil
	})
	if err != nil {
		return "", err
	}
	sort.Strings(files)
	h := sha256.New()
	for _, rel := range files {
		data, rErr := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if rErr != nil {
			return "", rErr
		}
		_, _ = h.Write([]byte(rel + "\x00"))
		_, _ = h.Write(data)
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// upsertWorkspaceLock cập nhật danh sách các gói đã cài hoặc nâng cấp vào tệp .agents/sen.lock.json.
func upsertWorkspaceLock(root string, updated []InstalledPackageResult) error {
	lockPath := filepath.Join(root, ".agents", "sen.lock.json")
	var lock WorkspaceLockFile
	if raw, err := os.ReadFile(lockPath); err == nil && len(raw) > 0 {
		_ = json.Unmarshal(raw, &lock)
	}
	byID := make(map[string]InstalledPackageResult, len(lock.Packages))
	var order []string
	for _, p := range lock.Packages {
		byID[p.ID] = p
		order = append(order, p.ID)
	}
	for _, u := range updated {
		if _, exists := byID[u.ID]; !exists {
			order = append(order, u.ID)
		}
		byID[u.ID] = u
	}
	lock.Version = workspace.CurrentVersion
	lock.UpdatedAt = time.Now().UTC().Format(time.RFC3339)
	lock.Packages = make([]InstalledPackageResult, 0, len(order))
	for _, id := range order {
		lock.Packages = append(lock.Packages, byID[id])
	}
	if err := os.MkdirAll(filepath.Dir(lockPath), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(lock, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(lockPath, append(data, '\n'), 0o644)
}

