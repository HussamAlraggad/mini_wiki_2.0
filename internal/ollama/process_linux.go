//go:build linux

package ollama

import (
	"os/exec"
	"syscall"
)

// sysProcAttrNewProcessGroup returns SysProcAttr configured to create a new
// process group on Linux. This allows killing the process and all its children.
func sysProcAttrNewProcessGroup() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		Setpgid: true,
	}
}

// killProcessGroup sends SIGTERM to the entire process group of the given command.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	// Negative PID means the process group
	pgid := -cmd.Process.Pid
	_ = syscall.Kill(pgid, syscall.SIGTERM)
}
