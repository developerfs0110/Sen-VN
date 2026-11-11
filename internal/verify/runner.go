// Tệp runner.go khởi chạy tiến trình con trực tiếp với môi trường đã lọc secret và ghi nhận bằng chứng cho từng task.
package verify

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/developerfs0110/Sen-VN/internal/redact"
	"github.com/developerfs0110/Sen-VN/internal/storage"
	"github.com/developerfs0110/Sen-VN/internal/workspace"
)

// maxStoredOutputBytes giới hạn dung lượng đầu ra lưu trong SQLite ở mức 64KB để giữ tệp state.db gọn nhẹ.
const maxStoredOutputBytes = 64 * 1024

// execResult đóng gói kết quả thực thi tiến trình con sau khi đã phân tích độ mạnh và ẩn thông tin nhạy cảm.
type execResult struct {
	commandLine    string
	strength       string
	testsRan       int
	rawExitCode    int
	redactedOutput string
	zeroTestErr    error
	execErr        error
}

// executeFilteredSubprocess chạy trực tiếp binary qua exec.CommandContext (không qua shell) với biến môi trường sạch.
func executeFilteredSubprocess(ctx context.Context, root string, allowedBins []string, cmdArgs []string, timeout time.Duration) (*execResult, error) {
	if len(cmdArgs) == 0 {
		return nil, errors.New("missing verification command after --")
	}
	resolvedBin, err := ResolveSafeBinary(cmdArgs[0], allowedBins, root)
	if err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = 2 * time.Minute
	}
	tctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(tctx, resolvedBin, cmdArgs[1:]...)
	cmd.Dir = root
	cmd.Env = redact.FilterEnv(os.Environ())

	var combined bytes.Buffer
	cmd.Stdout = &combined
	cmd.Stderr = &combined

	runErr := cmd.Run()
	exitCode := 0
	if runErr != nil {
		var exitErr *exec.ExitError
		if errors.As(runErr, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	rawOut := combined.String()
	strength, testsRan, zeroErr := ParseTestRun(cmdArgs[0], cmdArgs[1:], rawOut, exitCode)
	redacted := redact.ScrubText(rawOut)
	if len(redacted) > maxStoredOutputBytes {
		redacted = redacted[:maxStoredOutputBytes] + "\n...[truncated]..."
	}

	return &execResult{
		commandLine:    strings.Join(cmdArgs, " "),
		strength:       strength,
		testsRan:       testsRan,
		rawExitCode:    exitCode,
		redactedOutput: redacted,
		zeroTestErr:    zeroErr,
		execErr:        runErr,
	}, nil
}

// RunTaskVerification kiểm tra toàn vẹn trước & sau khi chạy lệnh, ép mức TEST cho BUGFIX, nâng __BASELINE__ và lưu evidence.
func RunTaskVerification(
	ctx context.Context,
	st *storage.Store,
	root, taskID string,
	allowedBins []string,
	cmdArgs []string,
	timeout time.Duration,
) (*storage.Evidence, error) {
	if err := workspace.ValidateTaskID(taskID); err != nil {
		return nil, err
	}
	task, err := st.GetTask(taskID)
	if err != nil {
		return nil, err
	}
	if err := workspace.CheckIntegrity(st, root, taskID, task.ScopeFiles); err != nil {
		return nil, err
	}

	res, err := executeFilteredSubprocess(ctx, root, allowedBins, cmdArgs, timeout)
	if err != nil {
		return nil, err
	}
	// Kiểm tra lại toàn vẹn ngay sau khi tiến trình con kết thúc để chặn test tự sửa tệp ngoài phạm vi.
	if err := workspace.CheckIntegrity(st, root, taskID, task.ScopeFiles); err != nil {
		return nil, err
	}

	policyErr := res.zeroTestErr
	if policyErr == nil && taskRequiresTestStrength(st, task) && (res.strength != "TEST" || res.testsRan <= 0) {
		policyErr = fmt.Errorf("BUGFIX task %q requires TEST verification strength with tests_ran > 0 (got %s, tests_ran=%d)",
			taskID, res.strength, res.testsRan)
	}

	storedExit := res.rawExitCode
	storedOut := res.redactedOutput
	if storedExit == 0 && policyErr != nil {
		storedExit = 1
		storedOut = storedOut + "\n[SEN REJECTED] " + policyErr.Error()
	}

	ev, err := st.AppendEvidence(taskID, res.commandLine, res.strength, res.testsRan, storedExit, storedOut)
	if err != nil {
		return nil, err
	}
	if policyErr != nil {
		return ev, policyErr
	}
	if res.rawExitCode != 0 || res.execErr != nil {
		return ev, fmt.Errorf("verification failed (exit %d): %s", res.rawExitCode, res.commandLine)
	}

	if err := workspace.AdvanceBaseline(st, root, task.ScopeFiles); err != nil {
		return ev, fmt.Errorf("advance baseline: %w", err)
	}
	if err := st.UpdateTaskStatus(taskID, "VERIFIED"); err != nil {
		return ev, err
	}
	return ev, nil
}

// taskRequiresTestStrength xác định xem task có thuộc diện sửa lỗi (BUGFIX) bắt buộc kiểm chứng bằng TEST thực hay không.
func taskRequiresTestStrength(st *storage.Store, task *storage.Task) bool {
	if strings.EqualFold(task.Kind, "BUGFIX") {
		return true
	}
	reqs, err := st.ListRequirements()
	if err != nil {
		return false
	}
	bugMap := make(map[string]bool, len(reqs))
	for _, r := range reqs {
		if r.IsBugfix {
			bugMap[r.ID] = true
		}
	}
	for _, id := range task.ReqIDs {
		if bugMap[id] {
			return true
		}
	}
	return false
}
