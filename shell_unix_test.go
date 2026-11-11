//go:build !windows

package senmvp_test

import "os/exec"

func newHookShellCommand(dir string, shellCommand string) *exec.Cmd {
	cmd := exec.Command("sh", "-c", shellCommand)
	cmd.Dir = dir
	return cmd
}
