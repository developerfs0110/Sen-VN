# Sen MVP v1.0 (Tiếng Việt)

**Sổ cái xác định & Trọng tài kiểm chứng cho AI Coding (`sen`)** — không cần API key trả phí, không CGO, không bắt buộc Docker, đóng gói sẵn dạng **Plugin Antigravity & Kho GitHub Zero-Build** (sinh viên không cần cài Go vẫn chạy ngay).

---

## Vì sao cần Sen MVP?

Các AI Agent khi lập trình thường gặp lỗi "ảo giác hoàn thành": chạy `go test` trên thư mục rỗng (`[no test files]`), tự ý sửa yếu file unit test cũ để ép pass, sửa lén file ngoài phạm vi, bỏ sót yêu cầu khi prompt dài, hoặc làm lộ `API_KEY` trong log.

**Sen MVP** đóng vai trò trọng tài độc lập tại máy cục bộ dựa trên SQLite WAL gồm đúng 5 bảng (`.sen/state.db`):
1. **Hậu kiểm phổ quát (Mọi IDE) + Tiền kiểm tự động (Antigravity):** Hoạt động trên mọi IDE qua file `AGENTS.md` gọn ~31 dòng, và tự động nâng cấp lên chặn trước thao tác thông qua `hooks.json` (`SessionStart`, `PreInvocation`, `PreToolUse`, `Stop`) trên Antigravity.
2. **Chỉ `sen` mới tạo được bằng chứng (`evidence`):** Agent không có bất kỳ cờ CLI hay đường tắt nào để tự đánh dấu `"PASS"`. `sen verify` và `sen done` gọi trực tiếp binary (`exec.CommandContext`, tuyệt đối không qua `sh -c` hay `cmd /C`), lọc biến môi trường nhạy cảm, từ chối bộ test rỗng (`0` test), che token rò rỉ (`[REDACTED]`) và nối chuỗi băm SHA-256 chống sửa trộm.
3. **Khung xương cứng + Kỹ năng đồng hành linh hoạt (`0` Token nhàn rỗi):** Tự động phát hiện các gói kỹ năng trong `.agents/` (`superpowers`, `security-audit`, `vn-law-check`), neo yêu cầu `R_sec` / `R_law` vào SQLite khi kích hoạt và tự thích ứng mượt mà khi máy chưa cài.

---

## Cài đặt Zero-Build 1 Lệnh (Không cần cài Go!)

### Cách 1 — Cài làm Plugin Antigravity (Zero-Build)
Clone trực tiếp vào thư mục `.agents/plugins/` của workspace:
```bash
git clone https://github.com/developerfs0110/Sen-VN.git .agents/plugins/sen-vn
```
Antigravity tự nhận diện `plugin.json`, `hooks.json`, `rules/AGENTS.md` và `skills/sen-setup/SKILL.md`. Lần chạy đầu tiên, wrapper `./bin/sen` (`bin/sen.cmd` trên Windows, `bin/sen` trên macOS/Linux) tự tải bản thực thi từ GitHub Releases, kiểm chứng mã băm `SHA-256` từ `checksums.txt` (hoặc tự `go build` một lần nếu đang offline và máy có sẵn Go), lưu vào `$PLUGIN_DATA/bin` rồi chạy tức thì `<2ms` cho mọi lần gọi tiếp theo.

### Cách 2 — Cài CLI vào `PATH` của User (Cho sinh viên trên Windows / macOS / Linux)
**Windows PowerShell (Không cần quyền Admin / SmartScreen-safe):**
```powershell
irm https://raw.githubusercontent.com/developerfs0110/Sen-VN/main/scripts/install-sen.ps1 | iex
```
*(Hoặc từ thư mục đã clone: `powershell -ExecutionPolicy Bypass -File .\scripts\install-sen.ps1`)* — Tự tải `checksums.txt` + `sen-windows-amd64.exe` qua HTTPS, đối chiếu `SHA-256`, chạy `Unblock-File` và thêm `%LOCALAPPDATA%\Programs\sen\bin` vào `PATH` của User.

