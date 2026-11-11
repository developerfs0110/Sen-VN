// Tệp snapshot.go xử lý quét cây thư mục độc lập với Git, chặn symlink/junction, sao lưu <=10MB và nâng mốc __BASELINE__.
package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/developerfs0110/Sen-VN/internal/storage"
)

const (
	// BaselineTaskID định danh mốc trạng thái nền chuẩn của toàn bộ workspace trong bảng snapshots.
	BaselineTaskID = "__BASELINE__"
	// MaxSnapshotBytes giới hạn ngưỡng 10MB để tránh sao chép tệp đa phương tiện dung lượng lớn.
	MaxSnapshotBytes  = 10 * 1024 * 1024
	allowDeleteMarker = ".allow_delete"
)

// ignoredDirs liệt kê các thư mục hệ thống/phụ thuộc được bỏ qua khi quét ảnh chụp workspace.
var ignoredDirs = map[string]bool{
	".git":         true,
	".sen":         true,
	".agents":      true,
	"node_modules": true,
	".venv":        true,
	"dist":         true,
	"build":        true,
	"__pycache__":  true,
}

// ensureNoSymlinkChain kiểm tra từng chặng thư mục cha bằng os.Lstat để chặn symlink và NTFS junction trỏ ra ngoài.
func ensureNoSymlinkChain(root, rel string) error {
	parts := strings.Split(filepath.ToSlash(rel), "/")
	cur := root
	for _, part := range parts {
		if part == "" || part == "." {
			continue
		}
		cur = filepath.Join(cur, part)
		info, err := os.Lstat(cur)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 || info.Mode()&os.ModeIrregular != 0 {
			return fmt.Errorf("symlink or reparse point forbidden in workspace: %s", rel)
		}
	}
	return nil
}

