//go:build windows

package sshd

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"io"
	"net"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/gliderlabs/ssh"
	gossh "golang.org/x/crypto/ssh"
)

func TestWindowsPTYSession(t *testing.T) {
	clientKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	clientSigner, err := gossh.NewSignerFromKey(clientKey)
	if err != nil {
		t.Fatal(err)
	}

	server, err := MakeServer(context.Background(), SSHServerOpts{
		AllowedUsers: []string{"runner"},
		AuthorizedKeys: map[string]string{
			string(gossh.MarshalAuthorizedKey(clientSigner.PublicKey())): "test",
		},
		Env:   os.Environ(),
		Shell: []string{os.Getenv("COMSPEC")},
	})
	if err != nil {
		t.Fatal(err)
	}

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	defer server.Server.Close()

	go func() {
		_ = server.Server.Serve(listener)
	}()

	client, err := gossh.Dial("tcp", listener.Addr().String(), &gossh.ClientConfig{
		User:            "runner",
		Auth:            []gossh.AuthMethod{gossh.PublicKeys(clientSigner)},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	if err := session.RequestPty("xterm", 24, 80, gossh.TerminalModes{}); err != nil {
		t.Fatal(err)
	}

	stdin, err := session.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := session.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	output := make(chan []byte, 16)
	go copyOutput(stdout, output)

	if err := session.Shell(); err != nil {
		t.Fatal(err)
	}

	if _, err := io.WriteString(stdin, "set /a 6*7\r"); err != nil {
		t.Fatal(err)
	}
	waitForOutput(t, output, "42")

	if err := session.WindowChange(37, 113); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(stdin, `powershell -NoProfile -Command "$d=(Get-Date).AddSeconds(5); do {$s=[Console]::WindowWidth.ToString()+'x'+[Console]::WindowHeight; Start-Sleep -Milliseconds 20} until ($s -eq '113x37' -or (Get-Date) -gt $d); Write-Output ('SIZE='+$s)"`+"\r"); err != nil {
		t.Fatal(err)
	}
	waitForOutput(t, output, "SIZE=113x37")

	if _, err := io.WriteString(stdin, "exit\r"); err != nil {
		t.Fatal(err)
	}
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- session.Wait()
	}()
	select {
	case err := <-waitDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Windows shell did not exit")
	}
}

func copyOutput(src io.Reader, output chan<- []byte) {
	defer close(output)
	buf := make([]byte, 4096)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			chunk := append([]byte(nil), buf[:n]...)
			output <- chunk
		}
		if err != nil {
			return
		}
	}
}

func waitForOutput(t *testing.T, output <-chan []byte, expected string) {
	t.Helper()
	timer := time.NewTimer(10 * time.Second)
	defer timer.Stop()

	var received strings.Builder
	for {
		select {
		case chunk, ok := <-output:
			if !ok {
				t.Fatalf("output closed before %q was received; output:\n%s", expected, received.String())
			}
			received.Write(chunk)
			if strings.Contains(received.String(), expected) {
				return
			}
		case <-timer.C:
			t.Fatalf("timed out waiting for %q; output:\n%s", expected, received.String())
		}
	}
}

func TestWindowsPTYStopsWhenSessionCloses(t *testing.T) {
	inputReader, inputWriter := io.Pipe()
	defer inputWriter.Close()

	winCh := make(chan ssh.Window)
	waitPty, err := startPty(
		context.Background(),
		struct {
			io.Reader
			io.Writer
		}{inputReader, io.Discard},
		ssh.Pty{Window: ssh.Window{Width: 80, Height: 24}},
		winCh,
		exec.Command(os.Getenv("COMSPEC")),
	)
	if err != nil {
		t.Fatal(err)
	}

	close(winCh)
	waitDone := make(chan error, 1)
	go func() {
		waitDone <- waitPty()
	}()
	select {
	case err := <-waitDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("Windows shell did not stop after its SSH session closed")
	}
}

func TestValidateConPTYSize(t *testing.T) {
	if err := validateConPTYSize(80, 24); err != nil {
		t.Fatal(err)
	}
	if err := validateConPTYSize(1<<15, 24); err == nil {
		t.Fatal("oversized ConPTY width was accepted")
	}
	if err := validateConPTYSize(80, 1<<15); err == nil {
		t.Fatal("oversized ConPTY height was accepted")
	}
}
