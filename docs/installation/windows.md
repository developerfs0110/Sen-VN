# Hướng dẫn cài đặt Sen trên Windows (PowerShell)

Tài liệu hướng dẫn chi tiết cách cài đặt, cấu hình và nâng cấp `sen.exe` trên hệ điều hành Windows mà **không cần quyền Quản trị viên (No Administrator / UAC)**.

---

## 1. Cài đặt nhanh 1 lệnh (Khuyến nghị)

Mở **PowerShell** (hoặc Windows Terminal) và dán lệnh sau:

```powershell
irm https://raw.githubusercontent.com/developerfs0110/Sen-VN/main/scripts/install-sen.ps1 | iex
```

### Quá trình script tự động thực hiện:
1. Tự động nhận diện kiến trúc CPU (`amd64` hoặc `arm64`).
2. Tải bản phân phối nhị phân chính thức `sen-windows-<arch>.exe` từ GitHub Releases qua kênh mã hóa HTTPS an toàn.
3. Kiểm tra tính toàn vẹn chữ ký **SHA-256** đối chứng với `checksums.txt` (tuyệt đối từ chối nếu sai lệch băm).
4. Lưu tệp thực thi vào thư mục người dùng: `%LOCALAPPDATA%\Programs\sen\bin\sen.exe`.
5. Tự động bổ sung đường dẫn vào **User PATH** (không ảnh hưởng đến System PATH hay các tài khoản khác).
6. Tự động hợp nhất Antigravity Lifecycle Hooks (`sen-vn`) vào `~/.gemini/config/hooks.json` (giữ nguyên các hook khác của bạn).

---

## 2. Kiểm tra cài đặt thành công

Đóng và mở lại một cửa sổ PowerShell mới (hoặc gõ `$env:Path = [System.Environment]::GetEnvironmentVariable("Path","User") + ";" + [System.Environment]::GetEnvironmentVariable("Path","Machine")`), sau đó kiểm tra:

```powershell
sen sdlc version
```

Kết quả hiển thị tương tự:
```text
Sen-VN: v1.2.0 | Pinned SSDS Version: uninitialized | Bundled/Available: [1.0.0]
```

---

## 3. Các tùy chọn cài đặt nâng cao

Nếu bạn tải mã nguồn repo về máy và muốn chạy cục bộ:

### a. Bỏ qua cấu hình Antigravity Hooks
Nếu bạn chỉ muốn dùng `sen` CLI độc lập mà không dùng Antigravity IDE:
```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\install-sen.ps1 -ConfigureHooks:$false
```

### b. Khởi tạo ngay Không gian làm việc kèm Gói kỹ năng
Khởi tạo ngay dự án hiện tại với các gói kỹ năng mở rộng (ví dụ dựng video, vẽ biểu đồ):
```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\install-sen.ps1 -Workspace . -Packs media-video,workflow-diagram
```

### c. Cập nhật lên bản mới nhất
```powershell
powershell -ExecutionPolicy Bypass -File .\scripts\install-sen.ps1 -Update
```

---

## 4. Biên dịch từ mã nguồn (Dành cho nhà phát triển)

Yêu cầu máy có cài **Go 1.26+**:
```powershell
git clone https://github.com/developerfs0110/Sen-VN.git
cd Sen-VN
go build -trimpath -ldflags="-s -w" -o sen.exe ./cmd/sen
```

