# Hướng dẫn cài đặt Sen trên Linux & macOS

Tài liệu hướng dẫn chi tiết cách cài đặt `sen` trên hệ điều hành Linux và macOS mà **không cần quyền root (`sudo`)**.

---

## 1. Cài đặt nhanh 1 lệnh (Khuyến nghị)

Mở terminal (bash/zsh) và chạy lệnh sau:

```bash
curl -fsSL https://raw.githubusercontent.com/developerfs0110/Sen-VN/main/scripts/install-sen.sh | sh
```

### Quá trình script tự động thực hiện:
1. Nhận diện hệ điều hành (`Linux` / `Darwin`) và kiến trúc CPU (`x86_64` / `arm64` - tương thích hoàn hảo Apple Silicon M1/M2/M3/M4).
2. Tải bản phát hành từ GitHub Releases qua HTTPS.
3. Đối chứng mã băm **SHA-256** với `checksums.txt`.
4. Đặt tệp nhị phân vào `$HOME/.local/bin/sen` và cấp quyền thực thi (`chmod +x`).
5. Hướng dẫn cập nhật `PATH` vào `~/.bashrc` hoặc `~/.zshrc` nếu chưa có.
6. Tự động hợp nhất các vòng đời hook vào `~/.gemini/config/hooks.json` nếu có cài đặt Antigravity.

---

## 2. Thiết lập PATH (Nếu cần)

Nếu lệnh `sen` chưa nhận diện ngay sau khi cài, hãy thêm dòng sau vào tệp cấu hình shell của bạn (`~/.bashrc` hoặc `~/.zshrc`):

```bash
export PATH="$HOME/.local/bin:$PATH"
```

Sau đó áp dụng ngay:
```bash
source ~/.bashrc  # hoặc source ~/.zshrc
sen sdlc version
```

---

## 3. Các tùy chọn cài đặt nâng cao

```bash
# Bỏ qua đăng ký Antigravity hooks
./scripts/install-sen.sh --no-hooks

# Cài đặt và khởi tạo ngay thư mục làm việc kèm gói mở rộng
./scripts/install-sen.sh --workspace /path/to/project --packs media-video,workflow-diagram

# Cập nhật lên bản mới nhất
./scripts/install-sen.sh --update
```

---

## 4. Biên dịch từ mã nguồn

Yêu cầu **Go 1.26+**:
```bash
git clone https://github.com/developerfs0110/Sen-VN.git
cd Sen-VN
go build -trimpath -ldflags="-s -w" -o sen ./cmd/sen
chmod +x sen
mv sen ~/.local/bin/
```

