# Sen-VN v1.2.0 (Tiếng Việt)

**Động cơ Cưỡng chế Chuẩn Kỹ thuật Phần mềm `SEN-STD-SDLC-001 (SSDS v1.0.0)` & Trọng tài Kiểm chứng Xác định cho AI (`sen`)** — không cần API key trả phí, không CGO, không bắt buộc Docker, đóng gói sẵn dạng **Plugin Antigravity & Kho GitHub Zero-Build**.

---

## Điểm mới đột phá trong Sen-VN v1.2.0 (`SSDS v1.0.0`)

Thay vì chỉ nhúng văn bản hướng dẫn tĩnh dễ bị AI bỏ qua, **Sen-VN v1.2.0** biên dịch toàn bộ 7 Phần / 28 Mục của **SEN SOFTWARE DEVELOPMENT STANDARD (`SEN-STD-SDLC-001 v1.0.0`)** thành cơ chế cưỡng chế vật lý bằng Go + SQLite WAL **8 bảng** và **9 lệnh CLI**:

1. **Tự động nhận diện & kích hoạt 0 câu nhắc thừa (`PreInvocation` — §3.1–§3.4):**
   - Tự động phân tích câu nhắc song ngữ Việt–Anh trên 15 miền phần mềm, tự quét khảo sát kho mã sẵn có (`Brownfield Repo Audit` — §2.3), phân loại 7 nhóm dự án (`GREENFIELD`, `BROWNFIELD`, `MAINTENANCE`, `MIGRATION`, `INTEGRATION`, `PROTOTYPE`, `RESEARCH`), chọn workflow (`WF_MICRO | WF_STANDARD | WF_CRITICAL`) và hiển thị bảng tuyên bố **`[SSDS v1.0.0 — Đã kích hoạt tự động]`**. Câu hỏi tra cứu thuần túy (`0 file edits`) hoặc thư mục dựng phim/tài liệu thuần không bị kích hoạt nhầm.
2. **Máy trạng thái 10 Pha (`P0..P9`) & Tường lửa khóa ghi theo pha (`PreToolUse` — §5–§14, §22):**
   - Khóa cứng mọi thao tác ghi mã nguồn production khi dự án đang ở `P0` (Charter), `P1` (Requirements), `P2` (Architecture & ADR), `P3` (Planning & Task Breakdown).
   - Từ chối tài liệu rỗng/`TBD`/`TODO`, ép chuẩn `REQ-F/NF/BR/CON` kèm `User Story` + `Given/When/Then`, và tách bạch hoàn toàn `REVIEW_REQUIRED` khỏi `APPROVED` (**Im lặng không phải là đồng ý** — §21).
3. **27 Vai trò Kỹ thuật, Phản biện Trung thực, Truy vết 6 Mắt xích & Khóa CIA (§4.3, §16–§20, §27):**
   - Cấm `author_role == reviewer_role`; tự động hạ bậc `INDEPENDENT_AI_REVIEW` xuống `SELF_REVIEW` kèm cảnh báo minh bạch nếu không có phiên subagent độc lập (`--ref`).
   - Khóa băm SHA-256 các tài liệu `P1`/`P2` đã duyệt — bắt buộc lập hồ sơ phân tích tác động 10 chiều (`sen sdlc impact` -> `CIA-*`) trước khi sửa đổi.
   - Chặn báo cáo láo `PASS` tại `P6` nếu bảng `evidence` chưa có lần chạy test thật đạt chuẩn, đối soát chuỗi `Requirement -> Component -> Task -> File -> Evidence -> Release` trong `docs/traceability-matrix.md`, và chặn ngoại lệ `EXC-*` quá hạn.
4. **Cô lập tuyệt đối Đa dự án & Quản lý Phiên bản SSDS Độc lập (Phần II):**
   - Tự động định vị dự án con gần nhất từ đường dẫn tệp đích (`ResolveNearestProjectRoot`, cờ `--project <dir>`, và `CheckNoSubprojectCrossing`) — cam kết **0% nhiễm chéo** khi mở Workspace mẹ chứa nhiều repo con.
   - Mỗi dự án ghim riêng `ssds_version` có chữ ký `.sen/config.sha256`. Khi phát hành `SSDS v1.1` / `v2.0`, dự án đang chạy `v1.0.0` giữ nguyên 100% quy tắc đã ghim, hỗ trợ kiểm tra tương thích chỉ-đọc (`--check`), nâng cấp có phê duyệt (`--upgrade --approve --quote`) và hoàn tác tức thời (`--rollback`).

