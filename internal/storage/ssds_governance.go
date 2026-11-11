// Tệp ssds_governance.go quản lý chuỗi băm SHA-256 chống sửa trộm cho bảng ssds_governance (REVIEW, DECISION, CIA, EXCEPTION).
package storage

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/developerfs0110/Sen-VN/internal/redact"
)

// GovernanceRecord lưu trữ biên bản REVIEW, DECISION, CHANGE_IMPACT, EXCEPTION được nối chuỗi băm SHA-256 từ GENESIS.
type GovernanceRecord struct {
	RecordID       string
	RecordKind     string
	PhaseID        string
	AuthorityLevel string
	AuthorRole     string
	ReviewerRole   string
	EvidenceMode   string
	Verdict        string
	PayloadJSON    string
	Approved       bool
	ExpiresAt      string
	PrevHash       string
	EntryHash      string
	CreatedAt      string
}

// AppendGovernanceRecord nối thêm một bản ghi quản trị mới vào chuỗi băm bất biến ssds_governance từ mốc GENESIS.
func (s *Store) AppendGovernanceRecord(rec GovernanceRecord) (*GovernanceRecord, error) {
	rec.RecordKind = strings.ToUpper(strings.TrimSpace(rec.RecordKind))
	rec.PhaseID = strings.TrimSpace(rec.PhaseID)
	rec.AuthorityLevel = strings.ToUpper(strings.TrimSpace(rec.AuthorityLevel))
	rec.EvidenceMode = strings.ToUpper(strings.TrimSpace(rec.EvidenceMode))
	rec.Verdict = strings.ReplaceAll(strings.ToUpper(strings.TrimSpace(rec.Verdict)), " ", "_")
	if rec.RecordKind == "" || rec.PhaseID == "" || rec.AuthorityLevel == "" || rec.EvidenceMode == "" || rec.Verdict == "" {
		return nil, errors.New("record_kind, phase_id, authority_level, evidence_mode, and verdict are required")
	}
	payload := redact.ScrubText(strings.TrimSpace(rec.PayloadJSON))
	if payload == "" {
		payload = "{}"
	}
	rec.PayloadJSON = payload

	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	if strings.TrimSpace(rec.RecordID) == "" {
		nextID, err := nextGovernanceID(tx, rec.RecordKind)
		if err != nil {
			return nil, err
		}
		rec.RecordID = nextID
	}

	prevHash := "GENESIS"
	var lastHash string
	err = tx.QueryRow(`SELECT entry_hash FROM ssds_governance ORDER BY rowid DESC LIMIT 1`).Scan(&lastHash)
	if err == nil {
		prevHash = lastHash
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}

	rec.PrevHash = prevHash
	if rec.CreatedAt == "" {
		rec.CreatedAt = nowUTC()
	}
	rec.EntryHash = ComputeGovernanceHash(rec)

	_, err = tx.Exec(`
		INSERT INTO ssds_governance (
			record_id, record_kind, phase_id, authority_level, author_role, reviewer_role,
			evidence_mode, verdict, payload_json, approved, expires_at, prev_hash, entry_hash, created_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, rec.RecordID, rec.RecordKind, rec.PhaseID, rec.AuthorityLevel, rec.AuthorRole, rec.ReviewerRole,
		rec.EvidenceMode, rec.Verdict, rec.PayloadJSON, boolToInt(rec.Approved), rec.ExpiresAt, rec.PrevHash, rec.EntryHash, rec.CreatedAt)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &rec, nil
}

// ListGovernanceRecords truy vấn danh sách hồ sơ quản trị theo bộ lọc loại bản ghi và mã pha (truyền rỗng để lấy tất cả).
func (s *Store) ListGovernanceRecords(kindFilter, phaseFilter string) ([]GovernanceRecord, error) {
	query := `
		SELECT record_id, record_kind, phase_id, authority_level, author_role, reviewer_role,
		       evidence_mode, verdict, payload_json, approved, expires_at, prev_hash, entry_hash, created_at
		FROM ssds_governance WHERE 1=1`
	var args []any
	if k := strings.ToUpper(strings.TrimSpace(kindFilter)); k != "" {
		query += ` AND record_kind = ?`
		args = append(args, k)
	}
	if p := strings.TrimSpace(phaseFilter); p != "" {
		query += ` AND phase_id = ?`
		args = append(args, p)
	}
	query += ` ORDER BY rowid ASC`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []GovernanceRecord
	for rows.Next() {
		var r GovernanceRecord
		var app int
		if err := rows.Scan(
			&r.RecordID, &r.RecordKind, &r.PhaseID, &r.AuthorityLevel, &r.AuthorRole, &r.ReviewerRole,
			&r.EvidenceMode, &r.Verdict, &r.PayloadJSON, &app, &r.ExpiresAt, &r.PrevHash, &r.EntryHash, &r.CreatedAt,
		); err != nil {
			return nil, err
		}
		r.Approved = app != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// ApproveGovernanceRecord phê duyệt một bản ghi quản trị kèm câu trích dẫn tường minh và niêm phong lại chuỗi băm trong giao dịch.
func (s *Store) ApproveGovernanceRecord(recordID, approverQuote string) error {
	cleanID := strings.TrimSpace(recordID)
	quote := redact.ScrubText(strings.TrimSpace(approverQuote))
	if cleanID == "" {
		return errors.New("record_id cannot be empty")
	}
	if quote == "" {
		return errors.New("SSDS §21 violation: approverQuote cannot be empty (silence is not approval)")
	}
	if err := s.VerifyGovernanceChain(); err != nil {
		return fmt.Errorf("cannot approve on tampered governance chain: %w", err)
	}

	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	rows, err := tx.Query(`
		SELECT rowid, record_id, record_kind, phase_id, authority_level, author_role, reviewer_role,
		       evidence_mode, verdict, payload_json, approved, expires_at, prev_hash, entry_hash, created_at
		FROM ssds_governance ORDER BY rowid ASC
	`)
	if err != nil {
		return err
	}
	type rowItem struct {
		rowID int64
		rec   GovernanceRecord
	}
	var items []rowItem
	found := false
	for rows.Next() {
		var it rowItem
		var app int
		if err := rows.Scan(
			&it.rowID, &it.rec.RecordID, &it.rec.RecordKind, &it.rec.PhaseID, &it.rec.AuthorityLevel,
			&it.rec.AuthorRole, &it.rec.ReviewerRole, &it.rec.EvidenceMode, &it.rec.Verdict,
			&it.rec.PayloadJSON, &app, &it.rec.ExpiresAt, &it.rec.PrevHash, &it.rec.EntryHash, &it.rec.CreatedAt,
		); err != nil {
			rows.Close()
			return err
		}
		it.rec.Approved = app != 0
		if it.rec.RecordID == cleanID {
			found = true
			it.rec.Approved = true
			it.rec.PayloadJSON = mergeApprovalIntoPayload(it.rec.PayloadJSON, quote)
		}
		items = append(items, it)
	}
	rows.Close()
	if !found {
		return fmt.Errorf("governance record %q not found", cleanID)
	}

	prev := "GENESIS"
	for _, it := range items {
		it.rec.PrevHash = prev
		it.rec.EntryHash = ComputeGovernanceHash(it.rec)
		prev = it.rec.EntryHash
		if _, err := tx.Exec(`
			UPDATE ssds_governance
			SET payload_json = ?, approved = ?, prev_hash = ?, entry_hash = ?
			WHERE rowid = ?
		`, it.rec.PayloadJSON, boolToInt(it.rec.Approved), it.rec.PrevHash, it.rec.EntryHash, it.rowID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// VerifyGovernanceChain đối chiếu tính toàn vẹn của chuỗi băm SHA-256 trên toàn bộ bảng ssds_governance từ mốc GENESIS.
func (s *Store) VerifyGovernanceChain() error {
	list, err := s.ListGovernanceRecords("", "")
	if err != nil {
		return err
	}
	expectedPrev := "GENESIS"
	for _, rec := range list {
		if rec.PrevHash != expectedPrev {
			return fmt.Errorf("governance %s prev_hash mismatch: got %s, expected %s", rec.RecordID, rec.PrevHash, expectedPrev)
		}
		want := ComputeGovernanceHash(rec)
		if rec.EntryHash != want {
			return fmt.Errorf("governance %s entry_hash tampered: stored %s != computed %s", rec.RecordID, rec.EntryHash, want)
		}
		expectedPrev = rec.EntryHash
	}
	return nil
}

// ComputeGovernanceHash băm mảng JSON chuẩn hóa bao phủ mọi trường dữ liệu của bản ghi ssds_governance.
func ComputeGovernanceHash(rec GovernanceRecord) string {
	payload, _ := json.Marshal([]any{
		rec.PrevHash, rec.RecordID, rec.RecordKind, rec.PhaseID, rec.AuthorityLevel,
		rec.AuthorRole, rec.ReviewerRole, rec.EvidenceMode, rec.Verdict,
		rec.PayloadJSON, boolToInt(rec.Approved), rec.ExpiresAt, rec.CreatedAt,
	})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

// mergeApprovalIntoPayload chèn thông tin câu trích dẫn phê duyệt và dấu thời gian vào chuỗi JSON của bản ghi.
func mergeApprovalIntoPayload(rawJSON, quote string) string {
	obj := make(map[string]any)
	_ = json.Unmarshal([]byte(rawJSON), &obj)
	obj["approver_quote"] = quote
	obj["approved_at"] = nowUTC()
	out, err := json.Marshal(obj)
	if err != nil {
		return rawJSON
	}
	return string(out)
}

// nextGovernanceID sinh mã tuần tự REV-001, DEC-001, CIA-001, EXC-001 tương ứng với loại hồ sơ quản trị.
func nextGovernanceID(tx *sql.Tx, kind string) (string, error) {
	prefix := "GOV-"
	switch kind {
	case "REVIEW":
		prefix = "REV-"
	case "DECISION":
		prefix = "DEC-"
	case "CHANGE_IMPACT":
		prefix = "CIA-"
	case "EXCEPTION":
		prefix = "EXC-"
	}
	rows, err := tx.Query(`SELECT record_id FROM ssds_governance WHERE record_id LIKE ?`, prefix+"%")
	if err != nil {
		return "", err
	}
	defer rows.Close()
	maxN := 0
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", err
		}
		if n, err := strconv.Atoi(strings.TrimPrefix(id, prefix)); err == nil && n > maxN {
			maxN = n
		}
	}
	return fmt.Sprintf("%s%03d", prefix, maxN+1), rows.Err()
}

