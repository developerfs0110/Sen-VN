// Tệp evidence.go quản lý chuỗi băm SHA-256 chống giả mạo cho bằng chứng kiểm thử (bảng evidence).
package storage

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
)

// Evidence lưu trữ một bản ghi kết quả chạy kiểm thử bất biến được nối mắt xích từ mốc GENESIS.
type Evidence struct {
	Seq            int64
	TaskID         string
	Command        string
	Strength       string
	TestsRan       int
	ExitCode       int
	RedactedOutput string
	PrevHash       string
	EntryHash      string
	CreatedAt      string
}

// AppendEvidence nối thêm một mắt xích bằng chứng mới vào cuối chuỗi băm trong giao dịch SQLite.
func (s *Store) AppendEvidence(taskID, command, strength string, testsRan, exitCode int, redactedOutput string) (*Evidence, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	var lastSeq int64
	prevHash := "GENESIS"
	var lastHash string
	err = tx.QueryRow(`SELECT seq, entry_hash FROM evidence ORDER BY seq DESC LIMIT 1`).Scan(&lastSeq, &lastHash)
	if err == nil {
		prevHash = lastHash
	} else if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	nextSeq := lastSeq + 1

	entryHash := ComputeEvidenceHash(nextSeq, prevHash, taskID, command, strength, testsRan, exitCode, redactedOutput)
	ts := nowUTC()
	_, err = tx.Exec(`
		INSERT INTO evidence (seq, task_id, command, strength, tests_ran, exit_code, redacted_output, prev_hash, entry_hash, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, nextSeq, taskID, command, strength, testsRan, exitCode, redactedOutput, prevHash, entryHash, ts)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &Evidence{
		Seq: nextSeq, TaskID: taskID, Command: command, Strength: strength,
		TestsRan: testsRan, ExitCode: exitCode, RedactedOutput: redactedOutput,
		PrevHash: prevHash, EntryHash: entryHash, CreatedAt: ts,
	}, nil
}

// ListEvidence đọc toàn bộ chuỗi bằng chứng theo số thứ tự seq tăng dần.
func (s *Store) ListEvidence() ([]Evidence, error) {
	rows, err := s.db.Query(`SELECT seq, task_id, command, strength, tests_ran, exit_code, redacted_output, prev_hash, entry_hash, created_at FROM evidence ORDER BY seq ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Evidence
	for rows.Next() {
		var ev Evidence
		if err := rows.Scan(&ev.Seq, &ev.TaskID, &ev.Command, &ev.Strength, &ev.TestsRan, &ev.ExitCode, &ev.RedactedOutput, &ev.PrevHash, &ev.EntryHash, &ev.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, ev)
	}
	return out, rows.Err()
}

// VerifyEvidenceChain kiểm tra tính liên tục của seq và đối chiếu lại toàn bộ mã băm SHA-256 từ GENESIS.
func (s *Store) VerifyEvidenceChain() error {
	list, err := s.ListEvidence()
	if err != nil {
		return err
	}
	expectedPrev := "GENESIS"
	for i, ev := range list {
		if ev.Seq != int64(i+1) {
			return fmt.Errorf("evidence sequence gap at index %d: got seq %d, expected %d", i, ev.Seq, i+1)
		}
		if ev.PrevHash != expectedPrev {
			return fmt.Errorf("evidence #%d prev_hash mismatch: got %s, expected %s", ev.Seq, ev.PrevHash, expectedPrev)
		}
		want := ComputeEvidenceHash(ev.Seq, ev.PrevHash, ev.TaskID, ev.Command, ev.Strength, ev.TestsRan, ev.ExitCode, ev.RedactedOutput)
		if ev.EntryHash != want {
			return fmt.Errorf("evidence #%d entry_hash tampered: stored %s != computed %s", ev.Seq, ev.EntryHash, want)
		}
		expectedPrev = ev.EntryHash
	}
	return nil
}

// ComputeEvidenceHash băm mảng JSON chuẩn hóa bao phủ toàn bộ 8 trường dữ liệu để phát hiện mọi hành vi sửa trộm.
func ComputeEvidenceHash(seq int64, prevHash, taskID, command, strength string, testsRan, exitCode int, redactedOutput string) string {
	payload, _ := json.Marshal([]any{seq, prevHash, taskID, command, strength, testsRan, exitCode, redactedOutput})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}
