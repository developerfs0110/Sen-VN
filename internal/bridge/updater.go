// Tệp updater.go kiểm tra phiên bản mới từ GitHub Releases API và đồng bộ gói kỹ năng với cơ chế sao lưu/rollback .bak.
package bridge

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/developerfs0110/Sen-VN/internal/workspace"
)

// defaultReleasesAPIURL trỏ tới điểm cuối GitHub Releases chính thức của Sen-VN khi không ghi đè SEN_UPDATE_API_URL.
const defaultReleasesAPIURL = "https://api.github.com/repos/developerfs0110/Sen-VN/releases/latest"

// githubReleaseInfo ánh xạ các trường phản hồi từ GitHub Releases API phục vụ so sánh phiên bản.
type githubReleaseInfo struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

// RunWorkspaceUpdate kiểm tra bản phát hành mới nhất của Sen-VN và cập nhật an toàn các gói trong .agents/sen.lock.json.
func RunWorkspaceUpdate(ctx context.Context, root string, stdout io.Writer) error {
	if err := checkLatestReleaseVersion(ctx, stdout); err != nil {
		return err
	}
	return syncInstalledWorkspacePackages(ctx, root, stdout)
}

// checkLatestReleaseVersion truy vấn GitHub Releases API (hoặc SEN_UPDATE_API_URL) và so sánh với CurrentVersion (v1.1.0).
func checkLatestReleaseVersion(ctx context.Context, stdout io.Writer) error {
	apiURL := strings.TrimSpace(os.Getenv("SEN_UPDATE_API_URL"))
	customURL := apiURL != ""
	if !customURL {
		apiURL = defaultReleasesAPIURL
	}
	tctx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(tctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "Sen-VN/"+workspace.CurrentVersion)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if customURL {
			return fmt.Errorf("check release API %s: %w", apiURL, err)
		}
		if stdout != nil {
			fmt.Fprintf(stdout, "Release check skipped (offline): %v\n", err)
		}
		return nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		if customURL {
			return fmt.Errorf("release API returned HTTP %d", resp.StatusCode)
		}
		if stdout != nil {
			fmt.Fprintf(stdout, "Release check status HTTP %d (current: v%s)\n", resp.StatusCode, workspace.CurrentVersion)
		}
		return nil
	}

	var info githubReleaseInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return fmt.Errorf("decode release JSON: %w", err)
	}
	if stdout != nil {
		if compareSemver(info.TagName, workspace.CurrentVersion) > 0 {
			fmt.Fprintf(stdout, "Sen-VN update available: %s (current: v%s) — run `install-sen.ps1 -Update` or `install-sen.sh --update`\n",
				info.TagName, workspace.CurrentVersion)
		} else {
			fmt.Fprintf(stdout, "Sen-VN CLI is up-to-date (v%s)\n", workspace.CurrentVersion)
		}
	}
	return nil
}

// syncInstalledWorkspacePackages đối chiếu .agents/sen.lock.json với danh mục mới nhất, sao lưu .bak và rollback nếu lỗi.
func syncInstalledWorkspacePackages(ctx context.Context, root string, stdout io.Writer) error {
	lockPath := filepath.Join(root, ".agents", "sen.lock.json")
	rawLock, err := os.ReadFile(lockPath)
	if os.IsNotExist(err) {
		if stdout != nil {
			fmt.Fprintln(stdout, "No managed .agents/sen.lock.json found (run `sen init --install-companions` to install companions).")
		}
		return nil
	}
	if err != nil {
		return err
	}
	var lock WorkspaceLockFile
	if err := json.Unmarshal(rawLock, &lock); err != nil {
		return fmt.Errorf("parse .agents/sen.lock.json: %w", err)
	}
	manifest, err := LoadCatalogManifestWithOverrides()
	if err != nil {
		return err
	}
	catalogByID := make(map[string]CatalogPackage, len(manifest.Packages))
	for _, p := range manifest.Packages {
		catalogByID[p.ID] = p
	}

	cache := make(map[string]clonedRepoEntry)
	defer cleanupCloneCache(cache)

	var updated []InstalledPackageResult
	for _, inst := range lock.Packages {
		target, ok := catalogByID[inst.ID]
		if !ok || !packageNeedsUpdate(root, inst, target) {
			continue
		}
		res, uErr := atomicUpdateSinglePackage(ctx, root, target, cache)
		if uErr != nil {
			return uErr
		}
		updated = append(updated, res)
		if stdout != nil {
			fmt.Fprintf(stdout, "Updated package %-22s -> %s\n", res.ID, res.Commit)
		}
	}
	if len(updated) == 0 {
		if stdout != nil {
			fmt.Fprintln(stdout, "All installed companion & pack skills are up-to-date.")
		}
		return nil
	}
	return upsertWorkspaceLock(root, updated)
}

