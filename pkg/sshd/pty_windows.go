//go:build windows

package sshd

import (
	"io"
	"os/exec"

	"github.com/gliderlabs/ssh"
)

func handlePty(session io.ReadWriter, _ ssh.Pty, _ <-chan ssh.Window, cmd *exec.Cmd) error {
	cmd.Stdin = session
	cmd.Stdout = session
	cmd.Stderr = session
	return cmd.Start()
}
