// Tệp agy_media_guard.go kiểm soát quyết định ghi tệp theo chế độ --express và bảo vệ tư liệu gốc trước lệnh ffmpeg/magick.
package bridge

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/developerfs0110/Sen-VN/internal/storage"
	"github.com/developerfs0110/Sen-VN/internal/workspace"
)

// expressTriggerPhrases liệt kê các cụm từ ủy quyền tự hành (tiếng Việt và tiếng Anh) từ câu nhắc của người dùng.
var expressTriggerPhrases = []string{
	"làm luôn đi",
	"lam luon di",
	"just do it",
	"chạy thẳng",
	"chay thang",
}

// ffmpegValueFlags liệt kê các cờ nhận đối số đi kèm trong câu lệnh ffmpeg để phân biệt tệp đầu ra ở cuối lệnh.
var ffmpegValueFlags = map[string]bool{
	"-i": true, "-vf": true, "-af": true, "-filter_complex": true, "-lavfi": true,
	"-c": true, "-c:v": true, "-c:a": true, "-c:s": true, "-vcodec": true, "-acodec": true,
	"-b:v": true, "-b:a": true, "-preset": true, "-crf": true, "-pix_fmt": true,
	"-map": true, "-r": true, "-s": true, "-t": true, "-ss": true, "-to": true,
	"-f": true, "-loglevel": true, "-v": true, "-threads": true, "-metadata": true, "-ar": true, "-ac": true,
}

// ContainsExpressPhrase kiểm tra không phân biệt hoa thường xem người dùng có ủy quyền chạy tự hành (--express) hay không.
func ContainsExpressPhrase(input string) bool {
	lower := strings.ToLower(input)
	for _, phrase := range expressTriggerPhrases {
		if strings.Contains(lower, phrase) {
			return true
		}
	}
	return false
}

// evaluateWriteToolDecision áp dụng khóa .sen/, khóa test/raw, kiểm tra chữ ký config.sha256 và phân quyền theo --express.
func evaluateWriteToolDecision(st *storage.Store, root, toolName string, args map[string]any, stdout io.Writer) error {
	rawPath := extractStringArg(args, "Path", "TargetFile", "file_path", "path")
	if rawPath == "" {
		return writeJSON(stdout, map[string]any{"decision": "allow"})
	}
	rel, err := workspace.NormalizeRelPath(root, rawPath)
	if err != nil {
		return writeJSON(stdout, map[string]any{"decision": "allow"})
	}
	lowerRel := strings.ToLower(rel)
	if lowerRel == ".sen" || strings.HasPrefix(lowerRel, ".sen/") {
		return writeJSON(stdout, map[string]any{
			"decision": "deny",
			"reason":   "Protected .sen/ ledger cannot be edited by agent tools.",
		})
	}
	if st == nil {
		return writeJSON(stdout, map[string]any{"decision": "allow"})
	}
	expressMode, _, tampered := ReadVerifiedRuntimeConfig(root)
	if tampered {
		return writeJSON(stdout, map[string]any{
			"decision": "deny",
			"reason":   ".sen/config.json integrity hash mismatch (tampering detected); restore valid config via `sen init`.",
		})
	}
	if lockedReason := checkLockedAssetReason(st, rel); lockedReason != "" {
		return writeJSON(stdout, map[string]any{"decision": "deny", "reason": lockedReason})
	}

	openTask := findOpenTask(st)
	if openTask == nil {
		return writeJSON(stdout, map[string]any{
			"decision": "deny",
			"reason":   "No active SEN task. Run sen intake and sen begin first.",
		})
	}
	if !isFileInTaskScope(root, rel, openTask.ScopeFiles) {
		return writeJSON(stdout, map[string]any{
			"decision": "deny",
			"reason":   fmt.Sprintf("File %q is outside active OPEN task %q scope_files %v.", rel, openTask.ID, openTask.ScopeFiles),
		})
	}
	return decideRiskOrExpressApproval(root, rel, toolName, openTask, expressMode, stdout)
}

