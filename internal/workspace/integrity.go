// Tệp integrity.go đối chiếu tính toàn vẹn của cây thư mục với __BASELINE__ và thực hiện hoàn tác từng byte (UndoTask).
package workspace

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/developerfs0110/Sen-VN/internal/storage"
)

// snapshotDrifted phát hiện sự thay đổi nội dung dựa trên SHA-256 (hoặc kích thước + mtime_ns với tệp >10MB).
func snapshotDrifted(sn storage.Snapshot, cur storage.Snapshot) bool {
	if sn.OrigSHA256 == "" || cur.OrigSHA256 == "" {
		return sn.OrigSize != cur.OrigSize || sn.OrigMtimeNS != cur.OrigMtimeNS
	}
	return sn.OrigSHA256 != cur.OrigSHA256
}

// TaskAllowsDelete kiểm tra xem task hiện tại có được mở kèm cờ cho phép xóa tệp (--allow-delete) hay không.
func TaskAllowsDelete(root, taskID string) bool {
	if taskID == "" {
		return false
	}
	_, err := os.Stat(filepath.Join(root, ".sen", "snapshots", taskID, allowDeleteMarker))
	return err == nil
}

// CheckIntegrity đảm bảo không có tệp test/raw bị khóa nào bị sửa đổi và không có tệp ngoài phạm vi bị thêm/sửa/xóa lén.
func CheckIntegrity(st *storage.Store, root, taskID string, scopeFiles []string) error {
	if taskID != "" {
		if err := ValidateTaskID(taskID); err != nil {
			return err
		}
	}
	currentByKey, _, err := scanWorkspaceFiles(root)
	if err != nil {
		return err
	}

	allowedKeys := make(map[string]bool)
	for _, raw := range scopeFiles {
		rel, err := NormalizeRelPath(root, raw)
		if err != nil {
			return err
		}
		if err := ensureNoSymlinkChain(root, rel); err != nil {
			return err
		}
		allowedKeys[PathKey(rel)] = true
	}

	if err := verifyLockedTestsUntouched(st, taskID, currentByKey); err != nil {
		return err
	}

	baseSnaps, err := st.GetSnapshots(BaselineTaskID)
	if err != nil {
		return err
	}
	baseByKey := make(map[string]storage.Snapshot, len(baseSnaps))
	for _, b := range baseSnaps {
		baseByKey[PathKey(b.RelPath)] = b
	}

	// Phát hiện tệp bị chỉnh sửa ngoài phạm vi hoặc tệp lạ mới phát sinh không nằm trong scope_files.
	for key, cur := range currentByKey {
		if base, existed := baseByKey[key]; existed {
			if snapshotDrifted(base, cur) && !allowedKeys[key] {
				return fmt.Errorf("out-of-scope file modified: %s", cur.RelPath)
			}
		} else if !allowedKeys[key] {
			return fmt.Errorf("out-of-scope untracked file created: %s", cur.RelPath)
		}
	}

	// Phát hiện hành vi xóa tệp ngoài phạm vi hoặc xóa tệp trong phạm vi khi chưa khai báo --allow-delete.
	allowDel := TaskAllowsDelete(root, taskID)
	for key, base := range baseByKey {
		if _, exists := currentByKey[key]; !exists {
			if !allowedKeys[key] {
				return fmt.Errorf("out-of-scope file deleted: %s", base.RelPath)
			}
			if !allowDel {
				return fmt.Errorf("scoped file %q deleted without --allow-delete (LARGE risk)", base.RelPath)
			}
		}
	}
	return nil
}