// packageNeedsUpdate kiểm tra xem gói đã cài có lệch mã commit cố định so với danh mục mục tiêu hay không.
func packageNeedsUpdate(root string, inst InstalledPackageResult, target CatalogPackage) bool {
	destAbs := filepath.Join(root, filepath.FromSlash(target.Dest))
	if !dirExists(destAbs) {
		return true
	}
	wantCommit := strings.TrimSpace(target.PinnedCommit)
	if strings.EqualFold(target.SourceType, "embedded") {
		wantCommit = "embedded-" + workspace.CurrentVersion
	}
	gotBytes, err := os.ReadFile(filepath.Join(destAbs, ".sen-commit"))
	if err != nil {
		return strings.TrimSpace(inst.Commit) != wantCommit
	}
	return !strings.EqualFold(strings.TrimSpace(string(gotBytes)), wantCommit)
}

// atomicUpdateSinglePackage đổi tên thư mục hiện tại sang .bak trước khi nâng cấp và tự động khôi phục nếu quá trình tải lỗi.
func atomicUpdateSinglePackage(
	ctx context.Context,
	root string,
	target CatalogPackage,
	cache map[string]clonedRepoEntry,
) (InstalledPackageResult, error) {
	destAbs := filepath.Join(root, filepath.FromSlash(target.Dest))
	bakAbs := destAbs + ".bak"
	_ = os.RemoveAll(bakAbs)

	hadBackup := false
	if dirExists(destAbs) {
		if err := os.Rename(destAbs, bakAbs); err != nil {
			return InstalledPackageResult{}, fmt.Errorf("backup %s to .bak: %w", target.ID, err)
		}
		hadBackup = true
	}

	res, err := installSingleCatalogPackage(ctx, root, target, cache)
	if err != nil {
		_ = os.RemoveAll(destAbs)
		if hadBackup {
			_ = os.Rename(bakAbs, destAbs)
		}
		return InstalledPackageResult{}, fmt.Errorf("update package %s failed (rolled back from .bak): %w", target.ID, err)
	}
	if hadBackup {
		_ = os.RemoveAll(bakAbs)
	}
	return res, nil
}

// compareSemver so sánh hai chuỗi phiên bản ngữ nghĩa dạng vX.Y.Z (trả về 1 nếu a > b, -1 nếu a < b, 0 nếu bằng nhau).
func compareSemver(a, b string) int {
	pa := parseSemverParts(a)
	pb := parseSemverParts(b)
	for i := 0; i < 3; i++ {
		if pa[i] > pb[i] {
			return 1
		}
		if pa[i] < pb[i] {
			return -1
		}
	}
	return 0
}

// parseSemverParts tách chuỗi phiên bản (bỏ tiền tố v và hậu tố -rc) thành bộ 3 số nguyên [major, minor, patch].
func parseSemverParts(v string) [3]int {
	var out [3]int
	clean := strings.TrimPrefix(strings.ToLower(strings.TrimSpace(v)), "v")
	if idx := strings.IndexByte(clean, '-'); idx >= 0 {
		clean = clean[:idx]
	}
	parts := strings.Split(clean, ".")
	for i := 0; i < len(parts) && i < 3; i++ {
		out[i], _ = strconv.Atoi(parts[i])
	}
	return out
}

