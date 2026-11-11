// Package storage quản lý sổ cái SQLite WAL 5 bảng thuần Go (modernc.org/sqlite, không cần CGO).
package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"time"

	_ "modernc.org/sqlite"
)

// MaxRules giới hạn cứng tối đa 20 luật dự án để tránh phình ngữ cảnh (context bloat).
const MaxRules = 20

// Store đóng gói kết nối SQLite đơn luồng tới tệp .sen/state.db của dự án.
type Store struct {
	db *sql.DB
}

// schemaSQL định nghĩa đúng 5 bảng cố định: rules, requirements, tasks, snapshots, evidence.
const schemaSQL = `
CREATE TABLE IF NOT EXISTS rules (
	id INTEGER PRIMARY KEY,
	rule_text TEXT NOT NULL,
	user_quote TEXT NOT NULL DEFAULT '',
	path_glob TEXT NOT NULL DEFAULT '*',
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS requirements (
	id TEXT PRIMARY KEY,
	user_quote TEXT NOT NULL,
	summary TEXT NOT NULL,
	is_bugfix INTEGER NOT NULL DEFAULT 0,
	approved INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS tasks (
	id TEXT PRIMARY KEY,
	req_ids_json TEXT NOT NULL,
	title TEXT NOT NULL,
	kind TEXT NOT NULL,
	risk_auto TEXT NOT NULL,
	scope_files_json TEXT NOT NULL,
	status TEXT NOT NULL,
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS snapshots (
	id INTEGER PRIMARY KEY,
	task_id TEXT NOT NULL,
	rel_path TEXT NOT NULL,
	existed_before INTEGER NOT NULL,
	orig_sha256 TEXT NOT NULL DEFAULT '',
	orig_size INTEGER NOT NULL DEFAULT 0,
	orig_mtime_ns INTEGER NOT NULL DEFAULT 0,
	backup_path TEXT NOT NULL DEFAULT '',
	is_locked_test INTEGER NOT NULL DEFAULT 0,
	UNIQUE(task_id, rel_path)
);

CREATE TABLE IF NOT EXISTS evidence (
	seq INTEGER PRIMARY KEY,
	task_id TEXT NOT NULL,
	command TEXT NOT NULL,
	strength TEXT NOT NULL,
	tests_ran INTEGER NOT NULL DEFAULT 0,
	exit_code INTEGER NOT NULL,
	redacted_output TEXT NOT NULL,
	prev_hash TEXT NOT NULL,
	entry_hash TEXT NOT NULL,
	created_at TEXT NOT NULL
);
`

// Close đóng kết nối cơ sở dữ liệu SQLite một cách an toàn.
func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

// DB trả về con trỏ *sql.DB bên dưới để phục vụ truy vấn tổng hợp tại cổng nghiệm thu done.
func (s *Store) DB() *sql.DB { return s.db }

// UserTableNames liệt kê toàn bộ bảng người dùng trong sqlite_master (phục vụ kiểm chứng bất biến đúng 5 bảng).
func (s *Store) UserTableNames() ([]string, error) {
	rows, err := s.db.Query(`SELECT name FROM sqlite_master WHERE type = 'table' AND name NOT LIKE 'sqlite_%' ORDER BY name ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		out = append(out, name)
	}
	return out, rows.Err()
}

// Open khởi tạo hoặc mở tệp SQLite ở chế độ WAL với busy_timeout 5000ms và khóa ngoại bật sẵn.
func Open(dbPath string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return nil, fmt.Errorf("mkdir db dir: %w", err)
	}
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("open sqlite: %w", err)
	}
	db.SetMaxOpenConns(1)

	pragmas := []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA busy_timeout = 5000;",
		"PRAGMA foreign_keys = ON;",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("pragma %q: %w", p, err)
		}
	}
	if _, err := db.Exec(schemaSQL); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("init schema: %w", err)
	}
	return &Store{db: db}, nil
}

// nowUTC trả về chuỗi thời gian chuẩn RFC3339 theo múi giờ UTC.
func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}