// scanWorkspaceFiles duyệt toàn bộ cây dự án, từ chối symlink/reparse point và thu thập siêu dữ liệu từng tệp.
func scanWorkspaceFiles(root string) (map[string]storage.Snapshot, []storage.Snapshot, error) {
	byKey := make(map[string]storage.Snapshot)
	var list []storage.Snapshot

	err := filepath.WalkDir(root, func(fullPath string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if fullPath == root {
			return nil
		}
		relRaw, err := filepath.Rel(root, fullPath)
		if err != nil {
			return err
		}
		rel := filepath.ToSlash(relRaw)
		if d.Type()&os.ModeSymlink != 0 || d.Type()&os.ModeIrregular != 0 {
			return fmt.Errorf("symlink or reparse point forbidden in workspace: %s", rel)
		}
		if d.IsDir() {
			if ignoredDirs[strings.ToLower(d.Name())] {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		sn, err := buildFileSnapshot(fullPath, rel, info)
		if err != nil {
			return err
		}
		byKey[PathKey(rel)] = sn
		list = append(list, sn)
		return nil
	})
	return byKey, list, err
}

// buildFileSnapshot tính SHA-256 cho tệp <=10MB; với tệp lớn hơn chỉ ghi nhận kích thước và thời gian sửa đổi (mtime_ns).
func buildFileSnapshot(fullPath, rel string, info fs.FileInfo) (storage.Snapshot, error) {
	sn := storage.Snapshot{
		RelPath:       rel,
		ExistedBefore: true,
		OrigSize:      info.Size(),
		OrigMtimeNS:   info.ModTime().UnixNano(),
		IsLockedTest:  IsTestFile(rel),
	}
	if info.Size() > MaxSnapshotBytes {
		return sn, nil
	}
	sum, err := hashFileSHA256(fullPath)
	if err != nil {
		return sn, err
	}
	sn.OrigSHA256 = sum
	return sn, nil
}

// hashFileSHA256 đọc luồng dữ liệu tệp và trả về mã băm SHA-256 thập lục phân.
func hashFileSHA256(fullPath string) (string, error) {
	f, err := os.Open(fullPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// copyFileBytes sao chép nguyên vẹn từng byte từ tệp nguồn sang tệp sao lưu đích.
func copyFileBytes(src, dst string) error {
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

// CaptureBaseline chụp lại trạng thái nền chuẩn của toàn bộ dự án vào khóa __BASELINE__ mà không phụ thuộc Git.
func CaptureBaseline(st *storage.Store, root string) error {
	_, list, err := scanWorkspaceFiles(root)
	if err != nil {
		return fmt.Errorf("scan baseline: %w", err)
	}
	for i := range list {
		list[i].TaskID = BaselineTaskID
	}
	return st.ReplaceSnapshots(BaselineTaskID, list)
}

// BeginTaskSnapshot khóa toàn bộ tệp kiểm thử có sẵn và sao lưu các tệp thuộc phạm vi khai báo (mặc định cấm xóa tệp).
func BeginTaskSnapshot(st *storage.Store, root, taskID string, scopeFiles []string) error {
	return BeginTaskSnapshotWithOpts(st, root, taskID, scopeFiles, false)
}

// BeginTaskSnapshotWithOpts thiết lập ảnh chụp trước khi sửa mã, từ chối đưa tệp test có sẵn vào --files.
func BeginTaskSnapshotWithOpts(st *storage.Store, root, taskID string, scopeFiles []string, allowDelete bool) error {
	if err := ValidateTaskID(taskID); err != nil {
		return err
	}
	normScope, err := normalizeAndValidateScope(root, scopeFiles)
	if err != nil {
		return err
	}
	currentByKey, currentList, err := scanWorkspaceFiles(root)
	if err != nil {
		return err
	}
	for _, rel := range normScope {
		if existing, ok := currentByKey[PathKey(rel)]; ok && existing.IsLockedTest {
			return fmt.Errorf("pre-existing test file %q is locked and cannot be listed in --files", rel)
		}
	}

	snapBaseDir := filepath.Join(root, ".sen", "snapshots", taskID)
	if err := os.MkdirAll(snapBaseDir, 0o755); err != nil {
		return err
	}
	if allowDelete {
		if err := os.WriteFile(filepath.Join(snapBaseDir, allowDeleteMarker), []byte("1\n"), 0o644); err != nil {
			return err
		}
	}

	snaps, err := collectTaskSnapshots(root, taskID, snapBaseDir, normScope, currentByKey, currentList)
	if err != nil {
		return err
	}
	return st.ReplaceSnapshots(taskID, snaps)
}

// normalizeAndValidateScope chuẩn hóa danh sách tệp phạm vi, loại trùng lặp và cấm chạm vào thư mục bảo vệ .sen/.
func normalizeAndValidateScope(root string, scopeFiles []string) ([]string, error) {
	normScope := make([]string, 0, len(scopeFiles))
	seen := make(map[string]bool)
	for _, raw := range scopeFiles {
		rel, err := NormalizeRelPath(root, raw)
		if err != nil {
			return nil, err
		}
		lower := strings.ToLower(rel)
		if lower == ".sen" || strings.HasPrefix(lower, ".sen/") {
			return nil, fmt.Errorf("cannot include protected .sen path %q in scope", rel)
		}
		if err := ensureNoSymlinkChain(root, rel); err != nil {
			return nil, err
		}
		key := PathKey(rel)
		if !seen[key] {
			seen[key] = true
			normScope = append(normScope, rel)
		}
	}
	return normScope, nil
}

// collectTaskSnapshots sao lưu các tệp test bị khóa cùng các tệp phạm vi <=10MB vào .sen/snapshots/<task_id>/.
func collectTaskSnapshots(
	root, taskID, snapBaseDir string,
	normScope []string,
	currentByKey map[string]storage.Snapshot,
	currentList []storage.Snapshot,
) ([]storage.Snapshot, error) {
	var snaps []storage.Snapshot
	for _, item := range currentList {
		if !item.IsLockedTest {
			continue
		}
		item.TaskID = taskID
		if item.OrigSize <= MaxSnapshotBytes {
			bkp := filepath.Join(snapBaseDir, "__locked__", filepath.FromSlash(item.RelPath))
			if err := copyFileBytes(filepath.Join(root, filepath.FromSlash(item.RelPath)), bkp); err != nil {
				return nil, fmt.Errorf("backup locked test %s: %w", item.RelPath, err)
			}
			item.BackupPath = bkp
		}
		snaps = append(snaps, item)
	}
	for _, rel := range normScope {
		if existing, ok := currentByKey[PathKey(rel)]; ok {
			existing.TaskID = taskID
			existing.IsLockedTest = false
			if existing.OrigSize <= MaxSnapshotBytes {
				bkp := filepath.Join(snapBaseDir, "files", filepath.FromSlash(existing.RelPath))
				if err := copyFileBytes(filepath.Join(root, filepath.FromSlash(existing.RelPath)), bkp); err != nil {
					return nil, fmt.Errorf("backup scope file %s: %w", existing.RelPath, err)
				}
				existing.BackupPath = bkp
			}
			snaps = append(snaps, existing)
		} else {
			snaps = append(snaps, storage.Snapshot{
				TaskID: taskID, RelPath: rel, ExistedBefore: false, IsLockedTest: false,
			})
		}
	}
	return snaps, nil
}

// AdvanceBaseline nâng mốc __BASELINE__ chỉ cho đúng các tệp thuộc task vừa VERIFIED và tự khóa các tệp test mới viết.
func AdvanceBaseline(st *storage.Store, root string, scopeFiles []string) error {
	for _, raw := range scopeFiles {
		rel, err := NormalizeRelPath(root, raw)
		if err != nil {
			return err
		}
		if err := ensureNoSymlinkChain(root, rel); err != nil {
			return err
		}
		full := filepath.Join(root, filepath.FromSlash(rel))
		info, err := os.Lstat(full)
		if os.IsNotExist(err) {
			if err := st.DeleteSnapshotEntry(BaselineTaskID, rel); err != nil {
				return err
			}
			continue
		}
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("non-regular file forbidden in verified scope: %s", rel)
		}
		sn, err := buildFileSnapshot(full, rel, info)
		if err != nil {
			return err
		}
		sn.TaskID = BaselineTaskID
		if err := st.UpsertSnapshotEntry(sn); err != nil {
			return err
		}
	}
	return nil
}
