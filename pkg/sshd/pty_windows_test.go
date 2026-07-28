//go:build windows

package sshd

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"net"
	"os"
	"strings"
	"testing"
	"time"

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

	const marker = "breakpoint-windows-pty-ok"
	var output bytes.Buffer
	session.Stdin = strings.NewReader("echo " + marker + "\r\nexit\r\n")
	session.Stdout = &output
	session.Stderr = &output

	if err := session.Shell(); err != nil {
		t.Fatal(err)
	}
	if err := session.Wait(); err != nil {
		t.Fatalf("Windows shell failed: %v\noutput:\n%s", err, output.String())
	}
	if !strings.Contains(output.String(), marker) {
		t.Fatalf("Windows shell output did not contain %q:\n%s", marker, output.String())
	}
}