// verifyLockedTestsUntouched kiểm tra toàn bộ tệp kiểm thử và tư liệu gốc bị khóa trong cả __BASELINE__ và snapshot của task.
func verifyLockedTestsUntouched(st *storage.Store, taskID string, currentByKey map[string]storage.Snapshot) error {
	checkIDs := []string{BaselineTaskID}
	if taskID != "" {
		checkIDs = append(checkIDs, taskID)
	}
	for _, tid := range checkIDs {
		snaps, err := st.GetSnapshots(tid)
		if err != nil {
			return err
		}
		for _, sn := range snaps {
			if !sn.IsLockedTest || !sn.ExistedBefore {
				continue
			}
			cur, exists := currentByKey[PathKey(sn.RelPath)]
			if !exists {
				if IsLockedRawMedia(sn.RelPath) {
					return fmt.Errorf("locked raw/master media file deleted: %s", sn.RelPath)
				}
				return fmt.Errorf("locked test file deleted: %s", sn.RelPath)
			}
			if snapshotDrifted(sn, cur) {
				if IsLockedRawMedia(sn.RelPath) {
					return fmt.Errorf("locked raw/master media file modified: %s", sn.RelPath)
				}
				return fmt.Errorf("locked test file modified: %s", sn.RelPath)
			}
		}
	}
	return nil
}

// UndoTask chỉ cho phép hoàn tác task đang OPEN: khôi phục từng byte tệp cũ, xóa tệp mới sinh và đánh dấu UNDONE.
func UndoTask(st *storage.Store, root, taskID string) ([]string, error) {
	if err := ValidateTaskID(taskID); err != nil {
		return nil, err
	}
	task, err := st.GetTask(taskID)
	if err != nil {
		return nil, err
	}
	if task.Status != "OPEN" {
		return nil, fmt.Errorf("cannot undo task %q with status %s: only OPEN tasks can be undone", taskID, task.Status)
	}
	snaps, err := st.GetSnapshots(taskID)
	if err != nil {
		return nil, err
	}
	var restored []string
	for _, sn := range snaps {
		changed, err := restoreSingleSnapshot(root, sn)
		if err != nil {
			return restored, err
		}
		if changed {
			restored = append(restored, sn.RelPath)
		}
	}
	if err := st.UpdateTaskStatus(taskID, "UNDONE"); err != nil {
		return restored, err
	}
	ClearQuarantineReset(root, taskID)
	return restored, nil
}

// restoreSingleSnapshot phục hồi an toàn một tệp đơn lẻ sau khi kiểm tra chuỗi thư mục cha không chứa symlink.
func restoreSingleSnapshot(root string, sn storage.Snapshot) (bool, error) {
	parentRel := filepath.ToSlash(filepath.Dir(filepath.FromSlash(sn.RelPath)))
	if parentRel != "." && parentRel != "" {
		if err := ensureNoSymlinkChain(root, parentRel); err != nil {
			return false, err
		}
	}
	dst := filepath.Join(root, filepath.FromSlash(sn.RelPath))
	linfo, lerr := os.Lstat(dst)

	if !sn.ExistedBefore {
		if lerr == nil {
			if err := os.Remove(dst); err != nil {
				return false, fmt.Errorf("remove created file %s: %w", sn.RelPath, err)
			}
			return true, nil
		}
		return false, nil
	}

	needsRestore := false
	if os.IsNotExist(lerr) {
		needsRestore = true
	} else if lerr != nil {
		return false, lerr
	} else if linfo.Mode()&os.ModeSymlink != 0 || !linfo.Mode().IsRegular() {
		if err := os.Remove(dst); err != nil {
			return false, fmt.Errorf("remove irregular/symlink %s: %w", sn.RelPath, err)
		}
		needsRestore = true
	} else {
		cur, err := buildFileSnapshot(dst, sn.RelPath, linfo)
		if err != nil {
			return false, err
		}
		needsRestore = snapshotDrifted(sn, cur)
	}

	if !needsRestore {
		return false, nil
	}
	if sn.BackupPath == "" {
		return false, fmt.Errorf("cannot restore >10MB file %s (not backed up by policy)", sn.RelPath)
	}
	if err := copyFileBytes(sn.BackupPath, dst); err != nil {
		return false, fmt.Errorf("restore %s: %w", sn.RelPath, err)
	}
	return true, nil
}
