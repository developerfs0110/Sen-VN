# Sen-VN v1.1.0 (Tiếng Việt)

**Sổ cái xác định & Trọng tài kiểm chứng đa chuyên ngành cho AI (`sen`)** — không cần API key trả phí, không CGO, không bắt buộc Docker, đóng gói sẵn dạng **Plugin Antigravity & Kho GitHub Zero-Build** (sinh viên không cần cài Go vẫn chạy ngay).

---

## Điểm mới nổi bật trong Sen-VN v1.1.0 (Giữ nguyên bất biến 8 lệnh CLI & 5 bảng SQLite)

1. **Định cỡ rủi ro 3 tầng (`SMALL` / `MEDIUM` / `LARGE`) & Chế độ tự hành `--express` (*"Làm luôn đi"*):**
   - Tự động phân tầng: `SMALL` (`<=2` file), `MEDIUM` (`3-5` file), `LARGE` (`>5` file, `bugfix`, hạ tầng trọng yếu hoặc `--allow-delete`).
   - Khi người dùng nói *"làm luôn đi"* / *"chạy thẳng"* / *"just do it"* (hoặc bật `--express`), Sen tự động cho phép chạy thẳng các tác vụ không phá hủy nhưng **vẫn bắt buộc hỏi xác nhận (`force_ask`)** nếu có xóa file (`--allow-delete`) hoặc chạm tới `.env`, `auth/`, `payment/`, DB migration. Tệp `.sen/config.json` được ký `.sen/config.sha256` chống Agent tự ý bật lén.
2. **Chốt chặn vòng lặp sửa lỗi `<= 3` vòng (`[QUARANTINE 3/3]`) & Thẩm định biện chứng (`sen-dialectical`):**
   - Khóa cứng lần `sen verify` thứ 4 khi thất bại 3 vòng liên tiếp, cho phép `Stop` hook thoát an toàn (không bị lặp vô hạn) để báo cáo nguyên nhân gốc rễ khách quan kèm cột **Tài nguyên tiêu tốn** trước khi người dùng mở khóa bằng `sen status --approve` hoặc hoàn tác `sen undo`.
3. **Đa chuyên ngành (`--profile auto|code|web|video|media|docs`) & Bảo vệ Video Production (`ffprobe` + Khóa Raw Media):**
   - Tự động nhận diện dự án lập trình, Web/UI, dựng phim (`video`), đồ họa/âm thanh (`media`) và tài liệu (`docs` — không ép chạy `go test`/`npm test` giả tạo).
   - Khóa bất biến toàn bộ thư mục `raw/`, `master/`, `footage/`, `originals/` (theo dõi `size + mtime_ns` cho file `>10MB` không tốn ổ cứng sao chép), chặn lệnh `ffmpeg` ghi đè file gốc ở `PreToolUse`, và tích hợp bộ phân tích `ffprobe` kiểm tra số luồng video/audio, codec, độ phân giải, thời lượng (`strength=MEDIA`).
4. **Cài đặt tự động gói kỹ năng mặc định + Fork dự phòng `developerfs0110` + Menu Gói mở rộng (`--packs`) + Cập nhật an toàn (`--update`):**
   - `sen init --install-companions`: tự cài `superpowers`, `security-audit`, `vn-law-check`, `sen-dialectical`, `sen-skill-capture` (nếu có sẵn sẽ báo `[exists]` và bỏ qua; nếu link gốc lỗi tự động chuyển sang fork `https://github.com/developerfs0110/...` đúng mã commit cố định).
   - `sen init --packs list` / `sen init --packs workflow-diagram,media-video,media-image,voice-tts,media-3d`: cài thêm gói kỹ năng theo nhu cầu (`archify`, `ffmpeg-skill`, `video-subtitles-effects`, `store-screenshots`, `voicestudio`, `blender-director`).
   - `sen init --update`: kiểm tra phiên bản mới trên GitHub Releases và nâng cấp nguyên tử các gói trong `.agents/sen.lock.json` kèm sao lưu/rollback `.bak`.

---

## Cài đặt Zero-Build 1 Lệnh (Không cần cài Go!)

