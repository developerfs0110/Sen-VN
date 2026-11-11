// Package storage quản lý sổ cái SQLite WAL 8 bảng chuẩn hóa thuần Go (modernc.org/sqlite, không cần CGO).
package storage

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// MaxRules giới hạn cứng tối đa 20 luật dự án để tránh phình ngữ cảnh (context bloat).
const MaxRules = 20

// Store đóng gói kết nối SQLite đơn luồng tới tệp .sen/state.db của dự án.
type Store struct {
	db *sql.DB
}

// columnMigration định nghĩa một cột bổ sung tương thích ngược cho các cơ sở dữ liệu từ phiên bản cũ.
type columnMigration struct {
	table  string
	column string
	ddl    string
}

// schemaSQL định nghĩa 8 bảng chuẩn hóa: 5 bảng lõi (rules, requirements, tasks, snapshots, evidence) và 3 bảng SSDS.
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
	req_type TEXT NOT NULL DEFAULT 'FUNC',
	user_quote TEXT NOT NULL,
	summary TEXT NOT NULL,
	acceptance_criteria TEXT NOT NULL DEFAULT '',
	doc_sha256 TEXT NOT NULL DEFAULT '',
	is_bugfix INTEGER NOT NULL DEFAULT 0,
	approved INTEGER NOT NULL DEFAULT 0,
	created_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS tasks (
	id TEXT PRIMARY KEY,
	phase_id TEXT NOT NULL DEFAULT 'P5',
	req_ids_json TEXT NOT NULL,
	arch_components_json TEXT NOT NULL DEFAULT '[]',
	title TEXT NOT NULL,
	kind TEXT NOT NULL,
	risk_auto TEXT NOT NULL,
	scope_files_json TEXT NOT NULL,
	status TEXT NOT NULL,
	exec_state TEXT NOT NULL DEFAULT 'IMPLEMENTED',
	verdict TEXT NOT NULL DEFAULT 'NOT_EXECUTED',
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

CREATE TABLE IF NOT EXISTS ssds_project (
	project_id TEXT PRIMARY KEY,
	project_name TEXT NOT NULL DEFAULT '',
	applied_ssds_version TEXT NOT NULL,
	category TEXT NOT NULL,
	workflow_tier TEXT NOT NULL,
	active_phases_json TEXT NOT NULL,
	risk_assessment_json TEXT NOT NULL DEFAULT '{}',
	current_phase TEXT NOT NULL,
	brownfield_audited INTEGER NOT NULL DEFAULT 0,
	release_tag TEXT NOT NULL DEFAULT '',
	created_at TEXT NOT NULL,
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS ssds_phases (
	phase_id TEXT PRIMARY KEY,
	status TEXT NOT NULL,
	primary_role TEXT NOT NULL,
	supporting_roles_json TEXT NOT NULL DEFAULT '[]',
	required_deliverables_json TEXT NOT NULL DEFAULT '[]',
	verified_deliverables_json TEXT NOT NULL DEFAULT '{}',
	requires_user_approval INTEGER NOT NULL DEFAULT 0,
	gate_passed INTEGER NOT NULL DEFAULT 0,
	approved_by TEXT NOT NULL DEFAULT '',
	approved_quote TEXT NOT NULL DEFAULT '',
	approved_at TEXT NOT NULL DEFAULT '',
	updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS ssds_governance (
	record_id TEXT PRIMARY KEY,
	record_kind TEXT NOT NULL,
	phase_id TEXT NOT NULL,
	authority_level TEXT NOT NULL,
	author_role TEXT NOT NULL DEFAULT '',
	reviewer_role TEXT NOT NULL DEFAULT '',
	evidence_mode TEXT NOT NULL,
	verdict TEXT NOT NULL,
	payload_json TEXT NOT NULL DEFAULT '{}',
	approved INTEGER NOT NULL DEFAULT 0,
	expires_at TEXT NOT NULL DEFAULT '',
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

// UserTableNames liệt kê toàn bộ bảng người dùng trong sqlite_master (phục vụ kiểm chứng đúng 8 bảng chuẩn hóa).
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

// Open khởi tạo hoặc mở tệp SQLite ở chế độ WAL với busy_timeout 5000ms, khóa ngoại và nâng cấp cột idempotent.
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
	if err := ensureColumnMigrations(db); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("migrate columns: %w", err)
	}
	return &Store{db: db}, nil
}

// ensureColumnMigrations kiểm tra PRAGMA table_info và bổ sung an toàn các cột SSDS còn thiếu trên dự án cũ.
func ensureColumnMigrations(db *sql.DB) error {
	migrations := []columnMigration{
		{table: "requirements", column: "req_type", ddl: "ALTER TABLE requirements ADD COLUMN req_type TEXT NOT NULL DEFAULT 'FUNC'"},
		{table: "requirements", column: "acceptance_criteria", ddl: "ALTER TABLE requirements ADD COLUMN acceptance_criteria TEXT NOT NULL DEFAULT ''"},
		{table: "requirements", column: "doc_sha256", ddl: "ALTER TABLE requirements ADD COLUMN doc_sha256 TEXT NOT NULL DEFAULT ''"},
		{table: "tasks", column: "phase_id", ddl: "ALTER TABLE tasks ADD COLUMN phase_id TEXT NOT NULL DEFAULT 'P5'"},
		{table: "tasks", column: "arch_components_json", ddl: "ALTER TABLE tasks ADD COLUMN arch_components_json TEXT NOT NULL DEFAULT '[]'"},
		{table: "tasks", column: "exec_state", ddl: "ALTER TABLE tasks ADD COLUMN exec_state TEXT NOT NULL DEFAULT 'IMPLEMENTED'"},
		{table: "tasks", column: "verdict", ddl: "ALTER TABLE tasks ADD COLUMN verdict TEXT NOT NULL DEFAULT 'NOT_EXECUTED'"},
	}

	colCache := make(map[string]map[string]bool)
	for _, m := range migrations {
		cols, ok := colCache[m.table]
		if !ok {
			loaded, err := tableColumnSet(db, m.table)
			if err != nil {
				return err
			}
			cols = loaded
			colCache[m.table] = cols
		}
		if cols[strings.ToLower(m.column)] {
			continue
		}
		if _, err := db.Exec(m.ddl); err != nil {
			return fmt.Errorf("alter %s.%s: %w", m.table, m.column, err)
		}
		cols[strings.ToLower(m.column)] = true
	}
	return nil
}

// tableColumnSet truy vấn danh sách tên cột hiện hữu của một bảng thông qua PRAGMA table_info.
func tableColumnSet(db *sql.DB, tableName string) (map[string]bool, error) {
	rows, err := db.Query(fmt.Sprintf("PRAGMA table_info(%s)", tableName))
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	cols := make(map[string]bool)
	for rows.Next() {
		var cid, notNull, pk int
		var name, colType string
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &colType, &notNull, &dflt, &pk); err != nil {
			return nil, err
		}
		cols[strings.ToLower(name)] = true
	}
	return cols, rows.Err()
}

// nowUTC trả về chuỗi thời gian chuẩn RFC3339 theo múi giờ UTC.
func nowUTC() string {
	return time.Now().UTC().Format(time.RFC3339)
}
