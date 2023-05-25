package quicproxy

import (
	"context"
	"errors"
	"net"
	"time"

	"github.com/quic-go/quic-go"
	"github.com/rs/zerolog"
	"inet.af/tcpproxy"
	"namespacelabs.dev/breakpoint/pkg/quicnet"
)

type Allocation struct {
	Endpoint string
}

type ProxyFrontend interface {
	ListenAndServe(context.Context) error
	Handle(context.Context, Handlers) error
}

type Handlers struct {
	OnAllocation func(Allocation) error
	OnCleanup    func(Allocation, error)
	HandleConn   func(net.Conn)
}

func ServeProxy(ctx context.Context, frontend ProxyFrontend, conn quic.Connection, callback func(Allocation) error) error {
	backend := tcpproxy.To("backend")
	backend.DialTimeout = 30 * time.Second
	backend.ProxyProtocolVersion = 1
	backend.DialContext = func(ctx context.Context, _, _ string) (net.Conn, error) {
		return quicnet.OpenStream(ctx, conn)
	}

	return frontend.Handle(ctx, Handlers{
		OnAllocation: func(alloc Allocation) error {
			zerolog.Ctx(ctx).Info().Str("allocation", alloc.Endpoint).Msg("New allocation")
			return callback(alloc)
		},
		OnCleanup: func(alloc Allocation, err error) {
			zerolog.Ctx(ctx).Info().Str("allocation", alloc.Endpoint).Err(cancelIsOK(err)).Msg("Released allocation")
		},
		HandleConn: backend.HandleConn,
	})
}

func cancelIsOK(err error) error {
	if errors.Is(err, context.Canceled) {
		return nil
	}

	return err
}
