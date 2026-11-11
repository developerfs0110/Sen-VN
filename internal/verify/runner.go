// Tệp runner.go khởi chạy tiến trình con trực tiếp, khóa vòng lặp sửa lỗi <= 3 vòng (Quarantine) và ghi nhận bằng chứng.
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

// RunTaskVerification kiểm tra ngưỡng Quarantine <= 3 vòng, tính toàn vẹn trước & sau khi chạy lệnh và ghi chuỗi evidence.
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
	failedBefore, err := workspace.ConsumeQuarantineResetForAttempt(st, root, taskID)
	if err != nil {
		return nil, err
	}
	if failedBefore >= workspace.MaxRepairRounds {
		return nil, fmt.Errorf(
			"bounded repair loop exceeded (3/3 consecutive failed verify attempts on task %q); present objective dialectical root-cause to user (unlock via 'sen status --approve') or rollback via 'sen undo %s'",
			taskID, taskID,
		)
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
	return finalizeTaskVerification(st, root, task, failedBefore, res)
}

// finalizeTaskVerification áp dụng chính sách BUGFIX, gắn nhãn [QUARANTINE 3/3] ở lần lỗi thứ 3 hoặc nâng mốc __BASELINE__ khi đạt.
func finalizeTaskVerification(
	st *storage.Store,
	root string,
	task *storage.Task,
	failedBefore int,
	res *execResult,
) (*storage.Evidence, error) {
	policyErr := res.zeroTestErr
	if policyErr == nil && taskRequiresTestStrength(st, task) && (res.strength != "TEST" || res.testsRan <= 0) {
		policyErr = fmt.Errorf("BUGFIX task %q requires TEST verification strength with tests_ran > 0 (got %s, tests_ran=%d)",
			task.ID, res.strength, res.testsRan)
	}

	storedExit := res.rawExitCode
	storedOut := res.redactedOutput
	if storedExit == 0 && policyErr != nil {
		storedExit = 1
		tag := policyErr.Error()
		if !strings.HasPrefix(tag, "[SEN REJECTED]") {
			tag = "[SEN REJECTED] " + tag
		}
		storedOut = storedOut + "\n" + tag
	}

	ev, err := st.AppendEvidence(task.ID, res.commandLine, res.strength, res.testsRan, storedExit, storedOut)
	if err != nil {
		return nil, err
	}
	if policyErr != nil || res.rawExitCode != 0 || res.execErr != nil {
		retErr := policyErr
		if retErr == nil {
			retErr = fmt.Errorf("verification failed (exit %d): %s", res.rawExitCode, res.commandLine)
		}
		if failedBefore+1 >= workspace.MaxRepairRounds {
			retErr = fmt.Errorf("[QUARANTINE 3/3] %w", retErr)
		}
		return ev, retErr
	}

	if err := workspace.AdvanceBaseline(st, root, task.ScopeFiles); err != nil {
		return ev, fmt.Errorf("advance baseline: %w", err)
	}
	if err := st.UpdateTaskStatus(task.ID, "VERIFIED"); err != nil {
		return ev, err
	}
	workspace.ClearQuarantineReset(root, task.ID)
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
