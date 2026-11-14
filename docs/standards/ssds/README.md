# 📚 Kho Lưu Trữ Quy Chuẩn SSDS (SEN SOFTWARE DEVELOPMENT STANDARD)

Thư mục lưu trữ văn bản pháp quy chính thức của tiêu chuẩn **SEN-STD-SDLC-001** do Chủ nhiệm Kiến trúc **Mr Bòn Bon (anh Gấu)** ban hành và cập nhật qua các phiên bản.

---

## 📑 Danh Mục Toàn Bộ Các Phiên Bản

| Phiên bản | Tệp Văn Bản Chính Thức | Ngày Cập Nhật | Chữ Ký Toàn Vẹn SHA-256 | Trạng Thái Trong Sen Core |
|:---:|:---|:---:|:---:|:---:|
| **v1.0.0** | [`SEN_SOFTWARE_DEVELOPMENT_STANDARD_v1.0.0.docx`](SEN_SOFTWARE_DEVELOPMENT_STANDARD_v1.0.0.docx) | 2026-11-11 | `B728AE78A5DC3B8D39EBD833B0DA05FB044F40D605206BB38652EF929291EA03` | **Đã Tích Hợp (Active)** |

---

## 🔄 Cơ Chế Tự Động Quét & Đồng Bộ (Auto-Discovery & Sync Engine)

1. **Cách Cập Nhật:**
   Mỗi khi anh Gấu ban hành phiên bản SSDS mới:
   - Lưu tệp Word vào thư mục này theo quy ước:
     `SEN_SOFTWARE_DEVELOPMENT_STANDARD_vX.Y.Z.docx` (hoặc `SEN SOFTWARE DEVELOPMENT STANDARD - SSDS_vX.Y.docx`).
2. **Cơ Chế Sen Tự Động Quét:**
   - Khi khởi động phiên hoặc chạy lệnh `sen sdlc sync`, Sen sẽ tự động bóc tách cấu trúc XML tệp `.docx`, phát hiện version mới nhất.
   - Nếu version trên đĩa mới hơn version hiện tại của Sen Core, Sen sẽ tự động cảnh báo và cung cấp phương án mapping/nâng cấp.
