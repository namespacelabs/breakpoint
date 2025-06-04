//go:build windows

package sshd

import (
	"errors"
	"io"
	"os/exec"

	"github.com/gliderlabs/ssh"
)

func handlePty(session io.ReadWriter, ptyReq ssh.Pty, winCh <-chan ssh.Window, cmd *exec.Cmd) error {
	return errors.New("pty not supported in windows")
}
