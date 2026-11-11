# Sử dụng Sen với VSCode, Cursor, Windsurf & Claude Code

Ngoài Google Antigravity IDE, Sen cung cấp các **Smart Zero-Build Wrappers** giúp bạn sử dụng Sen mượt mà trong bất kỳ IDE hoặc công cụ AI coding nào khác.

---

## 1. Smart Zero-Build Wrappers là gì?

Trong thư mục `bin/` của kho mã nguồn Sen có 2 tệp script thông minh:
- **`bin/sen.cmd`**: Dành cho môi trường Windows (cmd.exe / PowerShell).
- **`bin/sen`**: Dành cho môi trường Linux và macOS (POSIX shell).

### Cơ chế 3 tầng tự phục hồi:
1. **Tầng 1 (Siêu tốc <2ms)**: Nếu máy đã cài `sen.exe` hoặc có bản cache sẵn, wrapper chạy ngay lập tức.
2. **Tầng 2 (Tải tự động HTTPS)**: Nếu máy chưa có binary, wrapper tự động tải bản phát hành chính thức từ GitHub Releases, kiểm tra mã băm SHA-256 và lưu vào cache cục bộ.
3. **Tầng 3 (Biên dịch dự phòng)**: Nếu không có kết nối Internet nhưng có Go trên máy, wrapper tự động chạy `go build` để tạo binary.

---

## 2. Tích hợp vào VSCode & Cursor

### a. Mở Terminal tích hợp
Bạn có thể gọi trực tiếp `sen` từ Terminal tích hợp của VSCode hoặc Cursor:
```bash
./bin/sen status
./bin/sen sdlc status --report
```

### b. Tạo Task tự động trong `.vscode/tasks.json`
Thêm cấu hình sau vào thư mục `.vscode/tasks.json` của dự án để gọi Sen bằng 1 phím tắt (`Ctrl+Shift+B`):

```json
{
  "version": "2.0.0",
  "tasks": [
    {
      "label": "Sen: Kiểm tra trạng thái SDLC",
      "type": "shell",
      "command": "sen sdlc status --report",
      "problemMatcher": [],
      "group": "build"
    },
    {
      "label": "Sen: Xác minh kiểm thử toàn diện",
      "type": "shell",
      "command": "sen verify",
      "problemMatcher": []
    }
  ]
}
```

---

## 3. Sử dụng với Claude Code / Codex / Gemini CLI

Nếu bạn sử dụng các trợ lý dòng lệnh như Claude Code hoặc Gemini CLI:
1. Cài đặt `sen` toàn cục theo hướng dẫn Windows hoặc Linux/macOS.
2. Trong tệp chỉ dẫn của dự án (như `CLAUDE.md` hoặc `AGENTS.md`), thêm chỉ dẫn:
   > "Mọi thay đổi mã nguồn phải được mở task qua `sen begin <id>` và kiểm chứng qua `sen verify <id> -- <lệnh test>` trước khi hoàn thành."
3. Sen sẽ đóng vai trò là "người gác cổng" độc lập bên dưới, đảm bảo AI không bao giờ báo cáo sai lệch hay làm hỏng mã nguồn.

