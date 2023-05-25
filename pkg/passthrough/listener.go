package passthrough

import (
	"context"
	"errors"
	"net"

	"go.uber.org/atomic"
)

type Listener struct {
	ctx    context.Context
	addr   net.Addr
	ch     chan net.Conn
	closed *atomic.Bool
}

func NewListener(ctx context.Context, addr net.Addr) Listener {
	return Listener{ctx: ctx, addr: addr, ch: make(chan net.Conn), closed: atomic.NewBool(false)}
}

func (pl Listener) Accept() (net.Conn, error) {
	select {
	case <-pl.ctx.Done():
		return nil, pl.ctx.Err()

	case conn, ok := <-pl.ch:
		if !ok {
			return nil, errors.New("listener is closed")
		}
		return conn, nil
	}
}

func (pl Listener) Addr() net.Addr {
	return pl.addr
}

func (pl Listener) Close() error {
	if !pl.closed.Swap(true) {
		close(pl.ch)
		return nil
	} else {
		return errors.New("already closed")
	}
}

func (pl Listener) Offer(conn net.Conn) error {
	if pl.closed.Load() {
		return errors.New("listener closed")
	}

	pl.ch <- conn
	return nil
}
