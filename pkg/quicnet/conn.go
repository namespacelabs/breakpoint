package quicnet

import (
	"context"
	"net"

	"github.com/quic-go/quic-go"
)

type Conn struct {
	quic.Stream
	Conn quic.Connection
}

func (cw Conn) LocalAddr() net.Addr {
	return cw.Conn.LocalAddr()
}

func (cw Conn) RemoteAddr() net.Addr {
	return cw.Conn.RemoteAddr()
}

func OpenStream(ctx context.Context, conn quic.Connection) (Conn, error) {
	stream, err := conn.OpenStreamSync(ctx)
	if err != nil {
		return Conn{}, err
	}

	return Conn{Stream: stream, Conn: conn}, nil

}
