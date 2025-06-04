//go:build windows

package execbackground

import "os/exec"

func SetCreateSession(cmd *exec.Cmd) {
	panic("not supported")
}
