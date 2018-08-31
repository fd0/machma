package main

import (
	"os/exec"
)

func createProcessGroup(cmd *exec.Cmd) {
	// noop on Windows
}

func killProcessGroup(cmd *exec.Cmd) error {
	return cmd.Process.Kill()
}