### Cách 1 — Cài làm Plugin Antigravity (Zero-Build)
```bash
git clone https://github.com/developerfs0110/Sen-VN.git .agents/plugins/sen-vn
```
Antigravity tự nhận diện `plugin.json` (`v1.1.0`), `hooks.json`, `rules/AGENTS.md` và bộ kỹ năng `sen-setup`, `sen-dialectical`, `sen-skill-capture`.

### Cách 2 — Cài CLI vào `PATH` kèm Bộ Kỹ Năng Đồng Hành (Windows / macOS / Linux)
**Windows PowerShell (Không cần quyền Admin / SmartScreen-safe):**
```powershell
irm https://raw.githubusercontent.com/developerfs0110/Sen-VN/main/scripts/install-sen.ps1 | iex
# Khởi tạo dự án kèm gói mặc định + gói mở rộng hoặc cập nhật an toàn:
powershell -ExecutionPolicy Bypass -File .\scripts\install-sen.ps1 -Workspace . -Packs media-video,workflow-diagram
powershell -ExecutionPolicy Bypass -File .\scripts\install-sen.ps1 -Workspace . -Update
```

**macOS / Linux:**
```bash
curl -fsSL https://raw.githubusercontent.com/developerfs0110/Sen-VN/main/scripts/install-sen.sh | sh
./scripts/install-sen.sh --workspace . --packs media-video,workflow-diagram
./scripts/install-sen.sh --workspace . --update
```

---

## Bề mặt 8 Lệnh CLI Cố định (Scope Freeze)

| # | Lệnh | Vai trò |
|---|---|---|
| 1 | `sen init [--install-companions] [--packs <list\|all\|csv>] [--profile <...>] [--express] [--update]` | Khởi tạo `.sen/state.db` (5 bảng WAL), `.sen/config.json` (+ `.sen/config.sha256`), `AGENTS.md`, `.agents/hooks.json`, tự cài gói kỹ năng đồng hành / gói mở rộng hoặc chạy cập nhật an toàn (`--update`). |
| 2 | `sen brief [--path <file>] [--hook [event]]` | In tóm tắt `<250 token` (`profile`, `express`, kỹ năng đồng hành, cảnh báo `QUARANTINE` / task `OPEN` dở dang, `R_i` chưa đạt) hoặc xử lý JSON hook Antigravity. |
| 3 | `sen intake` | Ghi nhận nguyên văn yêu cầu (`--quote "<raw>" --req "<summary>" [--bugfix] [--security] [--vn-law] [--express]`) hoặc quản lý luật dự án (`<= 20` luật). |
| 4 | `sen begin <task_id>` | Mở task bất biến (`--reqs R1,R2 --files f1,f2 [--kind feature\|bugfix\|ui] [--allow-delete] [--express]`), tự động định cỡ `SMALL`/`MEDIUM`/`LARGE`, khóa SHA-256 file test và khóa bất biến file raw media. |
| 5 | `sen verify <task_id> -- <bin> [args...]` | Giới hạn `<= 3` lần lỗi liên tiếp (`[QUARANTINE 3/3]`), kiểm tra toàn vẹn phạm vi & tài nguyên khóa, chạy trực tiếp `<bin>` (`go`, `pytest`, `vitest`, `ffprobe`...), chặn 0 test / media hỏng 0 stream, nâng mốc `__BASELINE__` và nối chuỗi băm `evidence`. |
| 6 | `sen undo [task_id]` | Khôi phục chính xác từng byte cho task `OPEN`, dọn file mới tạo, xóa cờ cách ly và đánh dấu `UNDONE`. |
| 7 | `sen done [-- <suite_bin> <args...>]` | Kiểm chứng chuỗi băm từ `GENESIS`, toàn vẹn workspace/raw-media, bộ kiểm thử theo chuyên ngành và đối soát SQL đảm bảo **100% `R_i` đã `VERIFIED`**. Xuất `.sen/delivery-report.md` nâng cao. |
| 8 | `sen status [--approve] [--express]` | Hiển thị chuyên ngành, chế độ tự hành, luật, yêu cầu, task (`[QUARANTINE 3/3]`) và chuỗi bằng chứng; `--approve` phê duyệt yêu cầu đồng thời mở khóa vòng lặp sửa lỗi cho task đang kẹt. |
