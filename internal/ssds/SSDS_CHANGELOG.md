# SEN SOFTWARE DEVELOPMENT STANDARD — CHANGELOG (`SEN-STD-SDLC-001`)

Tài liệu này theo dõi toàn bộ lịch sử phiên bản của **SEN SOFTWARE DEVELOPMENT STANDARD (`SEN-STD-SDLC-001`)**, được quản trị độc lập với vòng đời phát hành của binary `Sen-VN`.

## Quy tắc phân loại nâng cấp chuẩn SSDS
- **`PATCH` (`1.0.x`)**: Tinh chỉnh diễn đạt, sửa lỗi parser Markdown mà không làm thay đổi cấu trúc pha, deliverables hay điều kiện cổng chất lượng.
- **`MINOR` (`1.x.0`)**: Bổ sung hướng dẫn chuyên ngành hoặc kiểm tra khuyến nghị tương thích ngược (dự án cũ không bị đứt gãy).
- **`MAJOR` (`x.0.0`)**: Thay đổi chuỗi pha, thêm mục bắt buộc mới trong tài liệu hoặc siết chặt cổng phê duyệt (yêu cầu chạy `sen sdlc version --check` và phê duyệt `CRITICAL` trước khi nâng cấp).

---

## `[1.0.0]` — 2026-11-11 (`INITIAL BASELINE`)
- **Phạm vi:** 7 Phần, 28 Điều khoản chính thức của `SEN-STD-SDLC-001 v1.0.0`.
- **Tự động kích hoạt (§1–§3):** Bao phủ 15 loại hình yêu cầu phần mềm, phân loại 7 nhóm dự án (`GREENFIELD..RESEARCH`), chấm điểm 8 chiều rủi ro và tuyển chọn 3 khung `WF_MICRO | WF_STANDARD | WF_CRITICAL`.
- **Phân tách vai trò & Phản biện độc lập (§4, §18):** 27 vai trò chuyên môn thuộc 6 khối; tách bạch trung thực `SELF_REVIEW` với `INDEPENDENT_AI_REVIEW`, `AUTOMATED_VALIDATION` và `HUMAN_APPROVAL`.
- **Máy trạng thái 10 Pha SDLC (§5–§14, §22):** `P0` Charter (8 mục) → `P1` Requirements (16 mục, `REQ-F/NF/BR/CON`, User Story, `Given/When/Then`) → `P2` Architecture & `ADR-*` → `P3` Planning (10 thuộc tính task) → `P4` Foundation → `P5` Implementation → `P6` QA & Security (chống báo láo `PASS`) → `P7` Release → `P8` Handover → `P9` Maintenance.
- **Kiểm soát chất lượng & Quản trị (§16–§27):** Truy vết 6 mắt xích (`docs/traceability-matrix.md`), khóa băm Change Impact Analysis (`CIA-*`), 4 trạng thái thực thi (`IMPLEMENTED..VERIFIED`), 3 cấp thẩm quyền (`MINOR/MAJOR/CRITICAL`), hồ sơ ngoại lệ có thời hạn (`EXC-*`) và báo cáo tiến độ chuẩn 8 phần (§25).