---

## Cài đặt Zero-Build 1 Lệnh & Đăng ký Hook Antigravity Toàn cục

### Cách 1 — Cài làm Plugin Antigravity (Zero-Build)
```bash
git clone https://github.com/developerfs0110/Sen-VN.git .agents/plugins/sen-vn
```

### Cách 2 — Cài CLI + Tự động hợp nhất `~/.gemini/config/hooks.json`
**Windows PowerShell (Không cần quyền Admin):**
```powershell
irm https://raw.githubusercontent.com/developerfs0110/Sen-VN/main/scripts/install-sen.ps1 | iex
# Tùy chọn bỏ qua ghi hook toàn cục hoặc khởi tạo ngay dự án kèm gói mở rộng:
powershell -ExecutionPolicy Bypass -File .\scripts\install-sen.ps1 -ConfigureHooks:$false
powershell -ExecutionPolicy Bypass -File .\scripts\install-sen.ps1 -Workspace . -Packs media-video,workflow-diagram
```

**macOS / Linux:**
```bash
curl -fsSL https://raw.githubusercontent.com/developerfs0110/Sen-VN/main/scripts/install-sen.sh | sh
./scripts/install-sen.sh --no-hooks --workspace . --packs media-video,workflow-diagram
```

---

## Bề mặt 9 Lệnh CLI Chuẩn Hóa (`sen [--project <dir>] <lệnh>`)

| # | Lệnh | Vai trò |
|---|---|---|
| 1 | `sen init [--install-companions] [--packs <...>] [--profile <...>] [--express] [--update]` | Khởi tạo `.sen/state.db` (8 bảng WAL), `.sen/config.json` có chữ ký SHA-256 (luôn bảo toàn `ssds_version` & `project_id` đã ghim), `AGENTS.md`, `.agents/hooks.json` và gói kỹ năng. |
| 2 | `sen brief [--path <file>] [--hook [event]]` | In tóm tắt `<250 token` (gồm dòng trạng thái `[SSDS v1.0.0 \| ...]` & thông báo phiên bản mới `ssds_notice`) hoặc xử lý 4 sự kiện JSON hook của Antigravity. |
| 3 | `sen intake` | Ghi nhận nguyên văn yêu cầu (`R1..Rn`, `R_sec`, `R_law`), bật/tắt `--express` hoặc quản lý `<= 20` luật dự án. |
| 4 | `sen begin <task_id>` | Mở task theo pha (`--reqs ... --files ... [--phase P5] [--components C1,C2]`), cưỡng chế tường lửa pha SSDS & ranh giới dự án con, khóa file test/raw media và sao lưu snapshot. |
| 5 | `sen verify <task_id> -- <bin> [args...]` | Giới hạn `<= 3` vòng sửa lỗi (`[QUARANTINE 3/3]`), chạy trực tiếp binary kiểm thử, cập nhật `exec_state` (`EXECUTED`/`TESTED`/`VERIFIED`) + `verdict` theo SSDS §20 và ghi chuỗi băm `evidence`. |
| 6 | `sen undo [task_id]` | Khôi phục chính xác từng byte cho task `OPEN` và xóa cờ cách ly. |
| 7 | `sen done [-- <suite_bin> <args...>]` | Kiểm chứng chuỗi băm `evidence` & `ssds_governance`, hạn ngoại lệ `EXC-*`, toàn vẹn baseline `P1/P2`, hoàn thành cổng `P1..P6` và độ phủ 100% yêu cầu; xuất `.sen/delivery-report.md`. |
| 8 | `sen status [--approve] [--express]` | Xem tổng quan dự án và mở khóa vòng lặp sửa lỗi khi có `--approve`. |
| 9 | `sen sdlc <subcommand>` | Điều phối trọn vòng đời SSDS: `classify`, `status [--report]`, `gate [P]`, `review`, `approve`, `impact`, `exception`, `sync`, và `version [--changelog \| --check \| --upgrade \| --rollback]`. |
