package sshd

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"fmt"
	"io"
	"net"
	"os/exec"
	"runtime"
	"time"

	"github.com/gliderlabs/ssh"
	"github.com/rs/zerolog"
	"go.uber.org/atomic"
	gossh "golang.org/x/crypto/ssh"
	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

type SSHServerOpts struct {
	AllowedUsers   []string
	AuthorizedKeys map[string]string // Key to owner
	Env            []string
	Shell          []string
	Dir            string

	InteractiveMOTD func(io.Writer)
	WriteNotify     func()
}

type sshKey struct {
	Key   ssh.PublicKey
	Owner string
}

type SSHServer struct {
	Server         *ssh.Server
	NumConnections func() uint32
}

func MakeServer(ctx context.Context, opts SSHServerOpts) (*SSHServer, error) {
	var authorizedKeys []sshKey
	for key, owner := range opts.AuthorizedKeys {
		key, _, _, _, err := ssh.ParseAuthorizedKey([]byte(key))
		if err != nil {
			return nil, err
		}
		authorizedKeys = append(authorizedKeys, sshKey{key, owner})
	}

	l := zerolog.Ctx(ctx).With().Str("service", "sshd").Logger()

	connCount := atomic.NewUint32(0)

	srv := &ssh.Server{
		Handler: func(session ssh.Session) {
			key, _ := lookupKey(authorizedKeys, session.PublicKey())
			sessionLog := l.With().Stringer("remote_addr", session.RemoteAddr()).Str("owner", key.Owner).Logger()

			sessionLog.Info().Str("user", session.User()).Msg("incoming ssh session")

			args := opts.Shell[1:]
			if session.RawCommand() != "" {
				if runtime.GOOS == "windows" {
					args = []string{"/C", session.RawCommand()}
				} else {
					args = []string{"-c", session.RawCommand()}
				}
			}

			cmd := exec.Command(opts.Shell[0], args...)
			cmd.Env = slices.Clone(opts.Env)
			cmd.Dir = opts.Dir

			if ssh.AgentRequested(session) {
				l, err := ssh.NewAgentListener()
				if err != nil {
					fmt.Fprintf(session, "Failed to forward agent.\n")
				} else {
					defer l.Close()
					go ssh.ForwardAgentConnections(l, session)
					cmd.Env = append(cmd.Env, fmt.Sprintf("%s=%s", "SSH_AUTH_SOCK", l.Addr().String()))
				}
			}

			ptyReq, winCh, isPty := session.Pty()

			sessionLog.Info().Bool("ssh_agent", ssh.AgentRequested(session)).Bool("pty", isPty).Msg("ssh session")

			ctx, cancel := context.WithCancel(session.Context())
			defer cancel()

			// Make sure that the connection with the client is kept alive.
			go keepAlive(ctx, sessionLog, session)

			// Wrapping the session lets us know when writes are happening.
			nsess := newNotifyingSession(ctx, session, opts.WriteNotify)

			if isPty {
				// Print MOTD only if no command was provided
				if opts.InteractiveMOTD != nil && nsess.RawCommand() == "" {
					opts.InteractiveMOTD(nsess)
				}

				if err := handlePty(nsess, ptyReq, winCh, cmd); err != nil {
					sessionLog.Err(err).Msg("pty start failed")
					nsess.Exit(1)
					return
				}
			} else {
				cmd.Stdout = nsess
				cmd.Stderr = nsess
				if err := cmd.Start(); err != nil {
					sessionLog.Err(err).Msg("start failed")
					nsess.Exit(1)
					return
				}
			}

			// XXX pass exit code to caller?
			err := cmd.Wait()
			sessionLog.Info().Err(err).Msg("ssh session end")
		},

		SessionRequestCallback: func(sess ssh.Session, requestType string) bool {
			return len(opts.AllowedUsers) == 0 || slices.Contains(opts.AllowedUsers, sess.User())
		},

		PublicKeyHandler: func(ctx ssh.Context, key ssh.PublicKey) bool {
			_, allowed := lookupKey(authorizedKeys, key)
			return allowed
		},

		LocalPortForwardingCallback: func(ctx ssh.Context, destinationHost string, destinationPort uint32) bool {
			sessionLog := l.With().Stringer("remote_addr", ctx.RemoteAddr()).Logger()
			sessionLog.Info().Str("dst", fmt.Sprintf("%s:%d", destinationHost, destinationPort)).Msg("Port forward request")
			return true
		},

		SubsystemHandlers: map[string]ssh.SubsystemHandler{
			"sftp": makeSftpHandler(l),
		},

		ConnCallback: func(ctx ssh.Context, conn net.Conn) net.Conn {
			connCount.Inc()
			go func() {
				<-ctx.Done()
				connCount.Dec()
			}()

			return conn
		},
	}

	srv.ChannelHandlers = maps.Clone(ssh.DefaultChannelHandlers)
	srv.ChannelHandlers["direct-tcpip"] = ssh.DirectTCPIPHandler

	t := time.Now()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	signer, err := gossh.NewSignerFromKey(key)
	if err != nil {
		return nil, err
	}

	srv.HostSigners = append(srv.HostSigners, signer)

	zerolog.Ctx(ctx).Info().Str("host_key_fingerprint", gossh.FingerprintSHA256(signer.PublicKey())).Dur("took", time.Since(t)).Msg("Generated ssh host key")

	return &SSHServer{
		Server:         srv,
		NumConnections: connCount.Load,
	}, nil
}

func lookupKey(allowed []sshKey, key ssh.PublicKey) (sshKey, bool) {
	for _, allowed := range allowed {
		if ssh.KeysEqual(key, allowed.Key) {
			return allowed, true
		}
	}
	return sshKey{}, false
}

type notifyingSession struct {
	ssh.Session
	notifyCh chan struct{}
	notify   func()
}

func newNotifyingSession(ctx context.Context, s ssh.Session, notify func()) ssh.Session {
	if notify == nil {
		return s
	}

	sess := notifyingSession{
		Session:  s,
		notifyCh: make(chan struct{}),
		notify:   notify,
	}
	go sess.listen(ctx)
	return sess
}

func (s notifyingSession) listen(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-s.notifyCh:
		}

		s.notify()

		select {
		case <-ctx.Done():
			return
		case <-time.After(1 * time.Second):
		}
	}
}

func (s notifyingSession) Write(p []byte) (int, error) {
	select {
	case s.notifyCh <- struct{}{}:
	default: // avoid blocking
	}
	return s.Session.Write(p)
}
