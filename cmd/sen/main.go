// Package main cung cấp điểm vào CLI của Sen-VN v1.1.0 với đúng 8 lệnh cố định (Scope Freeze).
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

// Config lưu trữ cấu hình kiểm chứng, chế độ tự hành và chuyên ngành tại .sen/config.json của từng workspace.
type Config struct {
	Version          string   `json:"version"`
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

// printUsage in bảng hướng dẫn sử dụng đúng 8 lệnh cốt lõi của Sen-VN v1.1.0.
func printUsage(w io.Writer) {
	fmt.Fprintln(w, `Sen-VN v1.1.0 — Deterministic AI Coding Referee (8 Commands)

Usage:
  sen init [--with-companions] [--install-companions] [--packs <list|all|csv>] [--profile <auto|code|web|video|media|docs>] [--express] [--agy-hooks] [--update]
  sen brief [--path <file>] [--hook [event]]
  sen intake --quote "<raw>" --req "<summary>" [--bugfix] [--security] [--vn-law] [--express]
  sen intake --rule "<text>" [--quote "<raw>"] [--glob "*"] [--replace <id>] [--delete <id>]
  sen begin <task_id> --reqs R1,R2 --files f1,f2 [--kind feature|bugfix|ui] [--allow-delete] [--express] [--title "..."]
  sen verify <task_id> -- <bin> [args...]
  sen undo [task_id]
  sen done [-- <suite_bin> <args...>]
  sen status [--approve] [--express]`)
}

// RunInDir điều phối và thực thi một trong 8 lệnh CLI cấp cao nhất của Sen-VN; từ chối mọi lệnh lạ.
func RunInDir(cwd string, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		printUsage(stderr)
		return 1
	}
	cmd := strings.TrimSpace(args[0])
	rest := args[1:]

	var err error
	switch cmd {
	case "init":
		err = cmdInit(cwd, rest, stdout)
	case "brief":
		err = cmdBrief(cwd, rest, stdin, stdout)
	case "intake":
		err = cmdIntake(cwd, rest, stdout)
	case "begin":
		err = cmdBegin(cwd, rest, stdout)
	case "verify":
		err = cmdVerify(cwd, rest, stdout)
	case "undo":
		err = cmdUndo(cwd, rest, stdout)
	case "done":
		err = cmdDone(cwd, rest, stdout)
	case "status":
		err = cmdStatus(cwd, rest, stdout)
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

// resolveProjectRoot xác định thư mục gốc của dự án (kể cả khi hook chạy từ bên trong .agents).
func resolveProjectRoot(cwd string) string {
	if fileExists(filepath.Join(cwd, ".sen", "state.db")) {
		return cwd
	}
	if strings.EqualFold(filepath.Base(cwd), ".agents") {
		return filepath.Dir(cwd)
	}
	return cwd
}

// openExistingStore mở sổ cái SQLite .sen/state.db hiện có, báo lỗi rõ ràng nếu chưa chạy sen init.
func openExistingStore(root string) (*storage.Store, error) {
	dbPath := filepath.Join(root, ".sen", "state.db")
	if !fileExists(dbPath) {
		return nil, fmt.Errorf("workspace not initialized (.sen/state.db missing); run `sen init` first")
	}
	return storage.Open(dbPath)
}

// loadConfig đọc .sen/config.json và bắt buộc đối chiếu chữ ký SHA-256 trong .sen/config.sha256 để chống sửa trộm.
func loadConfig(root string) (Config, error) {
	resolvedDefault := workspace.ResolveDomainProfile(root, "auto")
	cfg := Config{
		Version:          "1.1",
		AllowedBinaries:  verify.DefaultAllowedBinaries(),
		ProjectVerifyCmd: detectDefaultVerifyCmd(root, resolvedDefault),
		TimeoutSeconds:   120,
		DomainProfile:    "auto",
	}
	cfgPath := filepath.Join(root, ".sen", "config.json")
	raw, err := os.ReadFile(cfgPath)
	if err == nil && len(raw) > 0 {
		sumPath := filepath.Join(root, ".sen", "config.sha256")
		expectedBytes, sErr := os.ReadFile(sumPath)
		if sErr != nil || strings.TrimSpace(string(expectedBytes)) != sha256Hex(raw) {
			return cfg, fmt.Errorf(".sen/config.json integrity hash mismatch (tampering detected)")
		}
		cfg.ProjectVerifyCmd = nil
		if jErr := json.Unmarshal(raw, &cfg); jErr != nil {
			return cfg, fmt.Errorf("invalid .sen/config.json: %w", jErr)
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