// decideRiskOrExpressApproval cho qua SMALL/MEDIUM/LARGE thường khi bật --express, nhưng phanh gấp nếu xóa tệp hoặc chạm đường dẫn trọng yếu.
func decideRiskOrExpressApproval(root, rel, toolName string, openTask *storage.Task, expressMode bool, stdout io.Writer) error {
	isCrit, critReason := workspace.IsDestructiveOrCriticalFile(root, rel)
	hasDelete := workspace.TaskAllowsDelete(root, openTask.ID) || toolName == "delete_file"

	if expressMode {
		if hasDelete {
			return writeJSON(stdout, map[string]any{
				"decision": "force_ask",
				"reason":   fmt.Sprintf("Express mode brake: task %q involves file deletion; confirm editing %q.", openTask.ID, rel),
			})
		}
		if isCrit {
			return writeJSON(stdout, map[string]any{
				"decision": "force_ask",
				"reason":   fmt.Sprintf("Express mode brake (%s); confirm editing %q.", critReason, rel),
			})
		}
		return writeJSON(stdout, map[string]any{"decision": "allow"})
	}

	if strings.EqualFold(openTask.RiskAuto, "LARGE") || strings.EqualFold(openTask.RiskAuto, "HIGH") {
		return writeJSON(stdout, map[string]any{
			"decision": "force_ask",
			"reason":   fmt.Sprintf("Active Sen task %q has risk=%s; confirm editing %q.", openTask.ID, openTask.RiskAuto, rel),
		})
	}
	if isCrit {
		return writeJSON(stdout, map[string]any{
			"decision": "force_ask",
			"reason":   fmt.Sprintf("File %q is classified as LARGE risk (%s).", rel, critReason),
		})
	}
	return writeJSON(stdout, map[string]any{"decision": "allow"})
}

// checkLockedAssetReason trả về thông báo chặn nếu tệp là bài kiểm thử hoặc tư liệu gốc đã khóa trong sổ cái.
func checkLockedAssetReason(st *storage.Store, rel string) string {
	if !workspace.IsLockedAsset(rel) {
		return ""
	}
	targetKey := workspace.PathKey(rel)
	locked := false
	snaps, _ := st.GetSnapshots(workspace.BaselineTaskID)
	for _, s := range snaps {
		if s.IsLockedTest && s.ExistedBefore && workspace.PathKey(s.RelPath) == targetKey {
			locked = true
			break
		}
	}
	if !locked {
		if open := findOpenTask(st); open != nil {
			tsnaps, _ := st.GetSnapshots(open.ID)
			for _, s := range tsnaps {
				if s.IsLockedTest && s.ExistedBefore && workspace.PathKey(s.RelPath) == targetKey {
					locked = true
					break
				}
			}
		}
	}
	if !locked {
		return ""
	}
	if workspace.IsLockedRawMedia(rel) {
		return fmt.Sprintf("Pre-existing raw/master media file %q is locked (immutable) by Sen.", rel)
	}
	return fmt.Sprintf("Pre-existing test file %q is locked by Sen; add a new test file via `sen begin --files`.", rel)
}

// isFileInTaskScope đối chiếu đường dẫn đã chuẩn hóa với danh sách scope_files của task đang mở.
func isFileInTaskScope(root, rel string, scopeFiles []string) bool {
	target := workspace.PathKey(rel)
	for _, sf := range scopeFiles {
		if nsf, err := workspace.NormalizeRelPath(root, sf); err == nil && workspace.PathKey(nsf) == target {
			return true
		}
	}
	return false
}

