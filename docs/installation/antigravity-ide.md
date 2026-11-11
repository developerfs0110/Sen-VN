# Tích hợp Sen với Google Antigravity IDE

Sen được thiết kế dưới dạng **Native Antigravity Plugin**, kết nối trực tiếp với 4 sự kiện vòng đời chính của Antigravity để kiểm soát và bảo vệ chất lượng tự động.

---

## 1. Cài đặt dạng Plugin Native (Zero-Build)

Nếu bạn đang mở một Workspace trong Antigravity IDE, chỉ cần chạy lệnh clone vào thư mục `.agents/plugins/`:

```bash
git clone https://github.com/developerfs0110/Sen-VN.git .agents/plugins/sen-vn
```

Sau khi clone, Antigravity sẽ tự động nhận diện:
- `plugin.json`: Định danh plugin `sen-vn` phiên bản `v1.2.0`.
- `rules/AGENTS.md`: Tự động áp dụng quy tắc vận hành Sen Core Rule.
- `skills/`: Cung cấp các kỹ năng tích hợp sẵn (`sen-setup`, `sen-dialectical`, `sen-skill-capture`).

---

## 2. Cơ chế 4 Vòng đời Hook (`hooks.json`)

Tệp `hooks.json` cấu hình 4 móc vòng đời mà Sen tham gia kiểm soát:

```json
{
  "sen-vn": {
    "SessionStart": [
      { "type": "command", "command": "sen brief --hook SessionStart" }
    ],
    "PreInvocation": [
      { "type": "command", "command": "sen brief --hook PreInvocation" }
    ],
    "PreToolUse": [
      {
        "matcher": "*",
        "hooks": [
          { "type": "command", "command": "sen brief --hook PreToolUse" }
        ]
      }
    ],
    "Stop": [
      { "type": "command", "command": "sen brief --hook Stop" }
    ]
  }
}
```

### Ý nghĩa vận hành của từng Hook:
1. **`SessionStart`**: Bơm bản tóm tắt ngắn gọn (`<250 tokens`) về trạng thái dự án, pha SSDS hiện tại và nhiệm vụ tiếp theo vào đầu phiên, giúp Agent không bị "ngáo" hay quên bối cảnh.
2. **`PreInvocation`**: Tự động nhận diện bài toán phát triển phần mềm ngay từ câu nhắc đầu tiên của người dùng, tự quét dự án và kích hoạt quy chuẩn SSDS 10 pha mà không cần người dùng nhắc nhở. Bảo vệ thư mục mẹ đa dự án không bị ô nhiễm.
3. **`PreToolUse` (Tường lửa tiền thi hành)**: Chặn vật lý (`decision: deny`) các hành vi sửa code mã nguồn bừa bãi khi chưa qua duyệt tài liệu yêu cầu (P0..P3); ép buộc người dùng xác nhận (`decision: force_ask`) khi Agent cố tình tự chạy các lệnh phê duyệt nhạy cảm.
4. **`Stop`**: Giám sát khi Agent muốn kết thúc lượt làm việc. Nếu task vẫn đang `OPEN` hoặc chưa có bằng chứng kiểm thử đạt chuẩn, hook sẽ yêu cầu Agent tiếp tục thực thi đến khi hoàn tất.

