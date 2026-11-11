//go:build windows

package senmvp_test

import (
	"os/exec"
	"syscall"
)

func newHookShellCommand(dir string, shellCommand string) *exec.Cmd {
	cmd := exec.Command("cmd.exe")
	cmd.Dir = dir
	cmd.SysProcAttr = &syscall.SysProcAttr{
		CmdLine: `cmd.exe /d /s /c "` + shellCommand + `"`,
	}
	return cmd
}
