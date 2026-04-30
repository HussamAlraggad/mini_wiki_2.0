//go:build !linux && !darwin

package ollama

import (
	"os/exec"
	"syscall"
)

// sysProcAttrNewProcessGroup returns nil on unsupported platforms.
// Process group killing is not supported, so we fall back to killing
// just the direct child process.
func sysProcAttrNewProcessGroup() *syscall.SysProcAttr {
	return nil
}

// killProcessGroup kills the process directly on unsupported platforms.
func killProcessGroup(cmd *exec.Cmd) {
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Kill()
}
