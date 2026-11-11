// Tệp ssds_helpers_test.go cung cấp các hàm tạo tài liệu chuẩn và gọi Hook JSON dùng chung cho 7 bộ kiểm thử tuân thủ SSDS.
package senmvp_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/developerfs0110/Sen-VN/internal/bridge"
	"github.com/developerfs0110/Sen-VN/internal/storage"
)

// invokeHookJSON gửi payload JSON trực tiếp vào bộ điều phối HandleAntigravityHookWithEvent và giải mã phản hồi.
func invokeHookJSON(t *testing.T, root, event string, payload map[string]any) (string, map[string]any) {
	t.Helper()
	rawIn, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal hook payload: %v", err)
	}
	var st *storage.Store
	dbPath := filepath.Join(root, ".sen", "state.db")
	if info, sErr := os.Stat(dbPath); sErr == nil && !info.IsDir() {
		st, err = storage.Open(dbPath)
		if err != nil {
			t.Fatalf("open store %s: %v", dbPath, err)
		}
		defer st.Close()
	}
	var out bytes.Buffer
	if err := bridge.HandleAntigravityHookWithEvent(st, root, event, bytes.NewReader(rawIn), &out); err != nil {
		t.Fatalf("hook %s failed: %v", event, err)
	}
	var resp map[string]any
	_ = json.Unmarshal(out.Bytes(), &resp)
	return out.String(), resp
}

// validCharterMD trả về nội dung docs/00-project-charter.md đạt chuẩn đủ 8 mục bắt buộc của Pha 0.
func validCharterMD(projectTitle string) string {
	return "# Project Charter — " + projectTitle + `

## 1. Project Overview
Deterministic inventory warehouse service with audit-ready stock movement tracking.

## 2. Problem Statement
Manual stock spreadsheets cause negative inventory drift and lost warehouse receipts.

## 3. Business Objectives
Guarantee 100% accurate SKU balance tracking and sub-50ms stock query latency.

## 4. Target Users
Warehouse operators, inventory supervisors, and finance auditors.

## 5. Initial Scope
Inbound/outbound stock movement API, SKU balance ledger, and automated regression suite.

## 6. Constraints
Pure Go single binary without external paid cloud dependencies.

## 7. Initial Risks
Concurrent stock adjustments causing race conditions if transactions are not serialized.

## 8. Success Metrics
Zero stock balance discrepancies across 100% of verified functional requirements.
`
}

// validRequirementsMD trả về nội dung docs/01-requirements.md đạt chuẩn đủ 16 mục, REQ-F/NF/BR/CON, User Story và Given/When/Then.
func validRequirementsMD(req1Summary, req2Summary string) string {
	return `# Software Requirements Specification

## 1. Project Overview
Core domain specification governed by SEN-STD-SDLC-001 v1.0.0.

## 2. Business Objectives
Deliver deterministic domain logic with full requirement-to-test traceability.

## 3. Stakeholders
Product Owner, Warehouse Operations Lead, QA Lead, Security Architect.

## 4. User Personas
Warehouse Clerk recording SKU receipts and Supervisor auditing stock levels.

## 5. Scope
Core SKU stock addition, stock deduction validation, and health telemetry.

## 6. Out of Scope
Third-party ERP billing synchronization and legacy hardware barcode drivers.

## 7. Functional Requirements
- REQ-F-001: ` + req1Summary + `
- REQ-F-002: ` + req2Summary + `

## 8. Non-functional Requirements
- REQ-NF-001: Stock operations execute deterministically in memory under 10ms per call.

## 9. User Stories
As a warehouse clerk, I want to add and reserve SKU quantities safely, so that inventory balances never become negative.

## 10. Business Rules
- REQ-BR-001: Outbound reservation exceeding available SKU quantity must be rejected with an explicit error.

## 11. Acceptance Criteria
- Given an empty SKU ledger, When 15 units of SKU-A are received, Then available balance equals 15.
- Given 15 units of SKU-A, When 20 units are requested, Then the operation fails and balance stays 15.

## 12. Constraints
- REQ-CON-001: Standard library Go implementation with zero CGO dependencies.

## 13. Assumptions
Single-node transactional caller invokes domain functions with UTF-8 SKU identifiers.

## 14. Risks
Integer overflow or negative quantity injection if input validation is skipped.

## 15. Open Questions
None — all critical functional boundaries are confirmed for v1.0.0.

## 16. Requirement Traceability
Mapped in docs/traceability-matrix.md across REQ-F-001, REQ-F-002, and REQ-NF-001.
`
}

