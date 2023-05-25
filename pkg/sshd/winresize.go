package sshd

import (
	"os"
	"syscall"
	"unsafe"

	"github.com/gliderlabs/ssh"
)

func syncWinSize(ptyFile *os.File, winCh <-chan ssh.Window) {
	for win := range winCh {
		setWinsize(ptyFile, win.Width, win.Height)
	}
}

func setWinsize(f *os.File, w, h int) {
	syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCSWINSZ),
		uintptr(unsafe.Pointer(&struct{ h, w, x, y uint16 }{uint16(h), uint16(w), 0, 0})))
}
