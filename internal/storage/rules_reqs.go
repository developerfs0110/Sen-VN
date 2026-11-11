// Tệp rules_reqs.go quản lý bảng luật dự án (rules <= 20) và bảng yêu cầu truy vết nguyên văn (requirements).
package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/developerfs0110/Sen-VN/internal/redact"
)

// Rule biểu diễn một quy tắc bất biến của người dùng kèm mẫu đường dẫn áp dụng (path_glob).
type Rule struct {
	ID        int64
	RuleText  string
	UserQuote string
	PathGlob  string
	CreatedAt string
}

// Requirement lưu trữ yêu cầu R1..Rn (hoặc R_sec, R_law) gắn với câu nhắc nguyên văn của người dùng.
type Requirement struct {
	ID        string
	UserQuote string
	Summary   string
	IsBugfix  bool
	Approved  bool
	CreatedAt string
}

// AddRule thêm một luật mới vào sổ cái sau khi lọc secret và kiểm tra trần cứng 20 luật trong giao dịch.
func (s *Store) AddRule(ruleText, userQuote, pathGlob string) (*Rule, error) {
	ruleText = redact.ScrubText(strings.TrimSpace(ruleText))
	userQuote = redact.ScrubText(userQuote)
	if ruleText == "" {
		return nil, errors.New("rule_text cannot be empty")
	}
	if strings.TrimSpace(pathGlob) == "" {
		pathGlob = "*"
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM rules`).Scan(&count); err != nil {
		return nil, err
	}
	if count >= MaxRules {
		return nil, fmt.Errorf("rule cap reached (%d/%d): use --replace <id> or --delete <id>", count, MaxRules)
	}
	ts := nowUTC()
	res, err := tx.Exec(`INSERT INTO rules (rule_text, user_quote, path_glob, created_at) VALUES (?, ?, ?, ?)`,
		ruleText, userQuote, pathGlob, ts)
	if err != nil {
		return nil, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &Rule{ID: id, RuleText: ruleText, UserQuote: userQuote, PathGlob: pathGlob, CreatedAt: ts}, nil
}

// ReplaceRule cập nhật nội dung của một luật đã tồn tại theo ID mà không làm tăng tổng số luật.
func (s *Store) ReplaceRule(id int64, ruleText, userQuote, pathGlob string) error {
	ruleText = redact.ScrubText(strings.TrimSpace(ruleText))
	userQuote = redact.ScrubText(userQuote)
	if ruleText == "" {
		return errors.New("rule_text cannot be empty")
	}
	if strings.TrimSpace(pathGlob) == "" {
		pathGlob = "*"
	}
	res, err := s.db.Exec(`UPDATE rules SET rule_text = ?, user_quote = ?, path_glob = ? WHERE id = ?`,
		ruleText, userQuote, pathGlob, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("rule #%d not found", id)
	}
	return nil
}

// DeleteRule xóa một luật dự án theo ID khỏi bảng rules.
func (s *Store) DeleteRule(id int64) error {
	res, err := s.db.Exec(`DELETE FROM rules WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("rule #%d not found", id)
	}
	return nil
}

// ListRules trả về danh sách toàn bộ luật dự án sắp xếp tăng dần theo ID.
func (s *Store) ListRules() ([]Rule, error) {
	rows, err := s.db.Query(`SELECT id, rule_text, user_quote, path_glob, created_at FROM rules ORDER BY id ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Rule
	for rows.Next() {
		var r Rule
		if err := rows.Scan(&r.ID, &r.RuleText, &r.UserQuote, &r.PathGlob, &r.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// AddRequirement ghi mới hoặc cập nhật yêu cầu (bảo toàn cờ approved và câu trích dẫn gốc nếu đã có).
func (s *Store) AddRequirement(explicitID, userQuote, summary string, isBugfix bool) (*Requirement, error) {
	summary = redact.ScrubText(strings.TrimSpace(summary))
	userQuote = redact.ScrubText(strings.TrimSpace(userQuote))
	if summary == "" {
		return nil, errors.New("requirement summary cannot be empty")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	id := strings.TrimSpace(explicitID)
	if id == "" {
		nextID, err := nextRequirementID(tx)
		if err != nil {
			return nil, err
		}
		id = nextID
	}

	ts := nowUTC()
	bugInt := 0
	if isBugfix {
		bugInt = 1
	}
	// Khi ghi đè R_sec/R_law, giữ nguyên trạng thái approved, cờ is_bugfix=1 và user_quote thực tế ban đầu.
	_, err = tx.Exec(`
		INSERT INTO requirements (id, user_quote, summary, is_bugfix, approved, created_at)
		VALUES (?, ?, ?, ?, 0, ?)
		ON CONFLICT(id) DO UPDATE SET
			user_quote = CASE
				WHEN requirements.user_quote != '' AND (excluded.user_quote = '' OR excluded.user_quote = 'companion-compliance-anchor')
				THEN requirements.user_quote
				ELSE excluded.user_quote
			END,
			summary = excluded.summary,
			is_bugfix = CASE WHEN requirements.is_bugfix = 1 THEN 1 ELSE excluded.is_bugfix END
	`, id, userQuote, summary, bugInt, ts)
	if err != nil {
		return nil, err
	}
	var saved Requirement
	var sBug, sApp int
	if err := tx.QueryRow(`SELECT id, user_quote, summary, is_bugfix, approved, created_at FROM requirements WHERE id = ?`, id).
		Scan(&saved.ID, &saved.UserQuote, &saved.Summary, &sBug, &sApp, &saved.CreatedAt); err != nil {
		return nil, err
	}
	saved.IsBugfix = sBug != 0
	saved.Approved = sApp != 0
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &saved, nil
}

// ListRequirements trả về toàn bộ yêu cầu theo đúng thứ tự tiếp nhận ban đầu.
func (s *Store) ListRequirements() ([]Requirement, error) {
	rows, err := s.db.Query(`SELECT id, user_quote, summary, is_bugfix, approved, created_at FROM requirements ORDER BY rowid ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Requirement
	for rows.Next() {
		var r Requirement
		var bug, app int
		if err := rows.Scan(&r.ID, &r.UserQuote, &r.Summary, &bug, &app, &r.CreatedAt); err != nil {
			return nil, err
		}
		r.IsBugfix = bug != 0
		r.Approved = app != 0
		out = append(out, r)
	}
	return out, rows.Err()
}

// ApproveAllRequirements đánh dấu toàn bộ yêu cầu hiện có sang trạng thái đã duyệt (approved = 1).
func (s *Store) ApproveAllRequirements() error {
	_, err := s.db.Exec(`UPDATE requirements SET approved = 1`)
	return err
}

// nextRequirementID sinh mã tuần tự R1, R2, ... dựa trên chỉ số lớn nhất hiện có trong bảng requirements.
func nextRequirementID(tx *sql.Tx) (string, error) {
	rows, err := tx.Query(`SELECT id FROM requirements`)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	maxN := 0
	for rows.Next() {
		var existing string
		if err := rows.Scan(&existing); err != nil {
			return "", err
		}
		if strings.HasPrefix(existing, "R") {
			if n, err := strconv.Atoi(existing[1:]); err == nil && n > maxN {
				maxN = n
			}
		}
	}
	return fmt.Sprintf("R%d", maxN+1), rows.Err()
}
