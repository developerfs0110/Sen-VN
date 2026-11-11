// Package main cung cấp điểm vào CLI của Sen-VN v1.2.0 với đúng 9 lệnh cố định (bao gồm động cơ SSDS v1.0.0).
package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/developerfs0110/Sen-VN/internal/storage"
	"github.com/developerfs0110/Sen-VN/internal/verify"
	"github.com/developerfs0110/Sen-VN/internal/workspace"
)

// Config lưu trữ cấu hình kiểm chứng, phiên bản SSDS đã ghim, định danh dự án, chế độ tự hành và chuyên ngành tại .sen/config.json.
type Config struct {
	Version          string   `json:"version"`
	SSDSVersion      string   `json:"ssds_version,omitempty"`
	ProjectID        string   `json:"project_id,omitempty"`
	AllowedBinaries  []string `json:"allowed_binaries"`
	ProjectVerifyCmd []string `json:"project_verify_cmd"`
	TimeoutSeconds   int      `json:"timeout_seconds"`
	ExpressMode      bool     `json:"express_mode"`
	DomainProfile    string   `json:"domain_profile"`
}

// main khởi chạy CLI dựa trên thư mục làm việc hiện tại và trả mã thoát chuẩn cho hệ điều hành.
func main() {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Fprintf(os.Stderr, "sen: getwd error: %v\n", err)
		os.Exit(1)
	}
	os.Exit(RunInDir(cwd, os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// printUsage in bảng hướng dẫn sử dụng đúng 9 lệnh cốt lõi của Sen-VN v1.2.0 + SSDS v1.0.0.
func printUsage(w io.Writer) {
	fmt.Fprintln(w, `Sen-VN v1.2.0 — Deterministic AI Coding & SSDS Referee (9 Commands)

Usage:
  sen [--project <dir>] init [--with-companions] [--install-companions] [--packs <list|all|csv>] [--profile <auto|code|web|video|media|docs>] [--express] [--agy-hooks] [--update]
  sen [--project <dir>] brief [--path <file>] [--hook [event]]
  sen [--project <dir>] intake --quote "<raw>" --req "<summary>" [--bugfix] [--security] [--vn-law] [--express]
  sen [--project <dir>] intake --rule "<text>" [--quote "<raw>"] [--glob "*"] [--replace <id>] [--delete <id>]
  sen [--project <dir>] begin <task_id> --reqs R1,R2 --files f1,f2 [--phase P5] [--components C1,C2] [--kind feature|bugfix|ui] [--allow-delete] [--express] [--title "..."]
  sen [--project <dir>] verify <task_id> -- <bin> [args...]
  sen [--project <dir>] undo [task_id]
  sen [--project <dir>] done [-- <suite_bin> <args...>]
  sen [--project <dir>] status [--approve] [--express]
  sen [--project <dir>] sdlc <classify|status|gate|review|approve|impact|exception|sync|version> [flags]`)
}

// RunInDir điều phối và thực thi một trong 9 lệnh CLI cấp cao nhất của Sen-VN (hỗ trợ cờ --project cô lập đa dự án).
func RunInDir(cwd string, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	effCwd, remArgs, flagErr := consumeLeadingProjectFlag(cwd, args)
	if flagErr != nil {
		fmt.Fprintf(stderr, "sen error: %v\n", flagErr)
		return 1
	}
	if len(remArgs) == 0 {
		printUsage(stderr)
		return 1
	}
	cmd := strings.TrimSpace(remArgs[0])
	rest := remArgs[1:]

	var err error
	switch cmd {
	case "init":
		err = cmdInit(effCwd, rest, stdout)
	case "brief":
		err = cmdBrief(effCwd, rest, stdin, stdout)
	case "intake":
		err = cmdIntake(effCwd, rest, stdout)
	case "begin":
		err = cmdBegin(effCwd, rest, stdout)
	case "verify":
		err = cmdVerify(effCwd, rest, stdout)
	case "undo":
		err = cmdUndo(effCwd, rest, stdout)
	case "done":
		err = cmdDone(effCwd, rest, stdout)
	case "status":
		err = cmdStatus(effCwd, rest, stdout)
	case "sdlc":
		err = cmdSDLC(effCwd, rest, stdout)
	default:
		fmt.Fprintf(stderr, "sen: unknown command %q\n\n", cmd)
		printUsage(stderr)
		return 1
	}

	if err != nil {
		fmt.Fprintf(stderr, "sen %s error: %v\n", cmd, err)
		return 1
	}
	return 0
}

// consumeLeadingProjectFlag trích xuất cờ tùy chọn --project <dir> đứng trước tên lệnh con để chỉ định đúng dự án đích.
func consumeLeadingProjectFlag(cwd string, args []string) (string, []string, error) {
	effCwd := cwd
	for len(args) > 0 {
		tok := strings.TrimSpace(args[0])
		if tok == "--project" || tok == "-project" {
			if len(args) < 2 || strings.TrimSpace(args[1]) == "" {
				return "", nil, fmt.Errorf("--project requires a non-empty directory path")
			}
			target := strings.TrimSpace(args[1])
			if !filepath.IsAbs(target) {
				target = filepath.Join(effCwd, target)
			}
			effCwd = filepath.Clean(target)
			args = args[2:]
			continue
		}
		if strings.HasPrefix(tok, "--project=") || strings.HasPrefix(tok, "-project=") {
			idx := strings.IndexByte(tok, '=')
			target := strings.TrimSpace(tok[idx+1:])
			if target == "" {
				return "", nil, fmt.Errorf("--project requires a non-empty directory path")
			}
			if !filepath.IsAbs(target) {
				target = filepath.Join(effCwd, target)
			}
			effCwd = filepath.Clean(target)
			args = args[1:]
			continue
		}
		break
	}
	return effCwd, args, nil
}

// resolveProjectRoot định vị ranh giới gốc của dự án gần nhất mà không để rò rỉ sang Workspace mẹ bên ngoài.
func resolveProjectRoot(cwd string) string {
	projRoot, _ := workspace.ResolveNearestProjectRoot("", cwd)
	if strings.TrimSpace(projRoot) != "" {
		return projRoot
	}
	return cwd
}

// openExistingStore mở sổ cái SQLite .sen/state.db hiện có, báo lỗi rõ ràng nếu chưa chạy sen init.
func openExistingStore(root string) (*storage.Store, error) {
	dbPath := filepath.Join(root, ".sen", "state.db")
	if !fileExists(dbPath) {
		return nil, fmt.Errorf("workspace not initialized (.sen/state.db missing); run `sen init` or `sen sdlc classify` first")
	}
	return storage.Open(dbPath)
}

// loadConfig đọc .sen/config.json và bắt buộc đối chiếu chữ ký SHA-256 trong .sen/config.sha256 để chống sửa trộm.
func loadConfig(root string) (Config, error) {
	resolvedDefault := workspace.ResolveDomainProfile(root, "auto")
	cfg := Config{
		Version:          "1.2",
		AllowedBinaries:  verify.DefaultAllowedBinaries(),
		ProjectVerifyCmd: detectDefaultVerifyCmd(root, resolvedDefault),
		TimeoutSeconds:   120,
		DomainProfile:    "auto",
	}
	cfgPath := filepath.Join(root, ".sen", "config.json")
	sumPath := filepath.Join(root, ".sen", "config.sha256")
	hasCfg := fileExists(cfgPath)
	hasSum := fileExists(sumPath)
	if hasCfg || hasSum {
		raw, err := os.ReadFile(cfgPath)
		if err != nil || len(raw) == 0 {
			return cfg, fmt.Errorf(".sen/config.json missing or empty while signature exists (tampering detected)")
		}
		expectedBytes, sErr := os.ReadFile(sumPath)
		if sErr != nil || strings.TrimSpace(string(expectedBytes)) != sha256Hex(raw) {
			return cfg, fmt.Errorf(".sen/config.json integrity hash mismatch (tampering detected)")
		}
		cfg.ProjectVerifyCmd = nil
		if jErr := json.Unmarshal(raw, &cfg); jErr != nil {
			return cfg, fmt.Errorf("invalid .sen/config.json: %w", jErr)
		}
		if len(cfg.ProjectVerifyCmd) == 0 {
			cfg.ProjectVerifyCmd = detectDefaultVerifyCmd(root, workspace.ResolveDomainProfile(root, cfg.DomainProfile))
		}
	}
	if len(cfg.AllowedBinaries) == 0 {
		cfg.AllowedBinaries = verify.DefaultAllowedBinaries()
	}
	if cfg.TimeoutSeconds <= 0 {
		cfg.TimeoutSeconds = 120
	}
	if strings.TrimSpace(cfg.DomainProfile) == "" {
		cfg.DomainProfile = "auto"
	}
	return cfg, nil
}

// saveConfig ghi .sen/config.json kèm tệp băm đối chứng .sen/config.sha256.
func saveConfig(root string, cfg Config) error {
	p := filepath.Join(root, ".sen", "config.json")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, ".sen", "config.sha256"), []byte(sha256Hex(data)+"\n"), 0o644)
}

