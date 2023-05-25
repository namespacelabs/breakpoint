package quicnet

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/rs/zerolog"
)

var (
	errClosed        = errors.New("closed")
	errAlreadyClosed = errors.New("already closed")
)

type Listener struct {
	ctx      context.Context
	listener quic.Listener

	mu    sync.Mutex
	cond  *sync.Cond
	inbox []net.Conn
	lErr  error // If set, the listener is closed.
}

func NewListener(ctx context.Context, l quic.Listener) *Listener {
	lst := &Listener{ctx: ctx, listener: l}
	lst.cond = sync.NewCond(&lst.mu)
	go lst.loop()
	return lst
}

func (l *Listener) loop() {
	for {
		conn, err := l.listener.Accept(l.ctx)
		if err != nil {
			_ = l.closeWithErr(err)
			return
		}

		go l.waitForStream(conn)
	}
}

func (l *Listener) closeWithErr(err error) error {
	l.mu.Lock()
	wasErr := l.lErr
	inbox := l.inbox
	if l.lErr == nil {
		l.lErr = err
		l.inbox = nil
	}
	l.cond.Broadcast()
	l.mu.Unlock()

	if wasErr != nil {
		return errAlreadyClosed
	}

	for _, conn := range inbox {
		_ = conn.Close()
	}

	_ = l.listener.Close()

	return nil
}

func (l *Listener) waitForStream(conn quic.Connection) {
	// If we don't see a stream within the deadline, then close the connection.
	ctx, done := context.WithTimeout(l.ctx, 10*time.Second)
	defer done()

	stream, err := conn.AcceptStream(ctx)
	if err != nil {
		zerolog.Ctx(ctx).Info().Stringer("remote_addr", conn.RemoteAddr()).
			Stringer("local_addr", conn.LocalAddr()).Err(err).Msg("Failed to accept stream")
		conn.CloseWithError(0, "")
		return
	}

	l.queue(conn, stream)
}

func (l *Listener) queue(conn quic.Connection, stream quic.Stream) {
	l.mu.Lock()
	lErr := l.lErr
	if l.lErr == nil {
		l.inbox = append(l.inbox, Conn{Conn: conn, Stream: stream})
		l.cond.Signal()
	}
	l.mu.Unlock()

	if lErr != nil {
		zerolog.Ctx(l.ctx).Info().Stringer("remote_addr", conn.RemoteAddr()).
			Stringer("local_addr", conn.LocalAddr()).Err(lErr).Msg("Listener was closed")
		conn.CloseWithError(0, "")
	}
}

func (l *Listener) Accept() (net.Conn, error) {
	l.mu.Lock()
	defer l.mu.Unlock()

	for len(l.inbox) == 0 {
		l.cond.Wait()

		if l.lErr != nil {
			return nil, l.lErr
		}

		if err := l.ctx.Err(); err != nil {
			return nil, err
		}
	}

	conn := l.inbox[0]
	l.inbox = l.inbox[1:]
	return conn, nil
}

func (l *Listener) Close() error {
	return l.closeWithErr(errClosed)
}

func (l *Listener) Addr() net.Addr {
	return l.listener.Addr()
}
