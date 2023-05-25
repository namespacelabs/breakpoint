package quicgrpc

import (
	"context"
	"net"

	"github.com/quic-go/quic-go"
	"google.golang.org/grpc/credentials"
	"namespacelabs.dev/breakpoint/pkg/quicnet"
)

type QuicCreds struct {
	NonQuicCreds credentials.TransportCredentials
}

var _ credentials.TransportCredentials = QuicCreds{}

func (m QuicCreds) ClientHandshake(ctx context.Context, addr string, conn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	return m.NonQuicCreds.ClientHandshake(ctx, addr, conn)
}

func (m QuicCreds) ServerHandshake(conn net.Conn) (net.Conn, credentials.AuthInfo, error) {
	if quic, ok := conn.(quicnet.Conn); ok {
		return conn, QuicAuthInfo{Conn: quic.Conn}, nil
	}

	return m.NonQuicCreds.ServerHandshake(conn)
}

func (m QuicCreds) Info() credentials.ProtocolInfo {
	return credentials.ProtocolInfo{SecurityProtocol: "insecure"}
}

func (m QuicCreds) Clone() credentials.TransportCredentials {
	return QuicCreds{NonQuicCreds: m.NonQuicCreds.Clone()}
}

func (m QuicCreds) OverrideServerName(string) error {
	return nil
}

type QuicAuthInfo struct {
	credentials.CommonAuthInfo
	Conn quic.Connection
}

func (QuicAuthInfo) AuthType() string {
	return "quic"
}
