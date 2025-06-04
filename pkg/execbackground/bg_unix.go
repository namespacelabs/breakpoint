//go:build !windows

package execbackground

import (
	"os/exec"
	"syscall"
)

func SetCreateSession(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setsid: true,
	}
}
