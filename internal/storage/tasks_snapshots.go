// Tệp tasks_snapshots.go quản lý vòng đời task bất biến (4 trạng thái thực thi SSDS §20) và bảng ảnh chụp tệp tin (snapshots).
package storage

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Task biểu diễn đơn vị công việc gắn với pha SDLC, danh sách yêu cầu, thành phần kiến trúc và trạng thái bằng chứng SSDS §20.
type Task struct {
	ID             string
	PhaseID        string
	ReqIDs         []string
	ArchComponents []string
	Title          string
	Kind           string
	RiskAuto       string
	ScopeFiles     []string
	Status         string
	ExecState      string
	Verdict        string
	CreatedAt      string
	UpdatedAt      string
}

// Snapshot lưu siêu dữ liệu nguyên bản (SHA-256, kích thước, mtime) và đường dẫn sao lưu cho một tệp tin.
type Snapshot struct {
	ID            int64
	TaskID        string
	RelPath       string
	ExistedBefore bool
	OrigSHA256    string
	OrigSize      int64
	OrigMtimeNS   int64
	BackupPath    string
	IsLockedTest  bool
}

// CreateTask chèn mới một task và từ chối tái sử dụng mã task cũ để ngăn đánh tráo bằng chứng kiểm thử.
func (s *Store) CreateTask(t Task) error {
	var existingStatus string
	err := s.db.QueryRow(`SELECT status FROM tasks WHERE id = ?`, t.ID).Scan(&existingStatus)
	if err == nil {
		return fmt.Errorf("task %q already exists (status=%s); task IDs are immutable — choose a new task ID", t.ID, existingStatus)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	phaseID := strings.TrimSpace(t.PhaseID)
	if phaseID == "" {
		phaseID = "P5"
	}
	archComps := t.ArchComponents
	if archComps == nil {
		archComps = []string{}
	}
	execState, err := normalizeTaskExecState(t.ExecState, t.Status)
	if err != nil {
		return err
	}
	verdict, err := normalizeTaskVerdict(t.Verdict, t.Status)
	if err != nil {
		return err
	}

	reqJSON, err := json.Marshal(t.ReqIDs)
	if err != nil {
		return err
	}
	compJSON, err := json.Marshal(archComps)
	if err != nil {
		return err
	}
	filesJSON, err := json.Marshal(t.ScopeFiles)
	if err != nil {
		return err
	}
	ts := nowUTC()
	created := t.CreatedAt
	if created == "" {
		created = ts
	}
	_, err = s.db.Exec(`
		INSERT INTO tasks (
			id, phase_id, req_ids_json, arch_components_json, title, kind, risk_auto,
			scope_files_json, status, exec_state, verdict, created_at, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, t.ID, phaseID, string(reqJSON), string(compJSON), t.Title, t.Kind, t.RiskAuto, string(filesJSON), t.Status, execState, verdict, created, ts)
	return err
}

// SaveTask là bí danh tương thích gọi trực tiếp CreateTask với ràng buộc bất biến mã task.
func (s *Store) SaveTask(t Task) error {
	return s.CreateTask(t)
}

// GetTask truy vấn thông tin chi tiết của một task theo ID kèm giải mã JSON cho req_ids, arch_components và scope_files.
func (s *Store) GetTask(id string) (*Task, error) {
	var t Task
	var reqJSON, compJSON, filesJSON string
	err := s.db.QueryRow(`
		SELECT id, phase_id, req_ids_json, arch_components_json, title, kind, risk_auto,
		       scope_files_json, status, exec_state, verdict, created_at, updated_at
		FROM tasks WHERE id = ?
	`, id).Scan(
		&t.ID, &t.PhaseID, &reqJSON, &compJSON, &t.Title, &t.Kind, &t.RiskAuto,
		&filesJSON, &t.Status, &t.ExecState, &t.Verdict, &t.CreatedAt, &t.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, fmt.Errorf("task %q not found", id)
	}
	if err != nil {
		return nil, err
	}
	_ = json.Unmarshal([]byte(reqJSON), &t.ReqIDs)
	_ = json.Unmarshal([]byte(compJSON), &t.ArchComponents)
	_ = json.Unmarshal([]byte(filesJSON), &t.ScopeFiles)
	return &t, nil
}

// ListTasks liệt kê toàn bộ các task theo thứ tự khởi tạo trong sổ cái.
func (s *Store) ListTasks() ([]Task, error) {
	rows, err := s.db.Query(`
		SELECT id, phase_id, req_ids_json, arch_components_json, title, kind, risk_auto,
		       scope_files_json, status, exec_state, verdict, created_at, updated_at
		FROM tasks ORDER BY rowid ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Task
	for rows.Next() {
		var t Task
		var reqJSON, compJSON, filesJSON string
		if err := rows.Scan(
			&t.ID, &t.PhaseID, &reqJSON, &compJSON, &t.Title, &t.Kind, &t.RiskAuto,
			&filesJSON, &t.Status, &t.ExecState, &t.Verdict, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(reqJSON), &t.ReqIDs)
		_ = json.Unmarshal([]byte(compJSON), &t.ArchComponents)
		_ = json.Unmarshal([]byte(filesJSON), &t.ScopeFiles)
		out = append(out, t)
	}
	return out, rows.Err()
}

// UpdateTaskStatus cập nhật trạng thái của task (ví dụ OPEN -> VERIFIED hoặc UNDONE) và đồng bộ mặc định exec_state/verdict.
func (s *Store) UpdateTaskStatus(id, status string) error {
	res, err := s.db.Exec(`
		UPDATE tasks SET
			status = ?,
			exec_state = CASE WHEN ? = 'VERIFIED' AND exec_state = 'IMPLEMENTED' THEN 'VERIFIED' ELSE exec_state END,
			verdict = CASE WHEN ? = 'VERIFIED' AND verdict = 'NOT_EXECUTED' THEN 'PASS' ELSE verdict END,
			updated_at = ?
		WHERE id = ?
	`, status, status, status, nowUTC(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("task %q not found", id)
	}
	return nil
}

// UpdateTaskExecutionState cập nhật chi tiết 4 trạng thái thực thi (IMPLEMENTED..VERIFIED) và 6 nhãn kết luận SSDS §20 của task.
func (s *Store) UpdateTaskExecutionState(id, execState, verdict, taskStatus string) error {
	normExec, err := normalizeTaskExecState(execState, "")
	if err != nil {
		return err
	}
	normVerdict, err := normalizeTaskVerdict(verdict, "")
	if err != nil {
		return err
	}
	cleanStatus := strings.TrimSpace(taskStatus)
	res, err := s.db.Exec(`
		UPDATE tasks SET
			exec_state = ?,
			verdict = ?,
			status = CASE WHEN ? != '' THEN ? ELSE status END,
			updated_at = ?
		WHERE id = ?
	`, normExec, normVerdict, cleanStatus, cleanStatus, nowUTC(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("task %q not found", id)
	}
	return nil
}

// normalizeTaskExecState kiểm tra và chuẩn hóa trạng thái thực thi thuộc tập IMPLEMENTED, EXECUTED, TESTED, VERIFIED, BLOCKED.
func normalizeTaskExecState(raw, fallbackStatus string) (string, error) {
	v := strings.ToUpper(strings.TrimSpace(raw))
	if v == "" {
		if strings.EqualFold(strings.TrimSpace(fallbackStatus), "VERIFIED") {
			return "VERIFIED", nil
		}
		return "IMPLEMENTED", nil
	}
	switch v {
	case "IMPLEMENTED", "EXECUTED", "TESTED", "VERIFIED", "BLOCKED":
		return v, nil
	default:
		return "", fmt.Errorf("invalid task exec_state %q: must be IMPLEMENTED, EXECUTED, TESTED, VERIFIED, or BLOCKED", raw)
	}
}

// normalizeTaskVerdict chuẩn hóa nhãn kết luận SSDS §20 thuộc tập PASS, FAIL, NOT_EXECUTED, NOT_VERIFIED, BLOCKED, NOT_APPLICABLE.
func normalizeTaskVerdict(raw, fallbackStatus string) (string, error) {
	v := strings.ReplaceAll(strings.ToUpper(strings.TrimSpace(raw)), " ", "_")
	if v == "" {
		if strings.EqualFold(strings.TrimSpace(fallbackStatus), "VERIFIED") {
			return "PASS", nil
		}
		return "NOT_EXECUTED", nil
	}
	switch v {
	case "PASS", "FAIL", "NOT_EXECUTED", "NOT_VERIFIED", "BLOCKED", "NOT_APPLICABLE":
		return v, nil
	default:
		return "", fmt.Errorf("invalid task verdict %q: must be PASS, FAIL, NOT_EXECUTED, NOT_VERIFIED, BLOCKED, or NOT_APPLICABLE", raw)
	}
}

// ReplaceSnapshots ghi thay thế nguyên tử toàn bộ danh sách snapshot của một task_id (hoặc __BASELINE__).
func (s *Store) ReplaceSnapshots(taskID string, snaps []Snapshot) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.Exec(`DELETE FROM snapshots WHERE task_id = ?`, taskID); err != nil {
		return err
	}
	stmt, err := tx.Prepare(`
		INSERT INTO snapshots (task_id, rel_path, existed_before, orig_sha256, orig_size, orig_mtime_ns, backup_path, is_locked_test)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, sn := range snaps {
		if _, err := stmt.Exec(taskID, sn.RelPath, boolToInt(sn.ExistedBefore), sn.OrigSHA256, sn.OrigSize, sn.OrigMtimeNS, sn.BackupPath, boolToInt(sn.IsLockedTest)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// UpsertSnapshotEntry cập nhật một mục snapshot đơn lẻ (không phân biệt hoa thường trên đường dẫn) khi nâng mốc nền.
func (s *Store) UpsertSnapshotEntry(sn Snapshot) error {
	_, err := s.db.Exec(`DELETE FROM snapshots WHERE task_id = ? AND LOWER(rel_path) = ?`, sn.TaskID, strings.ToLower(sn.RelPath))
	if err != nil {
		return err
	}
	_, err = s.db.Exec(`
		INSERT INTO snapshots (task_id, rel_path, existed_before, orig_sha256, orig_size, orig_mtime_ns, backup_path, is_locked_test)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?)
	`, sn.TaskID, sn.RelPath, boolToInt(sn.ExistedBefore), sn.OrigSHA256, sn.OrigSize, sn.OrigMtimeNS, sn.BackupPath, boolToInt(sn.IsLockedTest))
	return err
}

// DeleteSnapshotEntry xóa mục snapshot tương ứng khi một tệp trong phạm vi được phép xóa hợp lệ sau khi VERIFIED.
func (s *Store) DeleteSnapshotEntry(taskID, relPath string) error {
	_, err := s.db.Exec(`DELETE FROM snapshots WHERE task_id = ? AND LOWER(rel_path) = ?`, taskID, strings.ToLower(relPath))
	return err
}

// GetSnapshots trả về danh sách các bản ghi snapshot của một task_id sắp xếp theo đường dẫn tương đối.
func (s *Store) GetSnapshots(taskID string) ([]Snapshot, error) {
	rows, err := s.db.Query(`
		SELECT id, task_id, rel_path, existed_before, orig_sha256, orig_size, orig_mtime_ns, backup_path, is_locked_test
		FROM snapshots WHERE task_id = ? ORDER BY rel_path ASC
	`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Snapshot
	for rows.Next() {
		var sn Snapshot
		var existed, locked int
		if err := rows.Scan(&sn.ID, &sn.TaskID, &sn.RelPath, &existed, &sn.OrigSHA256, &sn.OrigSize, &sn.OrigMtimeNS, &sn.BackupPath, &locked); err != nil {
			return nil, err
		}
		sn.ExistedBefore = existed != 0
		sn.IsLockedTest = locked != 0
		out = append(out, sn)
	}
	return out, rows.Err()
}

// boolToInt chuyển đổi giá trị kiểu bool sang số nguyên 0/1 lưu trong SQLite.
func boolToInt(v bool) int {
	if v {
		return 1
	}
	return 0
}