**macOS / Linux:**
```bash
curl -fsSL https://raw.githubusercontent.com/developerfs0110/Sen-VN/main/scripts/install-sen.sh | sh
```

### Cách 3 — Biên dịch từ mã nguồn (Dành cho Go Developer, Go 1.26+)
```bash
go install github.com/developerfs0110/Sen-VN/cmd/sen@latest
# hoặc tại thư mục gốc dự án:
go build -o sen.exe ./cmd/sen
```

---

## Bề mặt 8 Lệnh CLI Cố định (Scope Freeze)

| # | Lệnh | Vai trò |
|---|---|---|
| 1 | `sen init [--with-companions] [--agy-hooks]` | Khởi tạo `.sen/state.db` (5 bảng WAL), `.sen/config.json` (+ `.sen/config.sha256`), `AGENTS.md` (~31 dòng), `.agents/hooks.json` và chụp mốc nền `__BASELINE__`. |
| 2 | `sen brief [--path <file>] [--hook [event]]` | In tóm tắt ngữ cảnh `<250 token` (luật dự án, `R_i` còn thiếu, kỹ năng đồng hành, cảnh báo task `OPEN` bị gián đoạn) hoặc xử lý JSON hook của Antigravity. |
| 3 | `sen intake` | Ghi nhận nguyên văn yêu cầu (`--quote "<raw>" --req "<summary>" [--bugfix] [--security] [--vn-law]`) hoặc quản lý luật dự án (`--rule "<text>" [--glob "*"] [--replace <id>] [--delete <id>]`, tối đa `<= 20` luật). |
| 4 | `sen begin <task_id>` | Mở task bất biến (`--reqs R1,R2 --files f1,f2 [--kind feature\|bugfix\|ui] [--allow-delete]`), tính `risk_auto` xác định, khóa SHA-256 toàn bộ file test có sẵn (`is_locked_test = 1`) và sao lưu file `<= 10MB`. |
| 5 | `sen verify <task_id> -- <bin> [args...]` | Kiểm tra sửa lén ngoài phạm vi so với `__BASELINE__` & file test bị khóa, chạy trực tiếp `<bin>` với môi trường sạch, chặn chạy 0 test, nâng mốc `__BASELINE__` khi pass, ẩn secret và ghi chuỗi băm `evidence`. |
| 6 | `sen undo [task_id]` | Khôi phục chính xác từng byte cho task đang `OPEN` từ `.sen/snapshots/<task_id>/`, dọn file mới tạo và đánh dấu `UNDONE`. |
| 7 | `sen done [-- <suite_bin> <args...>]` | Cổng nghiệm thu cuối: kiểm chứng chuỗi băm SHA-256 từ `GENESIS`, kiểm tra toàn vẹn workspace, chạy lại bộ test dự án và đối soát SQL đảm bảo **100% `R_i` đều có task `VERIFIED` đạt `exit_code = 0`**. Xuất `.sen/delivery-report.md`. |
| 8 | `sen status [--approve]` | Hiển thị bảng tiến độ ASCII (`Rules N/20`, `Companions`, `Requirements`, `Tasks`, `Evidence`) và duyệt nhanh yêu cầu với `--approve`. |

---

## Ranh giới vận hành trung thực

- **Giới hạn tiến trình so với Container:** Danh sách trắng binary + kiểm tra đường dẫn `PATH` an toàn + `exec.CommandContext` trực tiếp + lọc biến môi trường giúp chặn chèn lệnh shell, giả mạo shim trong `%TEMP%` và lộ khóa API của máy mẹ mà không ép sinh viên phải cài Docker. Để cô lập mã độc hoàn toàn ở mức hệ điều hành vẫn cần VM/Container.
- **Truy vết xác định & Đối chiếu ngữ nghĩa:** SQL đảm bảo 100% `R_i` gắn với lệnh kiểm thử thực thi thành công; bảng nghiệm thu của `sen done` đặt `R_i <-> Câu gốc của người dùng <-> Lệnh kiểm chứng` cạnh nhau để con người rà soát trong vài giây.
