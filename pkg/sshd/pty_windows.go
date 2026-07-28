//go:build windows

package sshd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os/exec"
	"sync"
	"syscall"

	"github.com/charmbracelet/x/conpty"
	"github.com/gliderlabs/ssh"
	"golang.org/x/sys/windows"
)

type conPTYSession struct {
	mu sync.Mutex
	*conpty.ConPty
	stopped bool
}

func startPty(ctx context.Context, session io.ReadWriter, ptyReq ssh.Pty, winCh <-chan ssh.Window, cmd *exec.Cmd) (func() error, error) {
	if err := validateConPTYSize(ptyReq.Window.Width, ptyReq.Window.Height); err != nil {
		return nil, err
	}

	pseudoconsole, err := conpty.New(ptyReq.Window.Width, ptyReq.Window.Height, 0)
	if err != nil {
		return nil, err
	}
	conPTY := &conPTYSession{ConPty: pseudoconsole}

	_, processHandle, err := pseudoconsole.Spawn(cmd.Path, cmd.Args, &syscall.ProcAttr{
		Dir: cmd.Dir,
		Env: cmd.Env,
		Sys: cmd.SysProcAttr,
	})
	if err != nil {
		_ = pseudoconsole.Close()
		return nil, err
	}
	process := windows.Handle(processHandle)

	sessionClosed := make(chan struct{})
	go resizeConPTY(conPTY, winCh, sessionClosed)
	go func() {
		_, _ = io.Copy(pseudoconsole.InPipe(), session)
	}()
	outputDone := make(chan struct{})
	go func() {
		defer close(outputDone)
		drainConPTY(session, pseudoconsole.OutPipe())
	}()

	waitDone := make(chan processResult, 1)
	go func() {
		waitDone <- waitForProcess(process)
	}()

	return func() error {
		defer windows.CloseHandle(process)
		disconnected := false
		var result processResult
		select {
		case result = <-waitDone:
		case <-ctx.Done():
			disconnected = true
		case <-sessionClosed:
			disconnected = true
		}

		conPTY.stop()
		if disconnected {
			_ = pseudoconsole.InPipe().Close()
			_ = pseudoconsole.OutPipe().Close()
			_ = windows.TerminateProcess(process, 1)
			result = <-waitDone
		}

		closeErr := pseudoconsole.Close()
		<-outputDone
		if result.err != nil {
			return result.err
		}
		if !disconnected && result.exitCode != 0 {
			return fmt.Errorf("process exited with code %d", result.exitCode)
		}
		if !disconnected && closeErr != nil {
			return closeErr
		}
		return nil
	}, nil
}

func (c *conPTYSession) resize(width, height int) error {
	if err := validateConPTYSize(width, height); err != nil {
		return err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if c.stopped {
		return errors.New("ConPTY session is closed")
	}
	return c.ConPty.Resize(width, height)
}

func (c *conPTYSession) stop() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.stopped = true
}

func resizeConPTY(pseudoconsole *conPTYSession, winCh <-chan ssh.Window, sessionClosed chan<- struct{}) {
	defer close(sessionClosed)
	for win := range winCh {
		if win.Width > 0 && win.Height > 0 {
			_ = pseudoconsole.resize(win.Width, win.Height)
		}
	}
}

func validateConPTYSize(width, height int) error {
	if width <= 0 || width > math.MaxInt16 || height <= 0 || height > math.MaxInt16 {
		return fmt.Errorf("invalid ConPTY size: %dx%d", width, height)
	}
	return nil
}

func drainConPTY(dst io.Writer, src io.Reader) {
	buf := make([]byte, 32*1024)
	for {
		n, err := src.Read(buf)
		if n > 0 && dst != nil {
			if _, writeErr := dst.Write(buf[:n]); writeErr != nil {
				dst = nil
			}
		}
		if err != nil {
			return
		}
	}
}

type processResult struct {
	exitCode uint32
	err      error
}

func waitForProcess(process windows.Handle) processResult {
	status, err := windows.WaitForSingleObject(process, windows.INFINITE)
	if err != nil {
		return processResult{err: err}
	}
	if status != windows.WAIT_OBJECT_0 {
		return processResult{err: fmt.Errorf("unexpected process wait status: %d", status)}
	}

	var exitCode uint32
	if err := windows.GetExitCodeProcess(process, &exitCode); err != nil {
		return processResult{err: err}
	}
	return processResult{exitCode: exitCode}
}
