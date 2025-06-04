//go:build !windows

package sshd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"syscall"
	"unsafe"

	"github.com/creack/pty"
	"github.com/gliderlabs/ssh"
)

func handlePty(session io.ReadWriter, ptyReq ssh.Pty, winCh <-chan ssh.Window, cmd *exec.Cmd) error {
	cmd.Env = append(cmd.Env, fmt.Sprintf("TERM=%s", ptyReq.Term))
	ptyFile, err := pty.Start(cmd)
	if err != nil {
		return err
	}

	defer ptyFile.Close()

	go syncWinSize(ptyFile, winCh)
	go func() {
		_, _ = io.Copy(ptyFile, session) // stdin
	}()
	_, _ = io.Copy(session, ptyFile) // stdout

	return nil
}

func syncWinSize(ptyFile *os.File, winCh <-chan ssh.Window) {
	for win := range winCh {
		setWinsize(ptyFile, win.Width, win.Height)
	}
}

func setWinsize(f *os.File, w, h int) {
	syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCSWINSZ),
		uintptr(unsafe.Pointer(&struct{ h, w, x, y uint16 }{uint16(h), uint16(w), 0, 0})))
}
