# 🚀 Hướng Dẫn Cài Đặt Sen-VN (Setup Hub)

<div align="center">

| 🇻🇳 [**README (Tiếng Việt)**](../../README.md) | 🚀 [**Cài Đặt (Setup)**](README.md) | 🇬🇧 [**English Documentation**](../../README.en.md) | 📋 [**Release Notes**](../releases/v1.2.0.md) |
|:---:|:---:|:---:|:---:|

</div>

---

## 1. Cài đặt nhanh 1 lệnh duy nhất (Khuyến nghị)

### Windows PowerShell (Không cần quyền Admin / UAC):
```powershell
irm https://raw.githubusercontent.com/developerfs0110/Sen-VN/main/scripts/install-sen.ps1 | iex
```

### macOS & Linux (POSIX Shell, Không cần root):
```bash
curl -fsSL https://raw.githubusercontent.com/developerfs0110/Sen-VN/main/scripts/install-sen.sh | sh
```

### Plugin Google Antigravity IDE Bản địa:
```bash
git clone https://github.com/developerfs0110/Sen-VN.git .agents/plugins/sen-vn
```

---

## 2. Hướng Dẫn Chi Tiết Theo Từng Môi Trường

Chọn tài liệu tương ứng với môi trường làm việc của bạn để xem đầy đủ các tùy chọn nâng cao và cách cấu hình tối ưu:

| Môi trường | Mô tả tài liệu chi tiết | Liên kết |
|---|---|:---:|
| **Windows** | Cài đặt PowerShell, quản lý User PATH, kiểm tra băm SHA-256, chạy không cần quyền Quản trị viên. | [**Xem chi tiết**](windows.md) |
| **Linux & macOS** | Cài đặt POSIX bash/zsh, hỗ trợ Apple Silicon `arm64` (M1/M2/M3/M4) và Linux `x86_64`, cấu hình shell PATH. | [**Xem chi tiết**](linux-macos.md) |
| **Antigravity IDE** | Cài đặt plugin bản địa, giải thích chi tiết cơ chế vận hành của 4 hooks: `SessionStart`, `PreInvocation`, `PreToolUse`, `Stop`. | [**Xem chi tiết**](antigravity-ide.md) |
| **VSCode, Cursor & Claude** | Hướng dẫn sử dụng Smart Zero-Build Wrappers (`bin/sen.cmd`, `bin/sen`), tạo task `.vscode/tasks.json` và tích hợp vào Cursor, Windsurf, Claude Code. | [**Xem chi tiết**](vscode-cursor.md) |
| **Xử lý sự cố (Troubleshooting)** | Sổ tay giải quyết lỗi `ExecutionPolicy`, lỗi refresh biến môi trường `PATH`, sửa lỗi escape dấu ngoặc kép, và quy trình gỡ bỏ an toàn. | [**Xem chi tiết**](troubleshooting.md) |

---

## 3. Kiểm tra cài đặt thành công

Sau khi cài đặt, mở cửa sổ dòng lệnh mới và chạy:

```bash
sen sdlc version
```

Kết quả hiển thị:
```text
Sen-VN: v1.2.0 | Pinned SSDS Version: uninitialized | Bundled/Available: [1.0.0]
```

