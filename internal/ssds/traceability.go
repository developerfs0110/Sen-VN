// Tệp traceability.go kiểm toán chuỗi truy vết 6 mắt xích (§16) và kiểm tra trôi lệch mã băm tài liệu baseline (§17).
package ssds

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/developerfs0110/Sen-VN/internal/storage"
)

// TraceabilityRow biểu diễn 1 dòng liên kết 6 mắt xích chuẩn SSDS §16: Requirement -> Architecture -> Task -> Source -> Test -> Release.
type TraceabilityRow struct {
	ReqID      string `json:"req_id"`
	Component  string `json:"component"`
	TaskID     string `json:"task_id"`
	SourceFile string `json:"source_file"`
	TestCase   string `json:"test_case"`
	Release    string `json:"release"`
}

var reReqToken = regexp.MustCompile(`(?i)\bREQ-(?:F|NF|BR|CON)-\d+\b`)

// ParseTraceabilityMatrix giải mã tệp docs/traceability-matrix.md ở cả định dạng bảng Markdown 6 cột và chuỗi mũi tên (§16).
func ParseTraceabilityMatrix(content string) ([]TraceabilityRow, error) {
	var rows []TraceabilityRow
	lines := strings.Split(strings.ReplaceAll(content, "\r\n", "\n"), "\n")

	for i := 0; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		// 1. Dạng bảng Markdown: | REQ-F-001 | COMP-01 | TASK-01 | src/a.go | TEST-01 | v1.0.0 |
		if strings.HasPrefix(line, "|") && strings.HasSuffix(line, "|") {
			cells := splitMarkdownTableRow(line)
			if len(cells) >= 6 && reReqToken.MatchString(cells[0]) {
				rows = append(rows, TraceabilityRow{
					ReqID:      strings.ToUpper(reReqToken.FindString(cells[0])),
					Component:  cleanCellValue(cells[1]),
					TaskID:     cleanCellValue(cells[2]),
					SourceFile: cleanCellValue(cells[3]),
					TestCase:   cleanCellValue(cells[4]),
					Release:    cleanCellValue(cells[5]),
				})
			}
			continue
		}

		// 2. Dạng mũi tên một dòng hoặc nhiều dòng của SSDS §16: REQ-F-001 -> Component AUTH-001 -> TASK-025 -> ...
		if reReqToken.MatchString(line) && !strings.HasPrefix(line, "#") {
			combined := line
			for i+1 < len(lines) {
				next := strings.TrimSpace(lines[i+1])
				if strings.HasPrefix(next, "→") || strings.HasPrefix(next, "->") {
					combined += " " + next
					i++
				} else {
					break
				}
			}
			norm := strings.ReplaceAll(combined, "→", "->")
			parts := strings.Split(norm, "->")
			if len(parts) >= 6 {
				rows = append(rows, TraceabilityRow{
					ReqID:      strings.ToUpper(reReqToken.FindString(parts[0])),
					Component:  cleanCellValue(parts[1]),
					TaskID:     cleanCellValue(parts[2]),
					SourceFile: cleanCellValue(parts[3]),
					TestCase:   cleanCellValue(parts[4]),
					Release:    cleanCellValue(parts[5]),
				})
			}
		}
	}
	if len(rows) == 0 {
		return nil, errors.New("no valid 6-link traceability entries found in docs/traceability-matrix.md")
	}
	return rows, nil
}

// ValidateTraceabilityMatrix đối chiếu ma trận truy vết trên đĩa với danh sách REQ-F/NF trong SQLite và tệp mã nguồn thực tế.
func ValidateTraceabilityMatrix(
	st *storage.Store, projectRoot string, requireConcreteRelease bool,
) ([]TraceabilityRow, []string) {
	var violations []string
	matrixPath := filepath.Join(projectRoot, "docs", "traceability-matrix.md")
	rawBytes, err := os.ReadFile(matrixPath)
	if err != nil {
		return nil, []string{"missing required traceability matrix: docs/traceability-matrix.md"}
	}
	rows, err := ParseTraceabilityMatrix(string(rawBytes))
	if err != nil {
		return nil, []string{fmt.Sprintf("SSDS §16 traceability parse failure: %v", err)}
	}

	byReq := make(map[string]TraceabilityRow, len(rows))
	for _, r := range rows {
		byReq[r.ReqID] = r
		if isPlaceholderValue(r.Component) || isPlaceholderValue(r.TaskID) ||
			isPlaceholderValue(r.SourceFile) || isPlaceholderValue(r.TestCase) || isPlaceholderValue(r.Release) {
			violations = append(violations, fmt.Sprintf("SSDS §16 incomplete 6-link row for %s (all 6 links must be non-placeholder)", r.ReqID))
			continue
		}
		if requireConcreteRelease && strings.Contains(strings.ToUpper(r.Release), "PENDING") {
			violations = append(violations, fmt.Sprintf("SSDS §16 release link for %s is still %q", r.ReqID, r.Release))
		}
		if !sourceLinkExistsOnDisk(projectRoot, r.SourceFile) {
			violations = append(violations, fmt.Sprintf("SSDS §16 source file link %q for %s does not exist on disk", r.SourceFile, r.ReqID))
		}
	}

	reqs, err := st.ListRequirements()
	if err == nil {
		for _, req := range reqs {
			upper := strings.ToUpper(req.ID)
			if strings.HasPrefix(upper, "REQ-F-") || strings.HasPrefix(upper, "REQ-NF-") {
				if _, covered := byReq[upper]; !covered {
					violations = append(violations, fmt.Sprintf("SSDS §16 traceability gap: critical requirement %s is omitted from docs/traceability-matrix.md", req.ID))
				}
			}
		}
	}
	return rows, violations
}