// commandMutatesRawMedia phát hiện lệnh ffmpeg/magick ghi đầu ra vào thư mục tư liệu gốc hoặc ghi đè trực tiếp lên tệp đầu vào.
func commandMutatesRawMedia(cmdLine string) string {
	tokens := splitShellTokens(cmdLine)
	for idx := 0; idx < len(tokens); idx++ {
		base := strings.ToLower(filepath.Base(strings.ReplaceAll(tokens[idx], "\\", "/")))
		base = strings.TrimSuffix(base, ".exe")
		if base == "ffmpeg" {
			if reason := inspectFFmpegSegment(tokens[idx+1:]); reason != "" {
				return reason
			}
		} else if base == "magick" {
			if reason := inspectMagickSegment(tokens[idx+1:]); reason != "" {
				return reason
			}
		}
	}
	return ""
}

// inspectFFmpegSegment phân tích các cờ -i và đối số đầu ra của ffmpeg để chặn ghi đè tại chỗ hoặc ghi vào raw/master.
func inspectFFmpegSegment(args []string) string {
	inputs := make(map[string]bool)
	var outputs []string
	for i := 0; i < len(args); i++ {
		tok := args[i]
		if tok == ";" || tok == "&&" || tok == "||" || tok == "|" {
			break
		}
		if tok == "-i" && i+1 < len(args) {
			i++
			inClean := filepath.ToSlash(strings.ReplaceAll(args[i], "\\", "/"))
			inputs[workspace.PathKey(inClean)] = true
			continue
		}
		if ffmpegValueFlags[strings.ToLower(tok)] && i+1 < len(args) {
			i++
			continue
		}
		if strings.HasPrefix(tok, "-") {
			continue
		}
		outputs = append(outputs, tok)
	}
	for _, outTok := range outputs {
		cleanOut := filepath.ToSlash(strings.ReplaceAll(outTok, "\\", "/"))
		if workspace.IsLockedRawMedia(cleanOut) {
			return fmt.Sprintf("Blocked by Sen: ffmpeg cannot write output into immutable raw/master media folder (%s).", outTok)
		}
		if inputs[workspace.PathKey(cleanOut)] {
			return fmt.Sprintf("Blocked by Sen: ffmpeg cannot overwrite input media file in-place (%s); write to a separate output file.", outTok)
		}
	}
	return ""
}

// inspectMagickSegment chặn lệnh ImageMagick mogrify trên tư liệu gốc hoặc xuất tệp đầu ra đè vào thư mục raw/master.
func inspectMagickSegment(args []string) string {
	var pos []string
	isMogrify := false
	for i := 0; i < len(args); i++ {
		tok := args[i]
		if tok == ";" || tok == "&&" || tok == "||" || tok == "|" {
			break
		}
		if strings.EqualFold(tok, "mogrify") {
			isMogrify = true
			continue
		}
		if strings.HasPrefix(tok, "-") {
			continue
		}
		pos = append(pos, tok)
	}
	if isMogrify {
		for _, p := range pos {
			if workspace.IsLockedRawMedia(p) {
				return fmt.Sprintf("Blocked by Sen: magick mogrify cannot mutate immutable raw/master media (%s).", p)
			}
		}
	}
	if len(pos) >= 1 {
		outTok := pos[len(pos)-1]
		cleanOut := filepath.ToSlash(strings.ReplaceAll(outTok, "\\", "/"))
		if workspace.IsLockedRawMedia(cleanOut) {
			return fmt.Sprintf("Blocked by Sen: magick cannot write output into immutable raw/master media folder (%s).", outTok)
		}
		if len(pos) >= 2 && workspace.PathKey(filepath.ToSlash(strings.ReplaceAll(pos[0], "\\", "/"))) == workspace.PathKey(cleanOut) {
			return fmt.Sprintf("Blocked by Sen: magick cannot overwrite source media file in-place (%s).", outTok)
		}
	}
	return ""
}

// splitShellTokens tách chuỗi lệnh thành các token có tôn trọng cặp dấu nháy đơn và nháy kép.
func splitShellTokens(s string) []string {
	var out []string
	var cur strings.Builder
	var quote rune
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '"' || r == '\'':
			quote = r
		case r == ' ' || r == '\t' || r == '\r' || r == '\n':
			if cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
			}
		default:
			cur.WriteRune(r)
		}
	}
	if cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out
}

