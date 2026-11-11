package sshd

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"io"
	"net"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"

	gossh "golang.org/x/crypto/ssh"
)

func newSSHClient(t *testing.T) *gossh.Client {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := gossh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	shell := []string{"/bin/sh"}
	if runtime.GOOS == "windows" {
		shell = []string{os.Getenv("COMSPEC")}
	}
	server, err := MakeServer(context.Background(), SSHServerOpts{
		AllowedUsers: []string{"runner"},
		AuthorizedKeys: map[string]string{
			string(gossh.MarshalAuthorizedKey(signer.PublicKey())): "test",
		},
		Env:   os.Environ(),
		Shell: shell,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { server.Server.Close() })
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	go func() { _ = server.Server.Serve(listener) }()
	client, err := gossh.Dial("tcp", listener.Addr().String(), &gossh.ClientConfig{
		User:            "runner",
		Auth:            []gossh.AuthMethod{gossh.PublicKeys(signer)},
		HostKeyCallback: gossh.InsecureIgnoreHostKey(),
		Timeout:         10 * time.Second,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { client.Close() })
	return client
}

func TestExecCompletesWithOpenStdin(t *testing.T) {
	client := newSSHClient(t)
	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	stdin, err := session.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	defer stdin.Close()
	var output bytes.Buffer
	session.Stdout = &output
	if err := session.Start("echo stdout&&echo stderr>&2"); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- session.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		// Allow the original input-copy wait to finish before failing the test.
		stdin.Close()
		select {
		case <-done:
		case <-time.After(5 * time.Second):
			t.Fatal("command did not complete after stdin EOF")
		}
		t.Fatal("command did not complete while stdin remained open")
	}
	if got := strings.ReplaceAll(output.String(), "\r\n", "\n"); got != "stdout\nstderr\n" {
		t.Fatalf("command output = %q, want stdout and stderr", got)
	}
}

func TestExecReceivesStdinEOF(t *testing.T) {
	client := newSSHClient(t)
	session, err := client.NewSession()
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	stdin, err := session.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	session.Stdout = &output
	command := "cat"
	if runtime.GOOS == "windows" {
		command = "findstr ."
	}
	if err := session.Start(command); err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(stdin, "input\n"); err != nil {
		t.Fatal(err)
	}
	if err := stdin.Close(); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- session.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("command did not complete after reading stdin EOF")
	}
	if got := strings.ReplaceAll(output.String(), "\r\n", "\n"); got != "input\n" {
		t.Fatalf("command output = %q, want input", got)
	}
}