// VerifyApprovedBaselineIntegrity phát hiện tài liệu P1/P2 bị sửa lệch mã băm đã niêm phong mà chưa qua lại cổng gate + phê duyệt lại (§17).
func VerifyApprovedBaselineIntegrity(st *storage.Store, projectRoot string) []string {
	var violations []string
	for _, pid := range []string{"P1", "P2"} {
		ph, err := st.GetSSDSPhase(pid)
		if err != nil {
			continue
		}
		if ph.Status == "REVIEW_REQUIRED" && len(ph.VerifiedDeliverables) > 0 {
			violations = append(violations, fmt.Sprintf("updated baseline phase %s is in REVIEW_REQUIRED and requires explicit re-approval (`sen sdlc approve --phase %s`)", pid, pid))
			continue
		}
		if ph.Status != "APPROVED" && ph.Status != "COMPLETED" {
			continue
		}
		for rel, wantHash := range ph.VerifiedDeliverables {
			if wantHash == "" {
				continue
			}
			curHash, hErr := hashDeliverableFile(filepath.Join(projectRoot, filepath.FromSlash(rel)))
			if hErr != nil {
				violations = append(violations, fmt.Sprintf("approved baseline %s (%s) missing on disk", rel, pid))
				continue
			}
			if curHash != wantHash {
				okCIA, _ := hasAuthorizedChangeImpact(st, strings.ToLower(filepath.ToSlash(rel)), wantHash)
				if !okCIA {
					violations = append(violations, fmt.Sprintf("approved baseline %s (%s) drifted from sealed SHA-256 without authorized CHANGE_IMPACT (CIA-*)", rel, pid))
				} else {
					violations = append(violations, fmt.Sprintf("approved baseline %s (%s) modified under CIA-* must be re-sealed via `sen sdlc gate %s` and `sen sdlc approve --phase %s`", rel, pid, pid, pid))
				}
			}
		}
	}
	return violations
}

// sourceLinkExistsOnDisk xác minh ít nhất một tệp mã nguồn được liệt kê trong cột Source Code thực sự tồn tại trên đĩa.
func sourceLinkExistsOnDisk(projectRoot, rawCell string) bool {
	if strings.TrimSpace(projectRoot) == "" {
		return true
	}
	parts := strings.FieldsFunc(rawCell, func(r rune) bool { return r == ',' || r == ';' })
	for _, part := range parts {
		clean := strings.Trim(strings.TrimSpace(part), "`\"'")
		if clean == "" {
			continue
		}
		info, err := os.Stat(filepath.Join(projectRoot, filepath.FromSlash(clean)))
		if err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

// splitMarkdownTableRow tách một dòng bảng Markdown thành danh sách các ô đã cắt khoảng trắng.
func splitMarkdownTableRow(line string) []string {
	trimmed := strings.Trim(line, "|")
	raw := strings.Split(trimmed, "|")
	out := make([]string, 0, len(raw))
	for _, c := range raw {
		out = append(out, strings.TrimSpace(c))
	}
	return out
}

// cleanCellValue loại bỏ ký tự bọc mã (`...`) và khoảng trắng dư thừa trong ô dữ liệu ma trận truy vết.
func cleanCellValue(s string) string {
	return strings.Trim(strings.TrimSpace(s), "`")
}

// isPlaceholderValue nhận diện nội dung rỗng hoặc chỉ điền giữ chỗ (TBD, TODO, -, ..., N/A).
func isPlaceholderValue(s string) bool {
	v := strings.ToUpper(strings.Trim(strings.TrimSpace(s), "`. -_*"))
	switch v {
	case "", "TBD", "TODO", "WIP", "FIXME", "N/A", "NA":
		return true
	default:
		return false
	}
}

// hashDeliverableFile tính mã băm SHA-256 thập lục phân của một tệp bàn giao trên đĩa.
func hashDeliverableFile(fullPath string) (string, error) {
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

