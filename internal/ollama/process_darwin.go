//go:build darwin

package ollama

import (
	"os/exec"
	"syscall"
)

// sysProcAttrNewProcessGroup returns SysProcAttr configured to create a new
// process group on macOS.
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
	pgid := -cmd.Process.Pid
	_ = syscall.Kill(pgid, syscall.SIGTERM)
}
