// +build !windows

package main

import (
	"os/exec"
	"syscall"
)

func createProcessGroup(cmd *exec.Cmd) {
	// make sure the new process and all children get a new process group ID
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

func killProcessGroup(cmd *exec.Cmd) error {
	return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
}
