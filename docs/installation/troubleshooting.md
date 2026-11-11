# Sổ tay Xử lý Sự cố Cài đặt & Vận hành Sen (Troubleshooting)

Tổng hợp các tình huống thường gặp và cách xử lý nhanh chóng, triệt để nhất.

---

## 1. Lỗi PowerShell: "running scripts is disabled on this system"

### Nguyên nhân:
Chính sách bảo mật `ExecutionPolicy` mặc định của Windows PowerShell chặn thực thi script chưa được cấp phép.

### Cách xử lý:
Chạy lệnh sau trong PowerShell để cho phép thực thi trong phiên làm việc hiện tại:
```powershell
Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass -Force
```
Sau đó chạy lại lệnh cài đặt:
```powershell
irm https://raw.githubusercontent.com/developerfs0110/Sen-VN/main/scripts/install-sen.ps1 | iex
```

---

## 2. Lỗi: Lệnh `sen` không được nhận diện sau khi cài

### Nguyên nhân:
Cửa sổ terminal hiện tại chưa kịp tải lại biến môi trường `PATH` mới cập nhật của người dùng.

### Cách xử lý:
1. **Cách nhanh nhất**: Đóng toàn bộ cửa sổ Terminal / PowerShell / IDE và mở lại.
2. **Cập nhật ngay trong phiên PowerShell hiện tại**:
   ```powershell
   $env:Path = [System.Environment]::GetEnvironmentVariable("Path","User") + ";" + [System.Environment]::GetEnvironmentVariable("Path","Machine")
   sen sdlc version
   ```

---

## 3. Lỗi Hook Antigravity: `is not recognized as an internal or external command`

### Nguyên nhân:
Trước bản `v1.2.0`, đường dẫn lệnh trong `hooks.json` bị bọc trong dấu ngoặc kép thừa khiến trình thực thi lệnh Windows `cmd.exe` escape thành `\"C:\...\"` và báo lỗi không tìm thấy tệp.

### Cách xử lý:
Chạy lại script cài đặt mới nhất của Sen (đã sửa triệt để cơ chế xử lý đường dẫn không có dấu quote thừa):
```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\install-sen.ps1
```
Hoặc mở tệp `%USERPROFILE%\.gemini\config\hooks.json` và kiểm tra giá trị `command` phải có dạng sạch:
```json
"command": "C:\\Users\\<User>\\AppData\\Local\\Programs\\sen\\bin\\sen.exe brief --hook PreToolUse"
```

---

## 4. Cách gỡ bỏ hoặc tạm dừng Sen an toàn

Nếu bạn muốn gỡ cài đặt hoặc tạm dừng Sen:

### a. Gỡ bỏ Lifecycle Hooks:
Mở `%USERPROFILE%\.gemini\config\hooks.json` (và `.agents/hooks.json` trong workspace nếu có), xóa khối `"sen-vn": { ... }` rồi lưu lại.

### b. Xóa tệp thực thi:
- **Windows**: Xóa thư mục `%LOCALAPPDATA%\Programs\sen`.
- **Linux/macOS**: Xóa tệp `~/.local/bin/sen`.

