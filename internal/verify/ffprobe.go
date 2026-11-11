// Tệp ffprobe.go phân tích thông số luồng video/âm thanh từ đầu ra JSON hoặc key=value của ffprobe để chặn tệp media hỏng.
package verify

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// ffprobeDoc ánh xạ cấu trúc JSON chuẩn khi gọi ffprobe -print_format json -show_format -show_streams.
type ffprobeDoc struct {
	Streams []ffprobeStream `json:"streams"`
	Format  ffprobeFormat   `json:"format"`
}

// ffprobeStream mô tả thông số kỹ thuật của một luồng video, âm thanh hoặc phụ đề trong tệp đa phương tiện.
type ffprobeStream struct {
	CodecType    string `json:"codec_type"`
	CodecName    string `json:"codec_name"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	RFrameRate   string `json:"r_frame_rate"`
	AvgFrameRate string `json:"avg_frame_rate"`
	Channels     int    `json:"channels"`
	SampleRate   any    `json:"sample_rate"`
	Duration     any    `json:"duration"`
}

// ffprobeFormat chứa siêu dữ liệu mức vùng chứa (container), đặc biệt là tổng thời lượng phát (duration).
type ffprobeFormat struct {
	Duration any `json:"duration"`
}

// parseFFprobeOutput kiểm định nghiêm ngặt đầu ra ffprobe, từ chối tệp 0 luồng, 0x0 khung hình hoặc thời lượng <= 0.
func parseFFprobeOutput(stdout string, exitCode int) (string, int, error) {
	if exitCode != 0 {
		return "MEDIA", 0, fmt.Errorf("[SEN REJECTED] invalid media output: ffprobe exited with code %d", exitCode)
	}
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return "MEDIA", 0, fmt.Errorf("[SEN REJECTED] invalid media output: empty ffprobe output")
	}

	var doc ffprobeDoc
	if strings.HasPrefix(trimmed, "{") {
		if err := json.Unmarshal([]byte(trimmed), &doc); err != nil {
			return "MEDIA", 0, fmt.Errorf("[SEN REJECTED] invalid media output: malformed ffprobe JSON: %w", err)
		}
	} else {
		var ok bool
		doc, ok = parseFFprobeKeyValue(trimmed)
		if !ok {
			return "MEDIA", 0, fmt.Errorf("[SEN REJECTED] invalid media output: unrecognized ffprobe output")
		}
	}

	count, err := validateFFprobeDoc(doc)
	if err != nil {
		return "MEDIA", 0, err
	}
	return "MEDIA", count, nil
}

// validateFFprobeDoc kiểm tra số luồng >= 1, codec_name, kích thước khung hình, tốc độ khung hình, kênh âm thanh và duration > 0.
func validateFFprobeDoc(doc ffprobeDoc) (int, error) {
	if len(doc.Streams) == 0 {
		return 0, fmt.Errorf("[SEN REJECTED] invalid media output: 0 streams detected")
	}
	needsDuration := false
	maxDuration := parseAnyFloat(doc.Format.Duration)

	for i, s := range doc.Streams {
		if strings.TrimSpace(s.CodecName) == "" {
			return 0, fmt.Errorf("[SEN REJECTED] invalid media output: stream #%d missing codec_name", i)
		}
		if d := parseAnyFloat(s.Duration); d > maxDuration {
			maxDuration = d
		}
		ctype := strings.ToLower(strings.TrimSpace(s.CodecType))
		if ctype == "video" {
			needsDuration = true
			if err := validateVideoStream(i, s); err != nil {
				return 0, err
			}
		} else if ctype == "audio" {
			needsDuration = true
			if err := validateAudioStream(i, s); err != nil {
				return 0, err
			}
		}
	}
	if needsDuration && maxDuration <= 0 {
		return 0, fmt.Errorf("[SEN REJECTED] invalid media output: media duration must be > 0")
	}
	return len(doc.Streams), nil
}

// validateVideoStream bắt buộc width > 0, height > 0 và tỷ lệ khung hình khác 0/0 đối với luồng hình ảnh động.
func validateVideoStream(idx int, s ffprobeStream) error {
	if s.Width <= 0 || s.Height <= 0 {
		return fmt.Errorf("[SEN REJECTED] invalid media output: video stream #%d has invalid resolution %dx%d", idx, s.Width, s.Height)
	}
	if isZeroFrameRate(s.RFrameRate) || isZeroFrameRate(s.AvgFrameRate) {
		return fmt.Errorf("[SEN REJECTED] invalid media output: video stream #%d has invalid 0/0 frame rate", idx)
	}
	return nil
}

// validateAudioStream bắt buộc số kênh âm thanh > 0 và tần số lấy mẫu (sample_rate) > 0 nếu trường đó xuất hiện.
func validateAudioStream(idx int, s ffprobeStream) error {
	if s.Channels <= 0 {
		return fmt.Errorf("[SEN REJECTED] invalid media output: audio stream #%d has 0 channels", idx)
	}
	if s.SampleRate != nil && strings.TrimSpace(fmt.Sprint(s.SampleRate)) != "" {
		if parseAnyFloat(s.SampleRate) <= 0 {
			return fmt.Errorf("[SEN REJECTED] invalid media output: audio stream #%d has invalid sample_rate", idx)
		}
	}
	return nil
}

// isZeroFrameRate phát hiện chuỗi tỷ lệ khung hình bất hợp lệ như "0/0" hoặc "0".
func isZeroFrameRate(rate string) bool {
	r := strings.TrimSpace(rate)
	if r == "" {
		return false
	}
	if r == "0/0" || r == "0" {
		return true
	}
	parts := strings.Split(r, "/")
	if len(parts) == 2 {
		num, err1 := strconv.ParseFloat(parts[0], 64)
		den, err2 := strconv.ParseFloat(parts[1], 64)
		if err1 == nil && err2 == nil && (num <= 0 || den <= 0) {
			return true
		}
	}
	return false
}

// parseFFprobeKeyValue bóc tách định dạng văn bản key=value ([STREAM]...[/STREAM], [FORMAT]...[/FORMAT]) của ffprobe.
func parseFFprobeKeyValue(text string) (ffprobeDoc, bool) {
	var doc ffprobeDoc
	var cur *ffprobeStream
	inFormat := false
	foundAny := false

	for _, rawLine := range strings.Split(text, "\n") {
		line := strings.TrimSpace(rawLine)
		switch strings.ToUpper(line) {
		case "[STREAM]":
			doc.Streams = append(doc.Streams, ffprobeStream{})
			cur = &doc.Streams[len(doc.Streams)-1]
			inFormat = false
			continue
		case "[/STREAM]":
			cur = nil
			continue
		case "[FORMAT]":
			cur = nil
			inFormat = true
			continue
		case "[/FORMAT]":
			inFormat = false
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		k = strings.ToLower(strings.TrimSpace(k))
		v = strings.TrimSpace(v)
		if cur == nil && !inFormat && (k == "codec_name" || k == "codec_type" || k == "width") {
			doc.Streams = append(doc.Streams, ffprobeStream{})
			cur = &doc.Streams[len(doc.Streams)-1]
		}
		if applyFFprobeKV(k, v, cur, &doc.Format, inFormat) {
			foundAny = true
		}
	}
	return doc, foundAny
}

// applyFFprobeKV gán cặp khóa-giá trị vào cấu trúc luồng hoặc vùng chứa tương ứng.
func applyFFprobeKV(k, v string, cur *ffprobeStream, fmtSec *ffprobeFormat, inFormat bool) bool {
	if inFormat && k == "duration" {
		fmtSec.Duration = v
		return true
	}
	if cur == nil {
		if k == "duration" {
			fmtSec.Duration = v
			return true
		}
		return false
	}
	switch k {
	case "codec_type":
		cur.CodecType = v
	case "codec_name":
		cur.CodecName = v
	case "width":
		cur.Width, _ = strconv.Atoi(v)
	case "height":
		cur.Height, _ = strconv.Atoi(v)
	case "r_frame_rate":
		cur.RFrameRate = v
	case "avg_frame_rate":
		cur.AvgFrameRate = v
	case "channels":
		cur.Channels, _ = strconv.Atoi(v)
	case "sample_rate":
		cur.SampleRate = v
	case "duration":
		cur.Duration = v
	default:
		return false
	}
	return true
}

// parseAnyFloat chuyển đổi linh hoạt giá trị JSON dạng chuỗi hoặc số thực sang float64.
func parseAnyFloat(v any) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case int:
		return float64(val)
	case string:
		f, _ := strconv.ParseFloat(strings.TrimSpace(val), 64)
		return f
	default:
		return 0
	}
}