// applyExpressFlagIfVisited cập nhật cờ express_mode vào .sen/config.json nếu người dùng truyền tường minh trên CLI.
func applyExpressFlagIfVisited(root string, fs *flag.FlagSet, val bool) error {
	if !flagVisited(fs, "express") {
		return nil
	}
	cfg, err := loadConfig(root)
	if err != nil {
		return err
	}
	cfg.ExpressMode = val
	return saveConfig(root, cfg)
}

// flagVisited kiểm tra xem một cờ cụ thể có được truyền tường minh trong danh sách tham số CLI hay không.
func flagVisited(fs *flag.FlagSet, name string) bool {
	found := false
	fs.Visit(func(f *flag.Flag) {
		if f.Name == name {
			found = true
		}
	})
	return found
}

// sha256Hex tính mã băm SHA-256 dạng chuỗi thập lục phân viết thường cho mảng byte.
func sha256Hex(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// detectDefaultVerifyCmd tự động nhận diện lệnh kiểm thử mặc định theo ngôn ngữ và chuyên ngành của dự án.
func detectDefaultVerifyCmd(root, resolvedProfile string) []string {
	switch resolvedProfile {
	case "docs", "video", "media":
		return nil
	}
	switch {
	case fileExists(filepath.Join(root, "go.mod")):
		return []string{"go", "test", "-v", "./..."}
	case fileExists(filepath.Join(root, "package.json")):
		return []string{"npm", "test"}
	case fileExists(filepath.Join(root, "pyproject.toml")) || fileExists(filepath.Join(root, "pytest.ini")):
		return []string{"pytest"}
	default:
		return nil
	}
}

// fileExists kiểm tra đường dẫn tồn tại và là tệp thông thường (không phải thư mục).
func fileExists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}

// splitDoubleDash tách danh sách tham số CLI thành phần trước và phần sau dấu phân cách "--".
func splitDoubleDash(args []string) (before, after []string) {
	for i, a := range args {
		if a == "--" {
			return args[:i], args[i+1:]
		}
	}
	return args, nil
}

// reorderFlagsFirst đưa các cờ (--flag) lên trước đối số vị trí để tương thích với bộ phân tích flag chuẩn của Go.
func reorderFlagsFirst(args []string, valueFlags map[string]bool) []string {
	var flags, pos []string
	for i := 0; i < len(args); i++ {
		tok := args[i]
		if strings.HasPrefix(tok, "-") && tok != "-" {
			flags = append(flags, tok)
			name := strings.TrimLeft(tok, "-")
			if !strings.Contains(name, "=") && valueFlags[name] && i+1 < len(args) {
				i++
				flags = append(flags, args[i])
			}
		} else {
			pos = append(pos, tok)
		}
	}
	return append(flags, pos...)
}
