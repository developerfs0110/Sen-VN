// Tệp packages_git.go thực thi tải kho Git với cơ chế tự động chuyển sang nguồn fork dự phòng (fallbackUrl) và ghim đúng commit.
package bridge

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// clonedRepoEntry lưu trữ đường dẫn thư mục clone tạm thời và nguồn thực tế đã sử dụng (primary hoặc fallback).
type clonedRepoEntry struct {
	dir        string
	sourceUsed string
}

// cleanupCloneCache xóa toàn bộ các thư mục Git tạm thời sau khi hoàn tất phiên cài đặt gói kỹ năng.
func cleanupCloneCache(cache map[string]clonedRepoEntry) {
	for _, entry := range cache {
		if entry.dir != "" {
			_ = os.RemoveAll(entry.dir)
		}
	}
}

// installSingleCatalogPackage điều phối cài đặt một gói kỹ năng từ tài nguyên nhúng hoặc kho Git có kiểm chứng commit.
func installSingleCatalogPackage(
	ctx context.Context,
	root string,
	pkg CatalogPackage,
	cache map[string]clonedRepoEntry,
) (InstalledPackageResult, error) {
	if strings.EqualFold(pkg.SourceType, "embedded") {
		return installEmbeddedPackage(root, pkg)
	}
	cacheKey := pkg.PrimaryURL + "|" + pkg.FallbackURL + "|" + pkg.PinnedCommit
	cloned, ok := cache[cacheKey]
	if !ok {
		var err error
		cloned, err = cloneWithForkFallback(ctx, pkg)
		if err != nil {
			return InstalledPackageResult{}, err
		}
		cache[cacheKey] = cloned
	}

	srcDir := cloned.dir
	if strings.TrimSpace(pkg.SubPath) != "" {
		srcDir = filepath.Join(cloned.dir, filepath.FromSlash(pkg.SubPath))
	}
	if !dirExists(srcDir) {
		return InstalledPackageResult{}, fmt.Errorf("package %s subPath %q missing in cloned repo", pkg.ID, pkg.SubPath)
	}

	destAbs := filepath.Join(root, filepath.FromSlash(pkg.Dest))
	if err := copyDirectoryClean(srcDir, destAbs); err != nil {
		return InstalledPackageResult{}, fmt.Errorf("copy package %s: %w", pkg.ID, err)
	}
	treeHash, err := writePackageIntegrityMarkers(destAbs, pkg.PinnedCommit)
	if err != nil {
		return InstalledPackageResult{}, err
	}
	return InstalledPackageResult{
		ID: pkg.ID, Pack: pkg.Pack, Status: "installed", SourceUsed: cloned.sourceUsed,
		Commit: pkg.PinnedCommit, TreeHash: treeHash, Dest: pkg.Dest,
	}, nil
}

// cloneWithForkFallback thử tải từ primaryUrl trước; nếu lỗi sẽ tự động chuyển sang fallbackUrl (fork developerfs0110).
func cloneWithForkFallback(ctx context.Context, pkg CatalogPackage) (clonedRepoEntry, error) {
	dir, primaryErr := clonePinnedRepo(ctx, pkg.PrimaryURL, pkg.PinnedCommit)
	if primaryErr == nil {
		return clonedRepoEntry{dir: dir, sourceUsed: "primary"}, nil
	}
	if strings.TrimSpace(pkg.FallbackURL) == "" {
		return clonedRepoEntry{}, fmt.Errorf("clone %s from primary failed: %w", pkg.ID, primaryErr)
	}
	fbDir, fbErr := clonePinnedRepo(ctx, pkg.FallbackURL, pkg.PinnedCommit)
	if fbErr != nil {
		return clonedRepoEntry{}, fmt.Errorf("clone %s failed on both primary (%v) and fallback (%v)", pkg.ID, primaryErr, fbErr)
	}
	return clonedRepoEntry{dir: fbDir, sourceUsed: "fallback"}, nil
}

