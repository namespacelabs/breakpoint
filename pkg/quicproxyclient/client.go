package quicproxyclient

import (
	"context"
	"crypto/tls"
	"errors"
	"net"
	"time"

	proxyproto "github.com/pires/go-proxyproto"
	"github.com/quic-go/quic-go"
	"github.com/rs/zerolog"
	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"
	v1 "namespacelabs.dev/breakpoint/api/public/v1"
	"namespacelabs.dev/breakpoint/pkg/bgrpc"
	"namespacelabs.dev/breakpoint/pkg/quicnet"
)

var DefaultConfig = &quic.Config{
	MaxIdleTimeout:  5 * time.Second,
	KeepAlivePeriod: 30 * time.Second,
}

type Handlers struct {
	OnAllocation func(string)
	Proxy        func(net.Conn) error
}

func Serve(ctx context.Context, endpoint string, md metadata.MD, handlers Handlers) error {
	tlsConf := &tls.Config{
		InsecureSkipVerify: true,
		NextProtos:         []string{v1.QuicProto},
	}

	zerolog.Ctx(ctx).Info().Str("endpoint", endpoint).Msg("Connecting")

	conn, err := quic.DialAddr(ctx, endpoint, tlsConf, DefaultConfig)
	if err != nil {
		return err
	}

	grpconn, err := bgrpc.DialContext(ctx, endpoint,
		grpc.WithBlock(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return quicnet.OpenStream(ctx, conn)
		}),
	)
	if err != nil {
		return err
	}

	cli := v1.NewProxyServiceClient(grpconn)

	rsrv, err := cli.Register(metadata.NewOutgoingContext(ctx, md), &v1.RegisterRequest{})
	if err != nil {
		return err
	}

	eg, ctx := errgroup.WithContext(ctx)

	eg.Go(func() error {
		for {
			stream, err := conn.AcceptStream(ctx)
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					zerolog.Ctx(ctx).Err(err).Msg("accept failed")
				}
				return err
			}

			pconn := proxyproto.NewConn(quicnet.Conn{Stream: stream, Conn: conn})

			zerolog.Ctx(ctx).Info().Stringer("remote_addr", pconn.RemoteAddr()).
				Stringer("local_addr", pconn.LocalAddr()).Msg("New remote connection")

			if err := handlers.Proxy(pconn); err != nil {
				zerolog.Ctx(ctx).Err(err).Msg("handle failed")
				return err
			}
		}
	})

	eg.Go(func() error {
		for {
			msg, err := rsrv.Recv()
			if err != nil {
				return err
			}

			handlers.OnAllocation(msg.Endpoint)
		}
	})

	return eg.Wait()
}