// clonePinnedRepo sao chép kho Git vào thư mục tạm, chặn chèn cờ Git, ép HTTPS ở môi trường thật và đối chiếu HEAD SHA-1.
func clonePinnedRepo(ctx context.Context, repoURL, pinnedCommit string) (string, error) {
	cleanURL := strings.TrimSpace(repoURL)
	if cleanURL == "" || strings.HasPrefix(cleanURL, "-") {
		return "", fmt.Errorf("invalid or unsafe repository URL %q", repoURL)
	}
	allowLocalOverride := strings.TrimSpace(os.Getenv("SEN_PACKAGE_URL_OVERRIDES")) != ""
	if !allowLocalOverride && !strings.HasPrefix(strings.ToLower(cleanURL), "https://") {
		return "", fmt.Errorf("non-HTTPS repository URL %q is forbidden", cleanURL)
	}
	tmpDir, err := os.MkdirTemp("", "sen-pkg-*")
	if err != nil {
		return "", err
	}
	cleanup := func() { _ = os.RemoveAll(tmpDir) }

	tctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	cloneArgs := []string{"clone", "--config", "core.autocrlf=false", "--no-checkout", "--", cleanURL, tmpDir}
	if allowLocalOverride {
		cloneArgs = append([]string{"-c", "protocol.file.allow=always"}, cloneArgs...)
	}
	if out, err := runGitCmd(tctx, "", cloneArgs...); err != nil {
		cleanup()
		return "", fmt.Errorf("git clone %s: %w (%s)", cleanURL, err, out)
	}
	want := strings.ToLower(strings.TrimSpace(pinnedCommit))
	if want != "" {
		if _, err := runGitCmd(tctx, tmpDir, "-c", "core.autocrlf=false", "checkout", "--detach", want); err != nil {
			fetchArgs := []string{"fetch", "--depth", "1", "origin", want}
			if allowLocalOverride {
				fetchArgs = append([]string{"-c", "protocol.file.allow=always"}, fetchArgs...)
			}
			_, _ = runGitCmd(tctx, tmpDir, fetchArgs...)
			if out, err2 := runGitCmd(tctx, tmpDir, "-c", "core.autocrlf=false", "checkout", "--detach", want); err2 != nil {
				cleanup()
				return "", fmt.Errorf("git checkout %s: %w (%s)", want, err2, out)
			}
		}
		gotHead, err := runGitCmd(tctx, tmpDir, "rev-parse", "HEAD")
		if err != nil || strings.ToLower(strings.TrimSpace(gotHead)) != want {
			cleanup()
			return "", fmt.Errorf("pinned commit mismatch: got %q, want %q", strings.TrimSpace(gotHead), want)
		}
	}
	return tmpDir, nil
}

// runGitCmd chạy tiến trình git với GIT_TERMINAL_PROMPT=0 để không bao giờ treo chờ nhập mật khẩu tương tác.
func runGitCmd(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
	out, err := cmd.CombinedOutput()
	return strings.TrimSpace(string(out)), err
}

// copyDirectoryClean sao chép đệ quy các tệp thông thường từ thư mục nguồn sang đích (bỏ qua siêu dữ liệu .git).
func copyDirectoryClean(srcDir, dstDir string) error {
	return filepath.WalkDir(srcDir, func(full string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() && d.Name() == ".git" {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(srcDir, full)
		if err != nil {
			return err
		}
		if rel == "." {
			return os.MkdirAll(dstDir, 0o755)
		}
		if d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		target := filepath.Join(dstDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyPlainFile(full, target)
	})
}

// copyPlainFile sao chép nội dung một tệp tin đơn lẻ và tạo sẵn chuỗi thư mục cha nếu cần.
func copyPlainFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}

// writePackageIntegrityMarkers tính mã băm cây thư mục và ghi cặp tệp .sen-commit + .sen-tree-hash vào gói đã cài.
func writePackageIntegrityMarkers(destAbs, commit string) (string, error) {
	treeHash, err := ComputeDirTreeHash(destAbs)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(destAbs, ".sen-commit"), []byte(strings.TrimSpace(commit)+"\n"), 0o644); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(destAbs, ".sen-tree-hash"), []byte(treeHash+"\n"), 0o644); err != nil {
		return "", err
	}
	return treeHash, nil
}
